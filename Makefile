.PHONY: unit test ci ci-local lint integration e2e build build-cli build-site run-site clean help \
         profile profile-web \
         format format-check vet security security-local validate-js test-js cover cover-html dev-check check-imports \
         imports-check check-vuln docker-build deps-check deploy-staging deploy-production

GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)
PROFILE ?= /tmp/gitvista-cli.cpu.prof
MEMPROFILE ?= /tmp/gitvista-cli.mem.prof

.DEFAULT_GOAL := help

## help: Display this informational message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## unit: Run unit tests
unit:
	$(GOTEST) -v -race -cover -timeout=60s ./...

## test: Run all tests (unit, integration, e2e, JavaScript)
test: unit integration e2e test-js
	@echo "All tests passed!"

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

## e2e: Run end-to-end tests (builds cli, compares output against git)
e2e:
	$(GOTEST) -v -race -tags=e2e -timeout=60s ./test/e2e/...

## format: Auto-format the source code with gofmt
format:
	@echo "Formatting code with gofmt..."
	@find . -name '*.go' \
		-not -path './vendor/*' \
		-not -path './web/*' \
		-not -path './.cache/*' \
		-print0 | xargs -0 gofmt -w

## check-imports: Organize and format imports with goimports
check-imports:
	@echo "Checking and fixing imports..."
	@if command -v goimports >/dev/null; then \
		find . -name '*.go' \
			-not -path './vendor/*' \
			-not -path './web/*' \
			-not -path './.cache/*' \
			-print0 | xargs -0 goimports -w; \
	else \
		echo "goimports not found - install with: go install golang.org/x/tools/cmd/goimports@latest"; \
		exit 1; \
	fi

## imports-check: Verify imports are organized (fails if goimports would change files)
imports-check:
	@echo "Checking import formatting with goimports..."
	@if command -v goimports >/dev/null; then \
		UNFORMATTED=$$(find . -name '*.go' \
			-not -path './vendor/*' \
			-not -path './web/*' \
			-not -path './.cache/*' \
			-print0 | xargs -0 goimports -l || true); \
		if [ -n "$$UNFORMATTED" ]; then \
			echo "Files with import/style issues:"; echo "$$UNFORMATTED"; \
			echo "Run 'make check-imports' to fix"; exit 1; \
		fi; \
	else \
		echo "goimports not found - install with: go install golang.org/x/tools/cmd/goimports@latest"; \
		exit 1; \
	fi
	@echo "All imports properly formatted"

## vet: Run go vet static analysis
vet:
	@echo "Running go vet..."
	@$(GOCMD) vet ./...

## security: Run security checks (govulncheck, gosec)
security: check-vuln
	@echo "Running gosec security scanner..."
	@if command -v gosec >/dev/null; then \
		gosec -quiet -exclude=G304,G204 ./internal/...; \
	else \
		echo "gosec not found - install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
		exit 1; \
	fi

## security-local: Run local-friendly security checks (best-effort govulncheck + gosec)
security-local:
	@echo "Checking for known vulnerabilities (best effort, offline-safe)..."
	@if command -v govulncheck >/dev/null; then \
		if ! govulncheck ./...; then \
			echo "Warning: govulncheck failed (likely offline), continuing for ci-local"; \
		fi; \
	else \
		echo "Warning: govulncheck not found - skipping for ci-local"; \
	fi
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
		mkdir -p "$(GOCACHE)" "$(GOLANGCI_LINT_CACHE)"; \
		golangci-lint run --config=.golangci.yml .; \
	else \
		echo "golangci-lint not found - install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## test-js: Run JavaScript unit tests (Node.js test runner)
test-js:
	@echo "Running JavaScript tests..."
	@TEST_FILES=$$(find web -type f -name '*.test.js' | sort); \
	if [ -z "$$TEST_FILES" ]; then \
		echo "Could not find JavaScript test files under web/"; \
		exit 1; \
	fi; \
	node --test $$TEST_FILES

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

## build: Build the local binaries (vista and cli)
build: build-cli
	@echo "Building main binary..."
	$(GOBUILD) -v -ldflags "$(LDFLAGS)" -o vista ./cmd/vista

## build-cli: Build the cli binary
build-cli:
	@echo "Building CLI binary..."
	$(GOBUILD) -v -ldflags "$(LDFLAGS)" -o cli ./cmd/cli

