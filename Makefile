.PHONY: help \
	test unit e2e test-js validate-js \
	fmt fmt-check vet lint security \
	build build-site run-site profile \
	ci-local ci-remote \
	deploy clean cloc docker-build deps-check \
	setup-test-repos docs-api

GOCMD = go
GOTEST = $(GOCMD) test
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOCACHE ?= $(CURDIR)/.cache/go-build
GOLANGCI_LINT_CACHE ?= $(CURDIR)/.cache/golangci-lint

PYTHON = uv run python
PYTEST = uv run pytest

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)
PROFILE ?= /tmp/gitvista-cli.cpu.prof
MEMPROFILE ?= /tmp/gitvista-cli.mem.prof
PPROF_WEB ?= 0
SECURITY_BEST_EFFORT ?= 0
DEPLOY_ENV ?= staging
SITE_ARGS ?=

.DEFAULT_GOAL := help

##@ Help
## help: Display this informational message
help:
	@printf "  GitVista Make Targets  \n"
	@printf "=========================\n"
	@awk ' \
		function flush() { \
			if (target == "") return; \
			line = sprintf("  %-16s %s", target, desc); \
			if (options != "") line = line " [" options "]"; \
			printf "%s\n", line; \
			target = ""; \
			desc = ""; \
			options = ""; \
		} \
		/^##@/ {next} \
		/^##   options:/ {pending_options = substr($$0, 15); next} \
		/^## / { \
			line = substr($$0, 4); \
			split(line, parts, ": "); \
			pending_desc = substr(line, length(parts[1]) + 3); \
			next; \
		} \
		/^[a-zA-Z0-9_.-]+:[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=/ {next} \
		/^\.(PHONY|DEFAULT_GOAL)/ {next} \
		/^[a-zA-Z0-9_.-]+:/ { \
			flush(); \
			split($$1, parts, ":"); \
			target = parts[1]; \
			desc = pending_desc; \
			options = pending_options; \
			pending_desc = ""; \
			pending_options = ""; \
			next; \
		} \
		END {flush()} \
	' $(MAKEFILE_LIST)

##@ Test
## test: Run all tests (unit, e2e, JavaScript)
test: unit e2e test-js
	@echo "All tests passed!"

## unit: Run unit tests
unit:
	$(GOTEST) -v -race -cover -timeout=60s ./...

## e2e: Run end-to-end tests
e2e: setup-test-repos
	$(PYTEST) -vs -n auto

## test-js: Run JavaScript unit tests
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
	@if find web -path 'web/vendor' -prune -o -name '*.js' -type f -print | xargs grep -n "module.exports\|require(" | grep -v "// @allow-commonjs"; then \
		echo "Found CommonJS syntax in ES modules"; \
		exit 1; \
	fi
	@echo "JavaScript validation passed"

##@ Code Quality
## fmt: Format Go source and organize imports
fmt:
	@echo "Formatting Go files with gofmt..."
	@find . -name '*.go' \
		-not -path './vendor/*' \
		-not -path './web/*' \
		-not -path './.cache/*' \
		-print0 | xargs -0 gofmt -w
	@echo "Formatting imports with goimports..."
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

