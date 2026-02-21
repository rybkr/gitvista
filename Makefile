.PHONY: test ci lint integration e2e build build-cli clean help

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

## e2e: Run end-to-end tests (builds gitvista-cli, compares output against git)
e2e:
	$(GOTEST) -v -race -tags=e2e ./test/e2e/...

## format: Auto-format the source code
format:
	gofmt -w .

## lint: Run go vet
lint:
	go vet ./...

## build: Build all binaries
build: build-cli
	$(GOBUILD) -v -o gitvista ./cmd/vista

## build-cli: Build the gitvista-cli binary
build-cli:
	$(GOBUILD) -v -o gitvista-cli ./cmd/gitcli

## ci: Run all CI checks (tests, lint, integration tests, e2e tests, build)
ci: test format lint integration e2e build

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f gitvista vista gitvista-cli

## cloc: Count lines of code
cloc:
	cloc .
