BINARY := rig
PKG := ./cmd/rig

.PHONY: build run install clean

build:
	go build -o bin/$(BINARY) $(PKG)

run:
	go run $(PKG)

install:
	go install $(PKG)

clean:
	rm -rf bin/
