# Thin convenience wrapper. GoReleaser (.goreleaser.yaml) is the source of truth
# for cross-platform builds; these targets just call it / the go toolchain.
# (On Windows, `make` isn't installed by default — run the commands directly.)

BINARY  := shortorder
PKG     := ./cmd/shortorder
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build run test vet fmt list snapshot release check clean tag

## build: compile a host binary into ./bin
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

## run: run the service locally (defaults to :8080)
run:
	go run $(PKG)

## list: build then print detected supported printers
list: build
	./bin/$(BINARY) -list

## test / vet / fmt
test:
	go test ./...
vet:
	go vet ./...
fmt:
	gofmt -w cmd internal

## snapshot: build the full cross-platform matrix locally into ./dist (no publish)
snapshot:
	goreleaser release --snapshot --clean

## release: tag-driven real release (normally run by CI on a pushed tag)
release:
	goreleaser release --clean

## check: validate the GoReleaser config
check:
	goreleaser check

clean:
	rm -rf dist bin

## tag: create and push a semver tag, triggering a release. Usage: make tag V=v0.1.0
tag:
	@test -n "$(V)" || (echo "usage: make tag V=v0.1.0"; exit 1)
	git tag -a "$(V)" -m "Release $(V)" && git push origin "$(V)"
