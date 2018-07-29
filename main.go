package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/blinsay/rss-dl/version"
)

// TODO(benl): tests

var (
	// enable verbose output
	verbose bool

	// print the printVersion and exit
	printVersion bool

	// how many parallel downloads to do
	downloaders int

	// where to put downloads
	downloadDir string

	// where to use as scratch space
	tempDir string

	// whether or not to clobber existing files
	clobber bool

	// whether to use the rss.Item.Title as the name of the download or not. defaults
	// to using the url filename.
	useTitleAsFilename bool

	// the timeout for the rss feed itself. can be pretty short.
	feedTimeout time.Duration

	// the timeout for fetchign individual feed items. this probably should be
	// long to account for the fact that audio files are big.
	itemTimeout time.Duration
)

func init() {
	log.SetFlags(0)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "rss-dl [flags] <url> [urls ...]\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Flags\n")
		flag.PrintDefaults()
	}

	flag.BoolVar(&verbose, "verbose", false, "turn on verbose output")
	flag.BoolVar(&printVersion, "version", false, "print the version and exit")
	flag.IntVar(&downloaders, "p", -1, "number of parallel downloads. if 0 or negative, uses 2x the number of available CPUs")
	flag.StringVar(&downloadDir, "dir", "", "the directory to download files to")
	flag.StringVar(&tempDir, "tempdir", "", "the directory to use as a temporary directory when downloading files")
	flag.BoolVar(&clobber, "clobber", false, "clobber files with the same name in the download directory")
	flag.BoolVar(&useTitleAsFilename, "use-title-as-name", false, "use the RSS episode title as the filename")
	flag.DurationVar(&feedTimeout, "feed-timeout", 3*time.Second, "timeout for fetching the rss feed")
	flag.DurationVar(&itemTimeout, "item-timeout", 10*time.Second, "timeout for fetching an individual file")
}

func main() {
	flag.Parse()

	if printVersion {
		log.Printf("%s (%s)", version.VERSION, version.GITCOMMIT)
		return
	}

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if downloadDir == "" {
		log.Fatalf("oops: please specify a download directory")
	}
	if stat, err := os.Stat(downloadDir); err != nil || !stat.IsDir() {
		log.Fatalf("oops: specify an existing directory for downloads")
	}

	if tempDir == "" {
		d, err := ioutil.TempDir("", "rss-dl")
		if err != nil {
			panic(err)
		}
		tempDir = d
	}
	if stat, err := os.Stat(tempDir); err != nil || !stat.IsDir() {
		log.Fatalf("welp: the specified temp dir doesn't exist: %s", tempDir)
	}

	var wg sync.WaitGroup
	items := make(chan *item)

	if downloaders < 1 {
		downloaders = runtime.NumCPU() * 2
	}
	for i := 0; i < downloaders; i++ {
		wg.Add(1)
		go downloadItems(&wg, items)
	}

	for _, feedURL := range flag.Args() {
		fetchFeed(feedURL, items)
	}
	close(items)
	wg.Wait()
}

// fetch a feed at feed url and shove items into a channel. logs info on error,
// but never kills the process so that other feed urls have a chance to get
// processed.
func fetchFeed(feedURL string, items chan<- *item) {
	request, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), feedTimeout)
	defer cancel()
	request = request.WithContext(ctx)

	resp, err := http.DefaultClient.Do(request)
	if urlErr, isURLErr := err.(*url.Error); isURLErr && urlErr.Timeout() || err == context.DeadlineExceeded {
		log.Printf("welp: timed out fetching %s", feedURL)
		return
	}
	if err != nil {
		log.Printf("welp: couldn't fetch %q", feedURL)
		if verbose {
			log.Printf("\t%s", err)
		}
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("welp: bad response fetching %s (%d)", feedURL, resp.StatusCode)
		return
	}

	var feed feed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		log.Printf("welp: %s isn't an rss feed", feedURL)
		return
	}

	for _, item := range feed.Channel.Items {
		items <- item
	}
}

// downloadItems pulls items off a channel and downloads them, logging info to
// stderr. use the log package for all output so that logging isn't weirdly
// interleaved
func downloadItems(wg *sync.WaitGroup, items <-chan *item) {
	defer wg.Done()

	for item := range items {
		status := downloadItem(item)

		if status.fatal {
			log.Fatalf("weeeelp: crashed downloading %s:\n\n%s: %s", status.name, status.msg, status.err)
		}

		if status.err != nil {
			log.Printf("welp: downloading %s failed: %s", status.name, status.msg)
			if verbose {
				log.Println("\t", status.err)
			}
			continue
		}

		if !status.downloaded {
			log.Printf("fyi: %s already exists", status.filename)
			continue
		}

		log.Printf("cool: downloaded %s", status.filename)
	}
}

