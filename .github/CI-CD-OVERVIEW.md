# GitVista CI/CD Architecture Overview

This document provides a high-level overview of the GitVista CI/CD infrastructure and how all components work together.

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Developer Workflow                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Edit Code         2. Git Commit       3. Push to GitHub     │
│     ↓                    ↓                   ↓                  │
│  ├─ internal/         ├─ Lefthook runs   ├─ Push to branch    │
│  ├─ cmd/              │  (local checks)   └─ Create PR         │
│  └─ web/              │                                        │
│                       └─ Auto-fixes:     GitHub Actions       │
│                          • gofmt         CI Pipeline           │
│                          • goimports      Starts               │
│                          • js syntax      (parallel jobs)       │
│                                                               │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│              GitHub Actions CI Pipeline                         │
│                    (3-5 minutes total)                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │   Format     │  │     Vet      │  │     Lint     │         │
│  │  Check       │  │              │  │              │         │
│  │   (gofmt)    │  │   (go vet)   │  │ (golangci-   │         │
│  │              │  │              │  │  lint)       │         │
│  │  ~10s        │  │   ~30s       │  │   ~2min      │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │  Security    │  │    Tests     │  │ Integration  │         │
│  │   Scan       │  │              │  │   Tests      │         │
│  │              │  │  (unit tests)│  │              │         │
│  │ (govulncheck)│  │   (race det.)│  │  (git ctx)   │         │
│  │              │  │              │  │              │         │
│  │   ~1min      │  │   ~3min      │  │   ~3min      │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │    E2E       │  │  Validate    │  │    Build     │         │
│  │   Tests      │  │  JavaScript  │  │              │         │
│  │              │  │              │  │  (binaries)  │         │
│  │ (workflows)  │  │  (syntax,    │  │              │         │
│  │              │  │   modules)   │  │   ~2min      │         │
│  │   ~3min      │  │              │  │              │         │
│  │              │  │   ~10s       │  │              │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │   Docker     │  │    Deps      │  │  CI Status   │         │
│  │   Build      │  │   Check      │  │              │         │
│  │              │  │              │  │  (aggregator)│         │
│  │  (image)     │  │ (go mod tidy)│  │              │         │
│  │              │  │              │  │   <1s        │         │
│  │   ~3min      │  │   ~30s       │  │              │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│           Branch Protection Status Check                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ALL CHECKS MUST PASS:                                         │
│  ✅ Format     ✅ Vet       ✅ Lint       ✅ Security           │
│  ✅ Tests      ✅ Integration  ✅ E2E    ✅ JS Validation       │
│  ✅ Build      ✅ Docker    ✅ Dependencies  ✅ Status          │
│                                                                 │
│  Result: ✅ PASS → Can Merge                                   │
│  Result: ❌ FAIL → Cannot Merge (fix and re-push)             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│              Code Review & Approval                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Requires:                                                     │
│  1. ✅ Minimum 1 code reviewer approval                        │
│  2. ✅ All conversations resolved                              │
│  3. ✅ Branch up-to-date with main                             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│              Merge to Main Branch                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Action: Squash merge (or rebase)                              │
│  Result: Code enters main branch                               │
│  Effect: Production-ready code (all checks passed)             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Component Details

### 1. Pre-commit Hooks (Lefthook)

**File:** `lefthook.yml`

**Purpose:** Catch issues before they reach GitHub

**Runs on:** Every `git commit`

**Checks:**
- gofmt - Code formatting
- goimports - Import organization
- go vet - Syntax errors
- staticcheck - Code quality
- gosec - Security issues
- js-syntax - JavaScript validation
- js-commonjs - Module compliance

**Characteristics:**
- Fast (~10 seconds)
- Runs locally (developer machine)
- Auto-fixes formatting and imports
- Optional (can be skipped with --no-verify)
- Installed via: `make setup-hooks`

**Why Lefthook?**
- Fast - only checks changed files
- Language agnostic - can extend beyond Go
- Easy to skip when needed (not opinionated)
- Simple configuration format

### 2. GitHub Actions CI Pipeline

**File:** `.github/workflows/ci.yml`

**Purpose:** Automated testing, building, and security scanning on every PR and push

**Triggers:**
- Every push to `main` or `dev`
- All PRs to `main`
- Manual trigger (on demand)

**Environment:**
- Ubuntu 20.04 runners (GitHub hosted)
- Go 1.26 (can extend to multiple versions)
- Node.js 20 (for JavaScript validation)

**Jobs:** 12 parallel jobs (see architecture diagram)

**Duration:** ~3-5 minutes total

**Key Features:**
- Concurrent job execution for speed
- Automatic caching of Go modules
- Pinned action versions (no @latest)
- Security-focused permissions model
- Detailed failure logging

### 3. Branch Protection Rules

**File:** `.github/BRANCH_PROTECTION.md`

**Purpose:** Enforce quality gates before code reaches main

**Settings:**
- Require all CI checks to pass
- Require 1 code review approval
- Require branches up-to-date with main
- Dismiss stale approvals on new commits

