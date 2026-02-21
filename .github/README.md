# CI/CD and Pre-commit Configuration

This directory contains GitHub Actions workflows and pre-commit hook configuration for continuous integration, security, and development automation.

## Quick Links

- **For Developers:** See [DEVELOPMENT.md](../../DEVELOPMENT.md) for local setup
- **For Branch Protection:** See [BRANCH_PROTECTION.md](BRANCH_PROTECTION.md) for main branch rules
- **For Infrastructure:** This document

## Overview

The GitVista project uses:

1. **Lefthook** - Fast pre-commit framework for local checks before committing
2. **GitHub Actions CI/CD** - Automated testing, building, and security scanning
3. **Branch Protection Rules** - Enforce quality gates on main branch

## Pre-commit Hooks (Lefthook)

### What Runs Locally

When you make a commit, lefthook automatically runs checks to catch issues before they reach GitHub:

**File: `lefthook.yml`**

| Check | Files | Time | Auto-fix |
|-------|-------|------|----------|
| gofmt | `*.go` | < 1s | ✅ Yes |
| goimports | `*.go` | < 2s | ✅ Yes |
| go vet | `*.go` | < 1s | ❌ No |
| staticcheck | `*.go` | < 5s | ❌ No |
| gosec | `internal/**/*.go` | < 2s | ❌ No |
| js-syntax | `web/**/*.js` | < 1s | ❌ No |
| js-commonjs | `web/**/*.js` | < 1s | ❌ No |

**Total time:** ~10 seconds (fast enough not to interrupt workflow)

### Installation

