.PHONY: all test lint fmt clean help

all: fmt lint test

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

clean:
	@echo "Cleaning..."
	go clean -testcache

help:
	@echo "Available targets:"
	@echo "  all   - Format, lint, and test (default)"
	@echo "  test  - Run all tests"
	@echo "  lint  - Run golangci-lint"
	@echo "  fmt   - Format code and tidy modules"
	@echo "  clean - Clean test cache"
	@echo "  help  - Show this help message"