**Prevents:**
- Broken code merging to main
- Bypassing reviews (except admin emergency)
- Outdated branches causing issues

## Data Flow

### On Commit

```
Developer edits code
  ↓
git commit -m "message"
  ↓
Lefthook triggers pre-commit hooks
  ├─ Check formatting (gofmt)
  ├─ Organize imports (goimports)
  ├─ Static analysis (go vet)
  └─ Security scan (gosec)
  ↓
Hooks pass? → Commit created
Hooks fail? → Commit blocked, developer fixes issues
```

### On Push/PR

```
Developer pushes branch
  ↓
Create Pull Request
  ↓
GitHub Actions triggered
  ├─ Job 1: Format check (gofmt)
  ├─ Job 2: Vet (go vet)
  ├─ Job 3: Lint (golangci-lint)
  ├─ Job 4: Security (govulncheck)
  ├─ Job 5: Tests (go test)
  ├─ Job 6: Integration (go test -tags=integration)
  ├─ Job 7: E2E (go test -tags=e2e)
  ├─ Job 8: JavaScript (node --check)
  ├─ Job 9: Build (go build)
  ├─ Job 10: Docker (docker build)
  ├─ Job 11: Dependencies (go mod tidy -check)
  └─ Job 12: CI Status (aggregator)
  ↓
All jobs complete
  ├─ All pass? → PR ready for review (green checkmark)
  └─ Any fail? → PR blocked (red X, shows which jobs failed)
  ↓
PR requires code review (independent of CI)
  ├─ Review checks: code quality, design, correctness
  ├─ Conversations must be resolved
  └─ At least 1 approval required
  ↓
Merge button available only if:
  ✅ All CI checks passing
  ✅ Code reviewed and approved
  ✅ Branch up-to-date with main
  ↓
Merge PR → Code enters main branch
```

## Critical Checks Explained

### Format Check (gofmt)
- **Why:** Consistent code style aids review and reduces noise
- **What:** Validates all `.go` files match Go standard formatting
- **How:** `gofmt -l . | grep -v vendor`
- **Recovery:** `make format` then commit again

### Vet (go vet)
- **Why:** Catches suspicious Go patterns before they cause bugs
- **What:** Detects pointer errors, unused variables, unreachable code
- **How:** `go vet ./...`
- **Recovery:** Review error, fix code, re-commit

### Lint (golangci-lint)
- **Why:** Enforces best practices and security patterns
- **What:** Runs 13 linters (errcheck, staticcheck, gosec, etc.)
- **How:** `golangci-lint run --config=.golangci.yml`
- **Recovery:** Review error, fix code, re-commit

### Security (govulncheck)
- **Why:** Prevents known CVE vulnerabilities from being deployed
- **What:** Checks Go dependencies against vulnerability database
- **How:** `govulncheck ./...`
- **Recovery:** Update dependency or patch vulnerability

### Tests (go test)
- **Why:** Ensures code functionality is correct
- **What:** Runs all unit tests with race detector
- **How:** `go test -v -race -cover ./...`
- **Recovery:** Fix test failures or implement missing features

### Integration Tests (go test -tags=integration)
- **Why:** Verifies components work together correctly
- **What:** Tests with real file I/O and Git repositories
- **How:** `go test -tags=integration ./test/integration/...`
- **Recovery:** Fix integration issues or component boundaries

### E2E Tests (go test -tags=e2e)
- **Why:** Validates complete user workflows work end-to-end
- **What:** Tests full workflows against real Git commands
- **How:** `go test -tags=e2e ./test/e2e/...`
- **Recovery:** Fix workflow or update test expectations

### Build
- **Why:** Ensures code compiles without errors
- **What:** Compiles both gitvista and gitvista-cli binaries
- **How:** `go build -v ./cmd/vista` and `./cmd/gitcli`
- **Recovery:** Fix compilation errors (missing imports, syntax)

### Docker Build
- **Why:** Validates containerized version builds correctly
- **What:** Builds production Docker image
- **How:** `docker build .` with multi-stage Dockerfile
- **Recovery:** Fix Dockerfile, missing files, or container environment

### Dependencies (go mod tidy -check)
- **Why:** Ensures go.mod and go.sum stay synchronized
- **What:** Validates module files are clean
- **How:** `go mod tidy -check && go mod verify`
- **Recovery:** `go mod tidy` then commit

## Performance Characteristics

### Local (Developer Machine)

**Lefthook pre-commit:** ~10 seconds
- Only checks modified files
- Parallelizes where possible
- Auto-fixes what it can

**Full test suite:** ~1-2 minutes
```bash
make test          # ~30-60s
make integration   # ~30-60s
make e2e           # ~30-60s
make lint          # ~30s
```

### CI (GitHub Actions)

**Total pipeline time:** ~3-5 minutes

- All jobs run in parallel (not sequential)
- Caching speeds up module downloads
- No local I/O contention

**Breakdown:**
- Format, Vet: < 1 minute (minimal work)
- Lint, Security, Build, Docker, Deps: 1-3 minutes each
- Tests (unit, integration, E2E): up to 3 minutes each
- JavaScript validation: < 10 seconds
- CI Status aggregator: < 1 second

