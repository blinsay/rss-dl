# rss-dl

A cli tool for downloading an entire RSS feed.

#### installing

*Binaries*

Prebuilt binaries are available on the [releases page](https://github.com/blinsay/rss-dl/releases).

*From Source*

Using a working Go toolchain:

```
go get github.com/blinsay/rss-dl
```


#### usage

```
$ rss-dl -h
rss-dl [flags] <url> [urls ...]

Flags
  -clobber
        clobber files with the same name in the download directory
  -dir string
        the directory to download files to
  -feed-timeout duration
        timeout for fetching the rss feed (default 3s)
  -item-timeout duration
        timeout for fetching an individual file (default 10s)
  -p int
        number of parallel downloads. if 0 or negative, uses 2x the number of available CPUs (default -1)
  -tempdir string
        the directory to use as a temporary directory when downloading files
  -use-title-as-name
        use the RSS episode title as the filename
  -verbose
        turn on verbose output
  -version
        print the version and exit
```
