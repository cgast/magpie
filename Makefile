BINARY := magpie
PKG    := github.com/cgast/magpie
VERSION ?= dev
LDFLAGS := -s -w -X $(PKG)/cmd.Version=$(VERSION)

.PHONY: build test vet install clean macos

## build: build for the host platform into ./bin
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

## test: run tests
test:
	go test ./...

## vet: static checks
vet:
	go vet ./...

## install: install into $GOPATH/bin
install:
	go install -ldflags "$(LDFLAGS)" .

## macos: build a universal (Intel + Apple Silicon) macOS binary for sharing
macos:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)_amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)_arm64 .
	lipo -create -output bin/$(BINARY) bin/$(BINARY)_amd64 bin/$(BINARY)_arm64
	rm -f bin/$(BINARY)_amd64 bin/$(BINARY)_arm64
	@echo "built universal binary at bin/$(BINARY)"

clean:
	rm -rf bin

## pkg: build a macOS .pkg installer into ./dist (macOS only)
pkg:
	VERSION=$(VERSION) ./scripts/build-pkg.sh