**Optimization opportunities:**
- Only run E2E on main branch (not all PRs)
- Cache Docker layers more aggressively
- Parallel matrix testing across Go versions

## Security Considerations

### No Secrets Needed

The pipeline requires NO secrets:
- No AWS credentials
- No API keys
- No database passwords
- No GitHub tokens (implicit GITHUB_TOKEN provided)

### Only Secret: Codecov Token

Optional `CODECOV_TOKEN` for private repos:
- Public repos: token not needed (inferred)
- Private repos: add token to GitHub secrets
- Used only for coverage upload (non-sensitive)

### Action Security

All GitHub Actions are pinned to specific commit SHAs:

```yaml
uses: actions/checkout@b4ffde65f46336ab88eb53be0f37341b4dfc8793  # v4.1.1
```

**Benefits:**
- Prevents supply chain attacks
- Audit trail of which versions were used
- Reproducible builds

**Update strategy:**
- Review updates quarterly
- Use Dependabot alerts for action updates
- Test updates in dev branch first

### Permissions

Minimal permissions granted:

```yaml
permissions:
  contents: read              # Read-only repo access
  security-events: write      # SARIF upload (security reports)
  checks: write               # Report test results
```

**Not granted:**
- Write access to code
- Delete access to anything
- Admin rights
- Secret access

## Scaling and Customization

### Adding Go Versions

To test against multiple Go versions:

```yaml
strategy:
  matrix:
    go-version: ['1.25', '1.26', '1.27']

steps:
  - uses: actions/setup-go@...
    with:
      go-version: ${{ matrix.go-version }}
```

### Extending Linters

Add linters to `.golangci.yml`:

```yaml
linters:
  enable:
    - gocyclo
    - gocognit
    - nolint
```

### Adding Coverage Thresholds

Fail if coverage drops below threshold:

```yaml
- name: Check coverage
  run: |
    coverage=$(go test -cover ./... | tail -1 | awk '{print $NF}')
    if [ $(echo "$coverage < 80%" | bc) -eq 1 ]; then
      echo "Coverage below 80%: $coverage"
      exit 1
    fi
```

### Conditional Job Execution

Run jobs only on certain conditions:

```yaml
publish:
  if: github.event_name == 'push' && github.ref == 'refs/heads/main'
  runs-on: ubuntu-latest
  steps: ...
```

## Troubleshooting Decision Tree

```
PR shows red X (failed check)
  ↓
Which job failed? (Check GitHub Actions tab)
  ├─ Format Check
  │  ↓
  │  Fix: make format && git add . && git commit -m "fmt"
  │
  ├─ Vet / Lint
  │  ↓
  │  Fix: Review error, edit code, commit
  │
  ├─ Security
  │  ↓
  │  Fix: Update dependency or patch vulnerability
  │
  ├─ Test / Integration / E2E
  │  ↓
  │  Fix: Run locally (make test), fix failing test, commit
  │
  ├─ Build
  │  ↓
  │  Fix: Check for missing imports, syntax errors
  │
  ├─ Docker Build
  │  ↓
  │  Fix: Test locally (docker build .), review Dockerfile
  │
  └─ Dependencies
     ↓
     Fix: go mod tidy && git add . && git commit -m "deps"

Cannot merge despite green checks
  ↓
Check branch protection rules page
  ├─ Needs 1 approval? → Wait for reviewer
  ├─ Needs rebase? → git pull --rebase origin main
  ├─ Unresolved conversations? → Reply to all comments
  └─ Requires up-to-date branch? → Rebase and push again
```

## Monitoring and Observability

### GitHub Dashboard

1. **PR page:** See all check statuses and details
2. **Actions tab:** Full workflow run logs
3. **Branch protection:** See why merge is blocked
4. **Insights → Checks:** Track check pass rates over time

### CI Status Badge

Add to README.md:

```markdown
[![CI](https://github.com/rybkr/gitvista/workflows/CI/badge.svg)](https://github.com/rybkr/gitvista/actions)
```

### Coverage Badge

Add to README.md:

```markdown
[![codecov](https://codecov.io/gh/rybkr/gitvista/branch/main/graph/badge.svg)](https://codecov.io/gh/rybkr/gitvista)
```

## Related Documentation

- [DEVELOPMENT.md](../../DEVELOPMENT.md) - Local development setup
- [BRANCH_PROTECTION.md](BRANCH_PROTECTION.md) - Branch protection rules
- [.github/README.md](README.md) - Workflow documentation
- [lefthook.yml](../../lefthook.yml) - Pre-commit hook configuration
- [.github/workflows/ci.yml](workflows/ci.yml) - CI workflow source

## Next Steps

1. **For developers:** Read [DEVELOPMENT.md](../../DEVELOPMENT.md)
2. **For repository admin:** Configure [BRANCH_PROTECTION.md](BRANCH_PROTECTION.md)
3. **For CI enhancements:** Review [.github/workflows/ci.yml](workflows/ci.yml)
4. **For troubleshooting:** Check [.github/README.md](README.md#troubleshooting)
