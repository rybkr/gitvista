.PHONY: test ci lint integration build clean help

GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean

.DEFAULT_GOAL := help

## help: Display this informational message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## test: Run all unit tests
test:
	$(GOTEST) -v -race -cover -timeout=60s ./...

## integration: Run integration tests
integration:
	$(GOTEST) -v -race -tags=integration ./test/integration/...

## format: Auto-format the source code
format:
	gofmt -w .

## lint: Run golangci-lint
lint:
	go vet ./...
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run --timeout=60s

## build: Build the binary
build:
	$(GOBUILD) -v -o gitvista ./cmd/vista

## ci: Run all CI checks (tests, lint, integration tests, build)
ci: test format lint integration build

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f gitvista vista

## cloc: Count lines of code
cloc:
	cloc .
