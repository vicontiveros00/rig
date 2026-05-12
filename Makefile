BINARY := rig
PKG := ./cmd/rig
VERSION_PKG := github.com/vicontiveros00/rig/internal/version

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT)"

.PHONY: build run install clean

build:
	go build $(LDFLAGS) -o bin/$(BINARY) $(PKG)

run:
	go run $(LDFLAGS) $(PKG)

install:
	go install $(LDFLAGS) $(PKG)

clean:
	rm -rf bin/
