SHELL := /bin/bash
MODULE := github.com/dxinli/collect-code
DIST := dist
BINARY := collect-code

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

GOBUILD := GOFLAGS= CGO_ENABLED=0 go build -trimpath -buildvcs=false -buildmode=exe -ldflags "$(LDFLAGS)"

.PHONY: build build-linux-amd64 build-linux-arm64 build-all verify-static test test-race vet fmt-check check clean

build:
	@mkdir -p $(DIST)
	$(GOBUILD) -o $(DIST)/$(BINARY) ./cmd/collect-code

build-linux-amd64:
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(DIST)/$(BINARY)-linux-amd64 ./cmd/collect-code

build-linux-arm64:
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(DIST)/$(BINARY)-linux-arm64 ./cmd/collect-code

build-all: build-linux-amd64 build-linux-arm64

verify-static: build-all
	./scripts/verify-static.sh $(DIST)/$(BINARY)-linux-amd64 amd64
	./scripts/verify-static.sh $(DIST)/$(BINARY)-linux-arm64 arm64

test:
	GOFLAGS= go test ./...

test-race:
	GOFLAGS= CGO_ENABLED=1 go test -race ./...

vet:
	GOFLAGS= go vet ./...

fmt-check:
	@files="$$(gofmt -l cmd internal)"; \
	if [ -n "$$files" ]; then echo "gofmt needed:"; echo "$$files"; exit 1; fi

check: fmt-check vet test build-all verify-static
	GOFLAGS= go run ./cmd/collect-code config-check --config configs/example

clean:
	rm -rf $(DIST)
