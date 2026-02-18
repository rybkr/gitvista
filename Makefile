.PHONY: test ci lint integration build clean help

GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean

.DEFAULT_GOAL := help

help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## test: Run all unit tests
test:
	$(GOTEST) -v ./...

## ci: Run all CI checks (tests, lint, integration tests, build)
ci: test lint integration build

## lint: Run golangci-lint
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run --timeout=5m

## integration: Run integration tests
integration:
	$(GOTEST) -v -race -tags=integration ./test/integration/...

## build: Build the binary
build:
	$(GOBUILD) -v -o gitvista ./cmd/vista

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f gitvista

## cloc: Count lines of code
cloc:
	cloc .
