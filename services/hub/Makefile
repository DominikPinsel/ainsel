.PHONY: all build test lint fmt clean help

MODULE  := github.com/DominikPinsel/ainsel/services/hub
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(MODULE)/pkg/version.Version=$(VERSION) \
           -X $(MODULE)/pkg/version.Commit=$(COMMIT) \
           -X $(MODULE)/pkg/version.BuildDate=$(DATE)

all: fmt lint test build

build:
	@echo "Building hub binary..."
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/hub ./cmd/hub

test:
	@echo "Running tests..."
	go test -v ./...

lint:
	@echo "Running linter..."
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	gofmt -s -w .
	go mod tidy
	go mod vendor

clean:
	rm -rf bin/

help:
	@echo "  build test lint fmt clean"
