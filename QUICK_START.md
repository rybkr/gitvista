# GitVista Quick Start Guide

Fast reference for common development tasks.

## First Time Setup (5 minutes)

```bash
# 1. Install pre-commit hooks
make setup-hooks

# 2. Verify installation
lefthook status

# Done! Hooks run automatically on every commit
```

## Daily Workflow

### Making Changes

```bash
# 1. Create feature branch
git checkout -b feature/my-feature

# 2. Edit files
vim internal/git/parser.go

# 3. Commit (pre-commit hooks run automatically)
git commit -m "Add feature: description"

# 4. If hooks fail: fix issues and commit again
# (gofmt/goimports auto-fix formatting and imports)

# 5. Push to GitHub
git push origin feature/my-feature

# 6. Create PR on GitHub
# CI will run all checks automatically
```

## Testing Locally

### Quick Checks (before commit)
```bash
make dev-check  # ~10s - format, imports, vet
```

### Before Pushing
```bash
make test       # Unit tests
make lint       # Linters
make build      # Build binaries
```

### Full CI Suite (replicates GitHub)
```bash
make ci-local   # ~5 minutes - all checks (no Docker needed)
```

### View Test Coverage
```bash
make cover-html # Opens coverage report in browser
```

## Common Commands

| Task | Command | Time |
|------|---------|------|
| Format code | `make format` | ~10s |
| Fix imports | `make check-imports` | ~10s |
| Run tests | `make test` | ~1m |
| Check linters | `make lint` | ~30s |
| Security scan | `make security` | ~2m |
| Build app | `make build` | ~30s |
| Build Docker | `make docker-build` | ~3m |
| All local checks | `make ci-local` | ~5m |
| See all targets | `make help` | instant |

## Pre-commit Hooks

### What They Do

Run automatically on every `git commit`:
- ✅ gofmt - Format code
- ✅ goimports - Organize imports
- ✅ go vet - Static analysis
- ✅ staticcheck - Code quality
- ✅ gosec - Security checks

### If Hooks Fail

```bash
# Most failures auto-fix (gofmt, goimports)
# Just commit again after they auto-correct

# If manual fix needed:
make format
make check-imports
git add .
git commit -m "Fix issues"
```

## Debugging Tests

```bash
# Run specific test
go test -v -run TestName ./...

# Run with race detector (catches concurrency bugs)
go test -race ./...

# Run with timeout
go test -timeout 10s ./...

# Run single package
go test ./internal/git

# Combined
go test -v -race -timeout 10s -run TestName ./...
```

## GitHub PR Checklist

Before creating a PR, run:
```bash
make ci-local  # All checks must pass locally first
```

On GitHub:
- ✅ Title is descriptive
- ✅ Description explains why (not what)
- ✅ All 12 checks show green ✓
- ✅ Request 1 reviewer
- ✅ Resolve any comments

## IDE Setup

### VS Code
1. Install "Go" extension (golang.go)
2. Restart VS Code
3. Set format-on-save: `Cmd+,` → search "gofmt" → enable
4. Run: `goimports` on save for imports

### GoLand / IntelliJ
- Built-in Go support
- Enable gofmt in Settings → Tools → Go
- Enable golangci-lint integration

## Troubleshooting

### "Hook failed: gofmt"
```bash
# Auto-fix and recommit
make format
git add .
git commit --amend --no-edit
# or create new commit:
git commit -m "Fix formatting"
```

### "Test failed"
```bash
# Run locally to debug
make test

# Run specific test
go test -v -run TestName ./...

# Check test output for clues, fix code, try again
```

### "Lint failed"
```bash
# Run locally
make lint

# Review error, edit code to fix
vim file.go

# Commit again
git commit -m "Fix lint issues"
```

### "Can't merge PR"
Check GitHub PR page:
- Branch needs update? → `git pull --rebase origin main`
- Needs approval? → Wait for reviewer
- Comments unresolved? → Reply to all

## File Locations

```
gitvista/
├── internal/          # Main app code
│   ├── git/          # Git parsing
│   ├── server/       # HTTP server
│   └── ...
├── cmd/
│   ├── vista/        # Main binary
│   └── gitcli/       # CLI binary
├── web/              # Frontend (JavaScript)
├── test/
│   ├── integration/  # Component tests
│   └── e2e/          # End-to-end tests
├── .github/workflows/ # CI/CD
├── lefthook.yml      # Pre-commit hooks
└── Makefile          # Build targets
```

## Go Version

Required: **Go 1.26+**

Check version:
```bash
go version
```

Install: https://golang.org/dl/

## Key Files

| File | Purpose |
|------|---------|
| `DEVELOPMENT.md` | Complete development guide |
| `.github/BRANCH_PROTECTION.md` | Branch rules for main |
| `.github/CI-CD-OVERVIEW.md` | Architecture overview |
| `lefthook.yml` | Pre-commit hook config |
| `Makefile` | Build targets |
| `.golangci.yml` | Linter configuration |

## Help Commands

```bash
# See all Makefile targets
make help

# Check hook status
lefthook status

# Run hooks manually
lefthook run pre-commit

# Get Go help
go help

# Get specific command help
go help test
go help build
```

## Common Mistakes

### ❌ Pushing without testing
```bash
# Always run locally first
make ci-local  # ~5 minutes
```

### ❌ Skipping pre-commit hooks
```bash
# Never do this
git commit --no-verify

# Only if absolutely necessary, but tests the whole CI then
```

### ❌ Making changes on main
```bash
# Always use a feature branch
git checkout -b feature/name

# NOT:
git checkout main
git add .
git commit -m "..."
```

### ❌ Forgetting to update imports
```bash
# Pre-commit hooks run goimports automatically
# But if they don't:
make check-imports
```

## Performance Tips

### Faster Local Tests
```bash
# Skip coverage (faster)
go test ./...

# Skip -v (less output)
go test ./...

# Run specific package (faster)
go test ./internal/git
```

### Faster Builds
```bash
# Skip trimpath for local builds
go build -o gitvista ./cmd/vista

# For production builds:
go build -trimpath -ldflags="-s -w" -o gitvista ./cmd/vista
```

### Faster CI
```bash
# Run only what you changed
go test ./internal/git

# Not:
make ci-local  # runs everything (no Docker needed)

# But before pushing:
make ci-local  # verify everything passes
```

## Getting Help

1. **For setup issues:** See DEVELOPMENT.md
2. **For branch rules:** Check .github/BRANCH_PROTECTION.md
3. **For CI/CD:** Read .github/CI-CD-OVERVIEW.md
4. **For linting:** See .golangci.yml comments
5. **For hooks:** Check lefthook.yml comments

## Important Links

- [GitHub Repository](https://github.com/rybkr/gitvista)
- [Go Documentation](https://golang.org/doc/)
- [Lefthook Documentation](https://evilmartians.com/chronicles/lefthook-knock-down-your-git-pre-commit-hook)

## Minimal Setup

Absolute minimum to contribute:

```bash
# 1. Clone and setup
git clone https://github.com/rybkr/gitvista.git
cd gitvista
make setup-hooks

# 2. Make changes
git checkout -b feature/my-feature
vim internal/git/parser.go
git commit -m "Add feature"

# 3. Test
make test

# 4. Push
git push origin feature/my-feature

# 5. Create PR on GitHub
```

That's it! Pre-commit hooks and CI handle the rest.

---

**For detailed information, see:**
- DEVELOPMENT.md - Complete developer guide
- .github/README.md - Workflow documentation
- .github/BRANCH_PROTECTION.md - Branch rules
