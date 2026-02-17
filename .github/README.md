# CI/CD Configuration

This directory contains GitHub Actions workflows for continuous integration and deployment.

## Workflows

### ci.yml - Main CI Pipeline

Runs on every push to `main` and on all pull requests.

**Jobs:**

1. **Test** (Matrix: Go 1.21, 1.22, 1.23)
   - Runs all unit tests with race detection
   - Generates coverage reports
   - Uploads coverage to Codecov (Go 1.23 only)
   - Timeout: 5 minutes

2. **Lint**
   - Runs golangci-lint with comprehensive checks
   - Configuration in `/.golangci.yml`
   - Checks: errcheck, gosec, staticcheck, govet, revive, and more
   - Timeout: 5 minutes

3. **Validate JavaScript**
   - Syntax validation for all `.js` files in `web/`
   - Checks for CommonJS/ES module mixing
   - Ensures ES6+ module syntax

4. **Build**
   - Compiles the `gitvista` binary
   - Verifies build succeeds on Ubuntu

5. **Integration Tests**
   - End-to-end server tests
   - HTTP/WebSocket communication
   - Rate limiting validation
   - Security checks (path traversal)
   - Requires: Build tag `integration`

6. **Security**
   - gosec: Static security analysis
   - govulncheck: Known vulnerability scanning
   - Non-blocking (informational)

## Badges

Add these to your main README.md:

```markdown
![CI](https://github.com/rybkr/gitvista/workflows/CI/badge.svg)
[![codecov](https://codecov.io/gh/rybkr/gitvista/branch/main/graph/badge.svg)](https://codecov.io/gh/rybkr/gitvista)
```

## Local Validation

Run the same checks locally before pushing:

```bash
# Tests with race detection
go test -race -timeout 5m ./...

# Linting (requires golangci-lint)
golangci-lint run --timeout=5m

# JavaScript validation
for file in web/*.js; do node --check "$file"; done

# Integration tests
go test -v -tags=integration ./test/integration/...

# Security scanning (requires gosec)
gosec -no-fail -fmt json -out gosec-report.json ./...
govulncheck ./...
```

## Setup Requirements

### Codecov Integration

1. Sign up at [codecov.io](https://codecov.io)
2. Add your repository
3. No token needed for public repos
4. For private repos, add `CODECOV_TOKEN` to GitHub secrets

### golangci-lint

The workflow uses the official [golangci-lint-action](https://github.com/golangci/golangci-lint-action) which automatically installs the linter.

Local installation:

```bash
# macOS
brew install golangci-lint

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Or via Go
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Security Tools

```bash
# Install gosec
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Install govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Workflow Customization

### Change Go Versions

Edit the matrix in `.github/workflows/ci.yml`:

```yaml
strategy:
  matrix:
    go-version: ['1.21', '1.22', '1.23']  # Modify as needed
```

### Adjust Timeouts

```yaml
- name: Run tests
  run: go test -v -race -timeout 5m ./...  # Change 5m as needed
```

### Skip Jobs on Certain Paths

```yaml
on:
  push:
    branches: [main]
    paths-ignore:
      - '**.md'
      - 'docs/**'
```

## Troubleshooting

### Tests Timeout

Increase timeout in workflow:

```yaml
- name: Run tests
  run: go test -v -race -timeout 10m ./...
  timeout-minutes: 15
```

### Linter Fails on Different Go Version

The linter runs on Go 1.23. If using features from newer versions, update:

```yaml
- name: Set up Go
  uses: actions/setup-go@v5
  with:
    go-version: '1.24'  # Update version
```

### Integration Tests Fail in CI

Integration tests need a Git repository. The workflow runs from repo root, which works. If issues occur:

```yaml
- name: Run integration tests
  working-directory: ${{ github.workspace }}
  run: go test -v -race -tags=integration ./test/integration/...
```

### Coverage Upload Fails

Codecov upload is non-blocking (`fail_ci_if_error: false`). Check:
- Repository is added to Codecov
- For private repos, `CODECOV_TOKEN` is set in secrets
- Coverage file exists: `test/cover/coverage.out`

## Performance

Typical run times (on GitHub hosted runners):

- Test job: ~30-60 seconds per Go version
- Lint job: ~45 seconds
- JavaScript validation: ~10 seconds
- Build: ~20 seconds
- Integration tests: ~15 seconds
- Security scan: ~30 seconds

**Total pipeline time:** ~2-3 minutes

## Future Enhancements

Potential improvements:

- [ ] Add benchmark regression detection
- [ ] Deploy to staging environment on main push
- [ ] Docker image build and publish
- [ ] Frontend E2E tests with Playwright
- [ ] Automatic dependency updates with Dependabot
- [ ] Code quality metrics (gocyclo, gocognit)
- [ ] License compliance checking
