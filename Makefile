.PHONY: test ci-local ci-remote lint integration e2e build build-cli clean help setup-hooks \
         format format-check vet security validate-js cover cover-html dev-check check-imports \
         check-vuln docker-build deps-check

GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean

.DEFAULT_GOAL := help

## help: Display this informational message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## setup-hooks: Install pre-commit hooks and required tools
setup-hooks:
	@echo "Setting up pre-commit hooks..."
	@bash scripts/setup-hooks.sh

## test: Run all unit tests
test:
	$(GOTEST) -v -race -cover -timeout=60s ./...

## cover: Run tests with coverage
cover:
	@mkdir -p test/cover
	$(GOTEST) -v -race -timeout=60s -covermode=atomic \
		-coverprofile=test/cover/coverage.out -coverpkg=./internal/... ./...

## cover-html: Generate and open HTML coverage report
cover-html: cover
	@go tool cover -html=test/cover/coverage.out -o test/cover/coverage.html
	@echo "Coverage report: test/cover/coverage.html"
	@if command -v open >/dev/null; then \
		open test/cover/coverage.html; \
	elif command -v xdg-open >/dev/null; then \
		xdg-open test/cover/coverage.html; \
	fi

## integration: Run integration tests
integration:
	$(GOTEST) -v -race -tags=integration -timeout=60s ./test/integration/...

## e2e: Run end-to-end tests (builds gitvista-cli, compares output against git)
e2e:
	$(GOTEST) -v -race -tags=e2e -timeout=60s ./test/e2e/...

## format: Auto-format the source code with gofmt
format:
	@echo "Formatting code with gofmt..."
	@gofmt -w .

## check-imports: Organize and format imports with goimports
check-imports:
	@echo "Checking and fixing imports..."
	@if command -v goimports >/dev/null; then \
		goimports -w .; \
	else \
		echo "goimports not found - install with: go install golang.org/x/tools/cmd/goimports@latest"; \
		exit 1; \
	fi

## vet: Run go vet static analysis
vet:
	@echo "Running go vet..."
	@go vet ./...

## security: Run security checks (govulncheck, gosec)
security: check-vuln
	@echo "Running gosec security scanner..."
	@if command -v gosec >/dev/null; then \
		gosec -quiet -exclude=G304,G204 ./internal/...; \
	else \
		echo "gosec not found - install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
		exit 1; \
	fi

## check-vuln: Check for known vulnerabilities with govulncheck
check-vuln:
	@echo "Checking for known vulnerabilities..."
	@if command -v govulncheck >/dev/null; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not found - install with: go install github.com/golang/vuln/cmd/govulncheck@latest"; \
		exit 1; \
	fi

## lint: Run golangci-lint
lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run --config=.golangci.yml; \
	else \
		echo "golangci-lint not found - install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## validate-js: Validate JavaScript syntax and ES module compliance
validate-js:
	@echo "Validating JavaScript..."
	@find web -name '*.js' -type f | while read file; do \
		echo "Checking $$file..."; \
		if ! node --check "$$file"; then \
			echo "Syntax error in $$file"; \
			exit 1; \
		fi; \
	done
	@echo "Checking for CommonJS in ES modules..."
	@if grep -rn "module.exports\|require(" web/ --include='*.js' | grep -v "// @allow-commonjs"; then \
		echo "Found CommonJS syntax in ES modules"; \
		exit 1; \
	fi
	@echo "JavaScript validation passed"

## build: Build all binaries
build: build-cli
	@echo "Building main binary..."
	$(GOBUILD) -v -o gitvista ./cmd/vista

## build-cli: Build the gitvista-cli binary
build-cli:
	@echo "Building CLI binary..."
	$(GOBUILD) -v -o gitvista-cli ./cmd/gitcli

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t gitvista:latest .

## deps-check: Check and tidy Go dependencies
deps-check:
	@echo "Checking dependencies..."
	@go mod download
	@go mod verify
	@if go mod tidy -check; then \
		echo "Dependencies are clean"; \
	else \
		echo "Dependencies need tidying - run 'go mod tidy'"; \
		exit 1; \
	fi

## dev-check: Run fast local checks (format, imports, vet) - suitable for CI pre-checks
dev-check: format check-imports vet
	@echo "Running development checks..."

## format-check: Verify all Go files are properly formatted (fails if not)
format-check:
	@echo "Checking gofmt compliance..."
	@UNFORMATTED=$$(gofmt -l . | grep -v vendor || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Files not formatted:"; echo "$$UNFORMATTED"; \
		echo "Run 'make format' to fix"; exit 1; \
	fi
	@echo "All files properly formatted"

## ci-local: Run CI checks that work offline (no Docker or network needed)
ci-local: format-check check-imports vet lint security test integration e2e validate-js build
	@echo "All local CI checks passed!"

## ci-remote: Run all CI checks including Docker build and dependency verification
ci-remote: format-check check-imports vet lint security test integration e2e validate-js build docker-build deps-check
	@echo "All CI checks passed!"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -f gitvista vista gitvista-cli
	@rm -rf test/cover/
	@echo "Clean complete"

## cloc: Count lines of code
cloc:
	@echo "Counting lines of code..."
	@if command -v cloc >/dev/null; then \
		cloc .; \
	else \
		echo "cloc not found - install with: brew install cloc or apt install cloc"; \
	fi