## fmt-check: Verify Go formatting and imports
fmt-check:
	@echo "Checking gofmt compliance..."
	@UNFORMATTED=$$(find . -name '*.go' \
		-not -path './vendor/*' \
		-not -path './web/*' \
		-not -path './.cache/*' \
		-print0 | xargs -0 gofmt -l || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Files not formatted:"; echo "$$UNFORMATTED"; \
		echo "Run 'make fmt' to fix"; exit 1; \
	fi
	@echo "Checking import formatting with goimports..."
	@if command -v goimports >/dev/null; then \
		UNFORMATTED=$$(find . -name '*.go' \
			-not -path './vendor/*' \
			-not -path './web/*' \
			-not -path './.cache/*' \
			-print0 | xargs -0 goimports -l || true); \
		if [ -n "$$UNFORMATTED" ]; then \
			echo "Files with import/style issues:"; echo "$$UNFORMATTED"; \
			echo "Run 'make fmt' to fix"; exit 1; \
		fi; \
	else \
		echo "goimports not found - install with: go install golang.org/x/tools/cmd/goimports@latest"; \
		exit 1; \
	fi
	@echo "All Go files properly formatted"

## vet: Run go vet static analysis
vet:
	@echo "Running go vet..."
	@$(GOCMD) vet ./...

## lint: Run golangci-lint
lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null; then \
		mkdir -p "$(GOCACHE)" "$(GOLANGCI_LINT_CACHE)"; \
		GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run --config=.golangci.yml .; \
	else \
		echo "golangci-lint not found - install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## security: Run security checks
##   options: SECURITY_BEST_EFFORT=0|1
security:
	@echo "Checking for known vulnerabilities..."
	@if command -v govulncheck >/dev/null; then \
		if ! govulncheck ./...; then \
			if [ "$(SECURITY_BEST_EFFORT)" = "1" ]; then \
				echo "Warning: govulncheck failed, continuing because SECURITY_BEST_EFFORT=1"; \
			else \
				exit 1; \
			fi; \
		fi; \
	elif [ "$(SECURITY_BEST_EFFORT)" = "1" ]; then \
		echo "Warning: govulncheck not found - skipping because SECURITY_BEST_EFFORT=1"; \
	else \
		echo "govulncheck not found - install with: go install github.com/golang/vuln/cmd/govulncheck@latest"; \
		exit 1; \
	fi
	@echo "Running gosec security scanner..."
	@if command -v gosec >/dev/null; then \
		gosec -quiet -exclude=G304,G204 ./...; \
	else \
		echo "gosec not found - install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
		exit 1; \
	fi

##@ Build
## build: Build local binaries
##   options: VERSION=..., COMMIT=..., BUILD_DATE=...
build:
	@echo "Building CLI binary..."
	$(GOBUILD) -v -ldflags "$(LDFLAGS)" -o cli ./cmd/cli
	@echo "Building main binary..."
	$(GOBUILD) -v -ldflags "$(LDFLAGS)" -o vista ./cmd/vista

## docs-api: Generate embedded API docs with doc2go
docs-api:
	./scripts/generate_doc2go_embed.sh

##@ Profiling
## profile: Capture repo profiles
##   options: REPO=... (required), PROFILE=..., MEMPROFILE=..., PPROF_WEB=0|1
profile:
	@if [ -z "$(REPO)" ]; then \
		echo "REPO is required"; \
		echo "Usage: make profile REPO=/path/to/repo [PROFILE=/tmp/gitvista-cli.cpu.prof] [MEMPROFILE=/tmp/gitvista-cli.mem.prof] [PPROF_WEB=1]"; \
		exit 1; \
	fi
	@echo "Profiling repository load for $(REPO)"
	$(GOCMD) run ./cmd/cli --repo "$(REPO)" --cpuprofile "$(PROFILE)" --memprofile "$(MEMPROFILE)" repo
	@echo "CPU profile written to $(PROFILE)"
	@echo "Memory profile written to $(MEMPROFILE)"
	@if [ "$(PPROF_WEB)" = "1" ]; then \
		echo "Opening CPU profile web UI at http://127.0.0.1:9090"; \
		echo "Opening memory profile web UI at http://127.0.0.1:9091"; \
		trap 'kill 0' INT TERM EXIT; \
			$(GOCMD) tool pprof -http=:9090 "$(PROFILE)" & \
			$(GOCMD) tool pprof -http=:9091 "$(MEMPROFILE)" & \
			wait; \
	else \
		echo "Inspect CPU profile with: $(GOCMD) tool pprof -http=:9090 $(PROFILE)"; \
		echo "Inspect memory profile with: $(GOCMD) tool pprof -http=:9091 $(MEMPROFILE)"; \
	fi

##@ CI
## ci-local: Run local CI checks
ci-local: SECURITY_BEST_EFFORT = 1
ci-local: fmt-check vet lint security test validate-js build
	@echo "All local CI checks passed!"

## ci-remote: Run full CI checks
ci-remote: fmt-check vet lint security test validate-js build docker-build deps-check
	@echo "All CI checks passed!"

##@ Maintenance
## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -f vista cli gitvista-site
	@rm -rf test/cover/
	@echo "Clean complete"

## cloc: Count lines of code
cloc:
	@echo "Counting lines of code..."
	@if command -v cloc >/dev/null; then \
		cloc . --fullpath --exclude-dir=.venv,testdata --not-match-d="web/vendor"; \
	else \
		echo "cloc not found - install with: brew install cloc or apt install cloc"; \
	fi

##@ Deployment
## deploy: Deploy to Fly.io
##   options: DEPLOY_ENV=staging|production (default staging)
deploy:
	@case "$(DEPLOY_ENV)" in \
		staging) \
			echo "Deploying to staging..."; \
			flyctl deploy --config fly.staging.toml --app gitvista-staging ;; \
		production) \
			echo "Deploying to production..."; \
			flyctl deploy --app gitvista ;; \
		*) \
			echo "DEPLOY_ENV must be 'staging' or 'production'"; \
			exit 1 ;; \
	esac

##@ Internal
## docker-build: Build the Docker image used by full CI
docker-build:
	@echo "Building Docker image..."
	@docker build -t gitvista:latest .

## deps-check: Verify module downloads and tidy state
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

## setup-test-repos: Download test git repositories
setup-test-repos:
	./scripts/prepare_test_repos.py
