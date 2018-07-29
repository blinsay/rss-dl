NAME := rss-dl
PKG := github.com/blinsay/rss-dl

.PHONY: all
all: clean build fmt lint test unused staticcheck install

# build and install

.PHONY: clean
clean:
	@$(RM) $(NAME)

.PHONY: build
build: $(NAME)

.PHONY: install
install:
	@echo "+$@"
	@go install -a .

$(NAME): $(wildcard *.go)
	@echo "+$@"
	@go build -o $(NAME) .

# deps

.PHONY: dep
dep:
	@echo "+$@"
	@dep ensure

# tests

.PHONY: test
	@echo "+ $@"
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

