# GitVista Development Guide

This guide covers local development setup, pre-commit hooks, and CI/CD pipeline validation.

## Quick Start

### Prerequisites

- **Go 1.26+** - [Download](https://golang.org/dl/)
- **Node.js 20+** (optional, for frontend work) - [Download](https://nodejs.org/)
- **Git** - [Download](https://git-scm.com/)
- **Docker** (optional, for containerized testing) - [Download](https://www.docker.com/)

### Initial Setup

```bash
# Clone the repository
git clone https://github.com/rybkr/gitvista.git
cd gitvista

# Install pre-commit hooks (strongly recommended)
brew install lefthook    # macOS
apt install lefthook     # Linux (Ubuntu/Debian)
# or download from: https://github.com/evilmartians/lefthook/releases

lefthook install

# Install optional linting tools (auto-used by pre-commit)
go install github.com/golang/tools/cmd/goimports@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/golang/vuln/cmd/govulncheck@latest
```

### Running the Application

```bash
# Run the main application (serves on http://localhost:8080)
go run ./cmd/vista

# Run the CLI tool
go run ./cmd/gitcli --help

# Build binaries
make build

# Run with a specific Git repository
./gitvista -repo=/path/to/git/repo
```

## Development Workflow

### Making Changes

1. **Create a feature branch:**
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make your changes:**
   ```bash
   # Edit files
   vim internal/git/parser.go
   ```

3. **Pre-commit hooks will run automatically** when you commit:
   ```bash
   git commit -m "Add feature: description"
   # Pre-commit hooks will:
   # ✓ Format code with gofmt
   # ✓ Organize imports with goimports
   # ✓ Run go vet
   # ✓ Run staticcheck
   # ✓ Check JavaScript syntax (if web files changed)
   ```

4. **If hooks fail:** Fix the issues and commit again. Hooks can auto-fix formatting and imports.

5. **Run full test suite before pushing:**
   ```bash
   make test
   make integration
   make e2e
   make lint
   ```

6. **Push to your branch:**
   ```bash
   git push origin feature/my-feature
   ```

7. **Create a PR on GitHub** - CI will run automatically

### Testing

#### Unit Tests
```bash
# Run all unit tests
make test

# Run specific test
go test -v ./internal/git/...

# Run with coverage
go test -cover ./...

# View coverage in browser
make cover-html  # opens coverage.html
```

#### Integration Tests
```bash
# Requires full Git repository context
make integration

# Run specific integration test
go test -v -tags=integration ./test/integration/...
```

#### E2E Tests
```bash
# Tests complete workflows, compares against git
make e2e

# Run specific e2e test
go test -v -tags=e2e ./test/e2e/...
```

#### All Tests
```bash
# Run everything (what CI runs)
make ci
```

### Code Quality

#### Formatting
```bash
# Auto-format all Go code
make format

# Check formatting without modifying
gofmt -l .

# Format imports
goimports -w .
```

#### Linting
```bash
# Run all configured linters
make lint

# Run specific linter
staticcheck ./...
go vet ./...
```

#### Security Scanning
```bash
# Check for known vulnerabilities
govulncheck ./...

# Scan for security issues
gosec ./internal/...
```

#### Go Module Management
```bash
# Ensure dependencies are clean and up-to-date
go mod tidy

# Verify module integrity
go mod verify

# Check for outdated dependencies
go list -u -m all
```

### Building

```bash
# Build main binary
go build -o gitvista ./cmd/vista

# Build CLI binary
go build -o gitvista-cli ./cmd/gitcli

# Build both
make build

# Build Docker image
docker build -t gitvista:latest .

# Run in Docker
docker run -p 8080:8080 -v /path/to/repo:/repo gitvista:latest
```

## Pre-commit Hooks (Lefthook)

### What Hooks Do

Pre-commit hooks run automatically before each commit to catch issues early. They're fast (~10s) and prevent committing broken code.

**Enabled Hooks:**
- ✅ **gofmt** - Format Go code
- ✅ **goimports** - Organize imports
- ✅ **go vet** - Static analysis
- ✅ **staticcheck** - Advanced linting
- ✅ **gosec** - Security scanning
- ✅ **js-syntax** - JavaScript syntax validation
- ✅ **js-commonjs** - ES module compliance

### Running Hooks Manually

```bash
# Run all pre-commit hooks
lefthook run pre-commit

# Run specific hook
lefthook run pre-commit --hook gofmt

# Skip hooks temporarily (not recommended!)
git commit --no-verify

# Check hook status
lefthook status

# View hook configuration
cat lefthook.yml
```

### Troubleshooting Hooks

**Issue: "goimports not found"**
```bash
go install github.com/golang/tools/cmd/goimports@latest
```

**Issue: "staticcheck not found"**
```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
```

**Issue: Hooks keep failing on same issue**
```bash
# Uninstall and reinstall hooks to ensure they're up to date
lefthook uninstall
lefthook install

# Or run auto-fix and commit again
make format
goimports -w .
git add .
git commit -m "Fix formatting"
```

**Issue: Commit blocked due to security issue in gosec**
```bash
# Review the gosec output carefully
gosec ./internal/...

# If the issue is a false positive, check .golangci.yml
# for gosec exclusions (e.g., G304 for file paths)

# If it's intentional, mark it with a comment
// #nosec G204 -- we intentionally use dynamic git command
```

## CI/CD Pipeline

### What GitHub Actions Checks

When you push a PR, these checks run automatically:

1. **Format** - All code must be gofmt-compliant
2. **Vet** - No suspicious Go code patterns
3. **Lint** - Passes all golangci-lint checks
4. **Security** - No known CVEs in dependencies
5. **Test** - All unit tests pass with coverage
6. **Integration** - All integration tests pass
7. **E2E** - All end-to-end tests pass
8. **Validate JavaScript** - Frontend syntax valid
9. **Build** - Binaries compile successfully
10. **Docker Build** - Docker image builds successfully
11. **Dependencies** - go.mod/go.sum are in sync

### Monitoring CI

Check CI status:
1. **In your PR:** GitHub shows check status on the PR page
2. **In Actions tab:** View detailed logs for failed checks
3. **Locally:** Run `make ci` to replicate CI environment

### Common CI Failures

#### "Format check failed"
```bash
# Auto-fix and commit
make format
git add .
git commit -m "Fix formatting"
git push
```

#### "Test failed"
```bash
# Run locally to debug
make test

# Run specific failing test
go test -v -run TestName ./...

# Run with verbosity
go test -v ./...
```

#### "Lint failed: [error name]"
```bash
# Run locally with same config
golangci-lint run

# Fix issues (most are auto-fixable)
# Then commit and push
```

#### "Security scan failed"
```bash
# Check for vulnerabilities
govulncheck ./...

# Update dependencies
go get -u ./...
go mod tidy
```

#### "Docker build failed"
```bash
# Build locally to debug
docker build .

# Check Dockerfile
cat Dockerfile

# Common issues:
# - Base image not found/accessible
# - Missing file in COPY/ADD
# - RUN command fails in container
```

## Branch Protection

The `main` branch is protected and requires:
- ✅ All CI checks to pass
- ✅ At least 1 code review
- ✅ Branch up-to-date with main

See [BRANCH_PROTECTION.md](.github/BRANCH_PROTECTION.md) for detailed settings.

## Common Tasks

### Update Dependencies
```bash
# Update all dependencies to latest compatible version
go get -u ./...

# Or update specific dependency
go get -u github.com/package/name

# Clean up go.mod/go.sum
go mod tidy

# Verify integrity
go mod verify

# Test after updating
make test
```

### Fix Import Issues

```bash
# Auto-organize imports
goimports -w .

# Or manually
# Remove: unused imports
# Add: missing imports
# Sort: alphabetically

# Verify
go vet ./...
```

### Profile Application Performance

```bash
# CPU profile
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./...
go tool pprof cpu.prof

# Memory profile
go test -memprofile=mem.prof ./...
go tool pprof mem.prof

# Benchmarks (if available)
go test -bench=. ./...
```

### Generate Coverage Report

```bash
# Generate coverage
go test -cover ./...

# Detailed coverage with HTML
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Or use make target
make cover-html
```

### Debug Tests

```bash
# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestName ./...

# Run with race detector
go test -race ./...

# With timeout
go test -timeout 10s ./...

# Combined
go test -v -race -timeout 10s -run TestName ./...
```

## IDE Setup

### VS Code

**Extensions:**
- **Go** (golang.go) - Official Go extension
- **GolangCI-Lint** (nametag.golangci-lint-plus) - Linter integration

**Settings** (.vscode/settings.json):
```json
{
  "[go]": {
    "editor.defaultFormatter": "golang.go",
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  }
}
```

### GoLand / IntelliJ IDEA

**Built-in Support:**
- Go formatting
- Goimports integration
- Golangci-lint integration

**Setup:**
1. Settings → Go → Go Modules → Enable Go modules integration
2. Settings → Languages & Frameworks → Go → Linter → Enable golangci-lint
3. Settings → Tools → Go → gofmt → Run gofmt on File save

### Vim/Neovim

**Using vim-go:**
```vim
Plugin 'fatih/vim-go'

" Auto-format on save
let g:go_fmt_on_save = 1
let g:go_fmt_command = "goimports"
```

## Troubleshooting

### Module Not Found
```bash
# Ensure go.mod references are correct
go mod graph

# Check specific module
go mod why -m module/name

# Add missing dependency
go get module/name

# Or if import issue
goimports -w .
```

### Build Fails

```bash
# Clean build cache
go clean -cache
go clean -testcache

# Rebuild
go build ./...

# Check for syntax errors
go vet ./...
```

### Test Timeouts

```bash
# Increase timeout
go test -timeout 10m ./...

# Run tests in parallel (slower for flaky tests)
go test -parallel 1 ./...

# See which tests are slow
go test -v ./... 2>&1 | grep -E "^\s+.*\s+[0-9.]+s"
```

### Git Hook Issues

```bash
# Check hook status
lefthook status

# Reinstall hooks
lefthook uninstall
lefthook install

# Debug specific hook
lefthook run pre-commit --hook gofmt -vv

# Temporarily skip hooks
git commit --no-verify

# Permanently remove hooks
lefthook uninstall
```

## Performance Tips

### Faster Tests
```bash
# Run tests in parallel
go test -parallel 4 ./...

# Skip verbose output
go test ./...

# Skip coverage for speed
go test ./...  # no -cover flag
```

### Faster Builds
```bash
# Use incremental build cache
go build ./...  # Uses go build cache

# Skip trimpath for faster local builds
go build -o gitvista ./cmd/vista
# vs. (slower, for production)
go build -trimpath -ldflags="-s -w" -o gitvista ./cmd/vista
```

### Faster Linting
```bash
# golangci-lint caches by default
golangci-lint run

# Clear cache if needed
golangci-lint cache clean

# Run specific linter
golangci-lint run --disable-all -E staticcheck
```

## Resources

- [Go Documentation](https://golang.org/doc/)
- [Go Testing](https://golang.org/pkg/testing/)
- [golangci-lint](https://golangci-lint.run/)
- [Lefthook](https://evilmartians.com/chronicles/lefthook-knock-down-your-git-pre-commit-hook)
- [GitHub Actions](https://docs.github.com/en/actions)
- [Docker Documentation](https://docs.docker.com/)

## Questions?

Check the main [README.md](README.md) or [BRANCH_PROTECTION.md](.github/BRANCH_PROTECTION.md) for more information.