## build-site: Build the hosted site binary
build-site:
	@echo "Building hosted site binary..."
	$(GOBUILD) -v -ldflags "$(LDFLAGS)" -o gitvista-site ./cmd/site

## run-site: Run the hosted site entrypoint directly from cmd/site
run-site:
	@echo "Running hosted site from cmd/site..."
	$(GOCMD) run ./cmd/site

## profile: Capture CPU and memory profiles for repository loading via cmd/cli repo (usage: make profile REPO=/path/to/repo [PROFILE=/tmp/gitvista-cli.cpu.prof] [MEMPROFILE=/tmp/gitvista-cli.mem.prof])
profile:
	@if [ -z "$(REPO)" ]; then \
		echo "REPO is required"; \
		echo "Usage: make profile REPO=/path/to/repo [PROFILE=/tmp/gitvista-cli.cpu.prof] [MEMPROFILE=/tmp/gitvista-cli.mem.prof]"; \
		exit 1; \
	fi
	@echo "Profiling repository load for $(REPO)"
	$(GOCMD) run ./cmd/cli --repo "$(REPO)" --cpuprofile "$(PROFILE)" --memprofile "$(MEMPROFILE)" repo
	@echo "CPU profile written to $(PROFILE)"
	@echo "Inspect CPU profile with: $(GOCMD) tool pprof -http=:9090 $(PROFILE)"
	@echo "Memory profile written to $(MEMPROFILE)"
	@echo "Inspect memory profile with: $(GOCMD) tool pprof -http=:9091 $(MEMPROFILE)"

## profile-web: Capture a CPU profile and open it in the pprof web UI (usage: make profile-web REPO=/path/to/repo [PROFILE=/tmp/gitvista-cli.cpu.prof])
profile-web:
	@if [ -z "$(REPO)" ]; then \
		echo "REPO is required"; \
		echo "Usage: make profile-web REPO=/path/to/repo [PROFILE=/tmp/gitvista-cli.cpu.prof]"; \
		exit 1; \
	fi
	@echo "Profiling repository load for $(REPO)"
	$(GOCMD) run ./cmd/cli --repo "$(REPO)" --cpuprofile "$(PROFILE)" repo
	@echo "Opening pprof web UI for $(PROFILE)"
	go tool pprof -http=:0 "$(PROFILE)"

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t gitvista:latest .

## deps-check: Check and tidy Go dependencies
deps-check:
	@echo "Checking dependencies..."
	@go mod download
	@go mod verify
	@if go mod tidy -diff; then \
		echo "Dependencies are clean"; \
	else \
		echo "Dependencies need tidying - run 'go mod tidy'"; \
		exit 1; \
	fi

## dev-check: Run fast local checks (format, imports, vet) - suitable for CI pre-checks
dev-check: format-check imports-check vet
	@echo "Running development checks..."

## format-check: Verify all Go files are properly formatted (fails if not)
format-check:
	@echo "Checking gofmt compliance..."
	@UNFORMATTED=$$(find . -name '*.go' \
		-not -path './vendor/*' \
		-not -path './web/*' \
		-not -path './.cache/*' \
		-print0 | xargs -0 gofmt -l || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Files not formatted:"; echo "$$UNFORMATTED"; \
		echo "Run 'make format' to fix"; exit 1; \
	fi
	@echo "All files properly formatted"

## ci-local: Run CI checks that work offline (no Docker or network needed)
ci-local: format-check imports-check vet lint security-local test validate-js build
	@echo "All local CI checks passed!"

## ci-remote: Run all CI checks including Docker build and dependency verification
ci-remote: format-check imports-check vet lint security test validate-js build docker-build deps-check
	@echo "All CI checks passed!"

## ci: Alias for full CI suite
ci: ci-remote

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -f vista cli gitvista-site
	@rm -rf test/cover/
	@echo "Clean complete"

## deploy-staging: Deploy to Fly.io staging environment
deploy-staging:
	@echo "Deploying to staging..."
	flyctl deploy --config fly.staging.toml --app gitvista-staging

## deploy-production: Deploy to Fly.io production environment
deploy-production:
	@echo "Deploying to production..."
	flyctl deploy --app gitvista

## cloc: Count lines of code
cloc:
	@echo "Counting lines of code..."
	@if command -v cloc >/dev/null; then \
		cloc . --fullpath --not-match-d="web/vendor"; \
	else \
		echo "cloc not found - install with: brew install cloc or apt install cloc"; \
	fi