// the status of a download. always includes an item. if filename is not blank,
// the item was successfully downloaded to that filename.
//
// if msg and error are non-zero, there was a problem downloading the item. if
// fatal is true, the error is bad enough to crash the program.
type status struct {
	name string

	filename   string
	downloaded bool

	err   error
	msg   string
	fatal bool
}

// download an item to downloadDir. partial downloads should never make it to
// the final downloadDir. safe to call concurrently after flag.Parse() happens
//
// returns a fatal status if there's a problem creating tempfiles, changing file
// perms, or anything else that the program can't really handle.
func downloadItem(item *item) status {
	name := truncate(item.Title, 60)

	downloadURL, err := downloadURL(item)
	if err != nil {
		return status{name: name, err: err, msg: "bad url"}
	}

	request, err := http.NewRequest(http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return status{name: name, err: err, msg: "invalid download url"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), itemTimeout)
	defer cancel()
	request = request.WithContext(ctx)

	resp, err := http.DefaultClient.Do(request)
	if urlErr, isURLErr := err.(*url.Error); isURLErr && urlErr.Timeout() || err == context.DeadlineExceeded {
		return status{name: name, err: err, msg: "timed out"}
	}
	if err != nil {
		return status{name: name, err: err, msg: "download failed"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return status{name: name, err: err, msg: fmt.Sprintf("unexpected response from the server: %d", resp.StatusCode)}
	}

	// check the filename and see if it exists before doing the download. if there's
	// no clobbering happening and the file exists, there's no reason to actually
	// copy bytes anywhere.
	var filename string
	if useTitleAsFilename {
		filename = escapeTitle(item.Title) + filepath.Ext(resp.Request.URL.Path)
	} else {
		filename = filepath.Base(resp.Request.URL.Path)
	}
	finalPath := filepath.Join(downloadDir, filename)

	if stat, _ := os.Stat(finalPath); !clobber && stat != nil {
		return status{name: name, filename: filename}
	}

	// make a tempfile, download things, rename, and peel out
	tempFile, err := ioutil.TempFile(tempDir, truncate(filename, 20))
	if err != nil {
		return status{name: name, msg: "creating a tempfile failed", err: err, fatal: true}
	}
	if _, err := io.Copy(bufio.NewWriter(tempFile), bufio.NewReader(resp.Body)); err != nil {
		return status{name: name, msg: "download failed", err: err}
	}
	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return status{name: name, msg: "moving file failed", err: err, fatal: true}
	}
	if err := os.Chmod(finalPath, os.FileMode(0644)); err != nil {
		return status{name: name, msg: "cmmod failed", err: err, fatal: true}
	}

	return status{name: name, filename: filename, downloaded: true}
}

func downloadURL(item *item) (*url.URL, error) {
	itemURL, err := url.Parse(item.Enclosure.URL)
	if err != nil {
		return nil, err
	}
	return itemURL, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// remove reserved characters but leave the rest of the unicode alone. this
// seems dumb but better than using a regexp.
//
// https://en.wikipedia.org/wiki/Filename#Reserved_characters_and_words
func escapeTitle(s string) string {
	var escaped []rune
	for _, r := range s {
		switch r {
		case '/', '\\', '?', '%', '*', ':', '|', '"', '<', '>':
			escaped = append(escaped, '_')
		default:
			escaped = append(escaped, r)
		}
	}
	return string(escaped)
}

// RSS feed types are all adapted from the types in github.com/gorilla/feeds

// a feed is a top-level rss feed element
type feed struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Xmlns   string   `xml:"xmlns:content,attr"`
	Channel *channel `xml:"channel"`
}

// a channel is the thing that contains everything
type channel struct {
	XMLName       xml.Name `xml:"channel"`
	PubDate       string   `xml:"pubDate"`
	LastBuildDate string   `xml:"lastBuildDate,omitempty"`
	Image         *image   `xml:"image"`
	Items         []*item  `xml:"item"`
}

// an image in an rss feed
type image struct {
	XMLName xml.Name `xml:"image"`
	URL     string   `xml:"url"`
	Title   string   `xml:"title"`
	Link    string   `xml:"link"`
	Width   int      `xml:"width,omitempty"`
	Height  int      `xml:"height,omitempty"`
}

// an item in an rss feed
type item struct {
	XMLName     xml.Name   `xml:"item"`
	Title       string     `xml:"title"`
	Link        string     `xml:"link"`
	Description string     `xml:"description"`
	Author      string     `xml:"author,omitempty"`
	Category    string     `xml:"category,omitempty"`
	Enclosure   *enclosure `xml:"enclosure"`
	PubDate     string     `xml:"pubDate,omitempty"`
}

// an enclosure is the piece of media that's part of an item. if you're scraping
// an rss feed, this is what you care about.
type enclosure struct {
	XMLName xml.Name `xml:"enclosure"`
	URL     string   `xml:"url,attr"`
	Length  string   `xml:"length,attr"`
	Type    string   `xml:"type,attr"`
}
