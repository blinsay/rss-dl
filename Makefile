NAME := rss-dl
PKG := github.com/blinsay/rss-dl

VERSION := $(shell cat VERSION.txt)
GITCOMMIT := $(shell git rev-parse --short head)

GOOSARCHES := $(shell cat .goosarch)
VERSION_FLAGS=-X $(PKG)/version.VERSION=$(VERSION) -X $(PKG)/version.GITCOMMIT=$(GITCOMMIT)
GO_LDFLAGS=-ldflags "$(VERSION_FLAGS)"
GO_LDFLAGS_STATIC=-ldflags "$(VERSION_FLAGS) -extldflags -static"

.PHONY: all
all: clean build fmt lint test unused staticcheck install

# build and install

.PHONY: clean
clean:
	@echo "+$@"
	@$(RM) $(NAME)
	@$(RM) -r build/

.PHONY: build
build: $(NAME)

$(NAME): $(wildcard *.go)
	@echo "+$@"
	@go build $(GO_LDFLAGS) -o $(NAME) .

.PHONY: install
install:
	@echo "+$@"
	@go install -a $(GO_LDFLAGS) .

define build_cross
mkdir -p build/$(1)/$(2);
GOOS=$(1) GOARCH=$(2) CGO_ENABLED=0 go build $(GO_LDFLAGS) -o build/$(1)/$(2)/$(NAME)-$(1)-$(2) .;
endef

.PHONY: cross
cross:
	@echo "+$@"
	@$(foreach GOOSARCH, $(GOOSARCHES), echo ++$(GOOSARCH) && $(call build_cross,$(subst /,,$(dir $(GOOSARCH))),$(notdir $(GOOSARCH))))

# deps

.PHONY: dep
dep:
	@echo "+$@"
	@dep ensure

# tests

.PHONY: test
	@echo "+$@"
	@go test ./...

# linting and static analysis

.PHONY: fmt
fmt:
	@echo "+$@"
	@gofmt -s -l .

.PHONY: lint
lint:
	@echo "+$@"
	@golint ./...


.PHONY: vet
vet:
	@echo "+$@"
	@go vet ./...

.PHONY: unused
unused:
	@echo "+$@"
	@unused ./...

.PHONY: staticcheck
staticcheck:
	@echo "+$@"
	@staticcheck ./... | tee /dev/stderr