See [DEVELOPMENT.md](../../DEVELOPMENT.md#quick-start) for setup instructions or run:

```bash
make setup-hooks
```

## GitHub Actions CI Workflow

### File: `workflows/ci.yml`

Comprehensive CI pipeline that runs on:
- Every push to `main` and `dev` branches
- All pull requests to `main`
- Automatically triggered by GitHub

### Jobs Overview

All jobs run in parallel for speed (~3-5 minutes total):

#### 1. **Format Check** (< 10s)
- Validates all Go code is gofmt-compliant
- Fails if any file needs formatting
- **Status:** Required

#### 2. **Vet** (< 30s)
- Runs `go vet` for suspicious code patterns
- Detects unused variables, pointer errors, unreachable code
- **Status:** Required

#### 3. **Lint** (< 2m)
- Runs golangci-lint with 13+ linters
- Configured in `/.golangci.yml`
- Linters: errcheck, staticcheck, gosec, revive, misspell, and more
- Only reports new issues on PRs (not on main push)
- **Status:** Required

#### 4. **Security Scan** (< 1m)
- Runs govulncheck for known CVEs in dependencies
- Uses Go's official vulnerability database
- Fails on confirmed exploitable vulnerabilities
- **Status:** Required

#### 5. **Test** (< 3m)
- Runs all unit tests with race detector
- Generates coverage report
- Uploads coverage to Codecov
- Coverage badge available in README
- **Status:** Required

#### 6. **Integration Tests** (< 3m)
- Runs integration tests with full Git context
- Tests component interactions
- Requires `integration` build tag
- **Status:** Required

#### 7. **E2E Tests** (< 3m)
- Runs end-to-end tests
- Tests complete workflows
- Compares output against git baseline
- Requires `e2e` build tag
- **Status:** Required

#### 8. **Validate JavaScript** (< 10s)
- Node.js syntax validation for all `.js` files
- Checks for CommonJS/ES module mixing
- Ensures ES module compliance
- **Status:** Required

#### 9. **Build** (< 2m)
- Compiles both binaries (gitvista and gitvista-cli)
- Verifies no missing imports
- Ensures clean compilation
- **Status:** Required

#### 10. **Docker Build** (< 3m)
- Builds production Docker image
- Uses multi-stage Dockerfile
- Caches intermediate layers
- **Status:** Required

#### 11. **Dependency Check** (< 30s)
- Verifies `go.mod`/`go.sum` are in sync
- Ensures no orphaned dependencies
- Runs `go mod tidy -check`
- **Status:** Required

#### 12. **CI Status** (instant)
- Aggregates all job results
- Master check for branch protection
- Ensures all checks passed
- **Status:** Required (master)

### Status Check Details

The **ci-status** job is the master check used in branch protection rules. It verifies that ALL other jobs passed.

Individual checks also appear in branch protection (see [BRANCH_PROTECTION.md](BRANCH_PROTECTION.md)):

```yaml
# All of these are required to be green
needs:
  - format
  - vet
  - lint
  - security
  - test
  - integration
  - e2e
  - validate-js
  - build
  - docker-build
  - dependencies
```

## Action Versions

All GitHub Actions are pinned to specific SHAs (not @latest):

```yaml
- uses: actions/checkout@b4ffde65f46336ab88eb53be0f37341b4dfc8793  # v4.1.1
- uses: actions/setup-go@cdcb36256577b078e2e2710620cd304ffbb09590    # v5.0.0
```

This prevents:
- Supply chain attacks via compromised actions
- Unexpected behavior changes
- Non-deterministic CI runs

Update actions periodically with Dependabot alerts.

## Local Development

### Install Pre-commit Hooks

```bash
# Automatic setup
make setup-hooks

# Manual setup
brew install lefthook  # macOS
apt install lefthook   # Linux
lefthook install
```

### Replicate CI Locally

```bash
# Run all CI checks (takes ~5 minutes)
make ci

# Run fast checks only
make dev-check  # format, imports, vet

# Run specific checks
make test
make lint
make integration
make build
make docker-build
```

### Make Targets Available

```bash
make help  # List all available targets
```

Key targets:

| Target | Purpose |
|--------|---------|
| `make setup-hooks` | Install pre-commit hooks |
| `make dev-check` | Quick format/vet/imports check |
| `make test` | Unit tests |
| `make cover` | Tests with coverage |
| `make cover-html` | Coverage report in browser |
| `make integration` | Integration tests |
| `make e2e` | End-to-end tests |
| `make lint` | Run linters |
| `make vet` | Static analysis |
| `make format` | Auto-format code |
| `make check-imports` | Fix import ordering |
| `make security` | Security checks |
| `make build` | Build binaries |
| `make docker-build` | Build Docker image |
| `make ci` | Run full CI suite |
| `make clean` | Clean artifacts |

## Secrets and Access

### Environment Variables

- **CODECOV_TOKEN** - Read-only token for Codecov uploads
  - Only needed for private repos
  - Public repos don't require this

### No Credentials Needed

The workflow doesn't require:
- AWS credentials
- API keys
- Database passwords
- GitHub token (except implicit GITHUB_TOKEN)

### OIDC Federated Access

For future cloud deployments, consider OIDC federation instead of long-lived credentials:

```yaml
- uses: aws-actions/configure-aws-credentials@v4
  with:
    role-to-assume: arn:aws:iam::ACCOUNT:role/GitHubActionsRole
    aws-region: us-east-1
```

## Troubleshooting

### Status Check Not Appearing

1. Wait 5 minutes for GitHub cache
2. Check workflow file is valid YAML: `yamllint .github/workflows/ci.yml`
3. Verify job names match branch protection settings exactly
4. Check Actions tab for workflow errors

### "Branch protection requires 12 checks but only 11 exist"

- A required check failed or hasn't run yet
- Branch protection config still references old job name
- Workflow file has a syntax error

**Solution:**
1. Verify latest workflow run succeeded
2. Update branch protection rules to match current job names
3. Check `.github/workflows/ci.yml` syntax

### PR Can't Merge Despite Green Checkmarks

Possible causes:
1. **"Requires up-to-date branch"** - Rebase: `git pull --rebase origin main`
2. **"Requires code review"** - Wait for reviewer
3. **"Requires conversation resolution"** - Reply to all review comments
4. **"Requires 1 approval"** - Reviewer approved but with "Request changes" flag set

### Test Timeout

Increase timeout in workflow:

```yaml
- name: Run tests
  run: go test -v -race -timeout 10m ./...
  timeout-minutes: 15
```

### Linter Fails Locally but Passes in CI

```bash
# Clear golangci-lint cache
golangci-lint cache clean

# Run with same config as CI
golangci-lint run --config=.golangci.yml

# Update linter
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Docker Build Fails

```bash
# Debug locally
docker build .

# Common issues:
# 1. Base image not available
# 2. Missing files in COPY
# 3. RUN command fails in container

# Check Dockerfile
cat Dockerfile

# Test specific stage
docker build --target build .
```

## Performance

### Typical Execution Times

Running in parallel on GitHub hosted runners:

| Job | Time |
|-----|------|
| Format Check | < 10s |
| Vet | < 30s |
| Lint | < 2m |
| Security | < 1m |
| Test | < 3m |
| Integration | < 3m |
| E2E | < 3m |
| JavaScript | < 10s |
| Build | < 2m |
| Docker | < 3m |
| Dependencies | < 30s |

**Total (parallel):** ~3-5 minutes ⚡

### Optimization Tips

1. **Cache Go modules** - Already enabled
2. **Reuse Docker layers** - Already using GitHub Actions cache
3. **Only run E2E on PRs** - Currently runs on every PR (might optimize later)
4. **Matrix builds** - Currently single Go version, could add 1.24 if needed

## Branch Protection Rules

See [BRANCH_PROTECTION.md](BRANCH_PROTECTION.md) for detailed setup.

Required settings for main:
- ✅ Require all CI checks to pass
- ✅ Require 1 code review
- ✅ Require branches up-to-date before merge
- ❌ Don't allow administrators to bypass (optional)

## Future Enhancements

Potential improvements:

- [ ] Multi-version Go testing (1.25, 1.26, 1.27)
- [ ] Benchmark regression detection
- [ ] Automated dependency updates (Dependabot)
- [ ] Container registry push (Docker Hub, GHCR)
- [ ] Automated semantic versioning
- [ ] SBOM generation
- [ ] Frontend E2E tests (Playwright)
- [ ] Code quality metrics (gocyclo, gocognit)
- [ ] License compliance checking
- [ ] Automatic changelog generation

## References

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Lefthook Documentation](https://evilmartians.com/chronicles/lefthook-knock-down-your-git-pre-commit-hook)
- [Go Testing Best Practices](https://golang.org/doc/effective_go#testing)
- [golangci-lint Linters](https://golangci-lint.run/usage/linters/)
- [Codecov Documentation](https://docs.codecov.io/)
- [GitHub Branch Protection](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)

## Need Help?

1. Check [DEVELOPMENT.md](../../DEVELOPMENT.md) for local development setup
2. See [BRANCH_PROTECTION.md](BRANCH_PROTECTION.md) for branch rules
3. Review workflow file: `.github/workflows/ci.yml`
4. Check Actions tab in GitHub for failed workflow details
5. Run `make help` for available targets
