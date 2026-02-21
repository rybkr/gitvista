# Branch Protection Rules for Main

This document describes the recommended GitHub branch protection settings for the `main` branch to ensure CI always passes before code is merged.

## Overview

The main branch is the source of truth for production-ready code. All changes must pass comprehensive automated checks before merging to ensure code quality, security, and functionality.

## Recommended Settings

Enable branch protection rules by navigating to:
**Settings → Branches → Add rule** with pattern `main`

### 1. Require a Pull Request before Merging

**Enable:** ✓
**Require approvals:** 1
**Dismiss stale pull request approvals when new commits are pushed:** ✓
**Require review from code owners:** ✗ (optional - enable if CODEOWNERS file exists)

**Rationale:** All changes must go through PR review and CI checks. Fresh approval after new commits ensures reviewers see final changes.

### 2. Require Status Checks to Pass Before Merging

**Enable:** ✓

This is the critical setting that enforces CI/CD compliance. The following status checks are required:

#### Required Status Checks (in order of importance)

1. **ci-status** ⭐ MASTER CHECK
   - Aggregates all CI job results
   - Must pass for any merge to proceed
   - This is the only check GitHub strictly requires to exist before CI runs

2. **Test** - Unit tests with coverage
   - Validates all code changes work as expected
   - Ensures test coverage maintained
   - Timeout: 5 minutes

3. **Integration Tests** - Integration test suite
   - Validates components work together
   - Uses full Git repository context
   - Timeout: 5 minutes

4. **E2E Tests** - End-to-end tests
   - Validates full user workflows
   - Compares output against git baseline
   - Timeout: 5 minutes

5. **Lint** - Code quality checks
   - Runs golangci-lint with configured linters
   - Catches common mistakes, security issues, code style
   - Only reports new issues in PRs

6. **Vet** - Static analysis
   - Runs `go vet` for correctness issues
   - Catches pointer/method errors, suspicious constructs
   - Prevents subtle bugs

7. **Format Check** - Code formatting
   - Ensures gofmt compliance
   - Maintains consistent code style
   - Prevents style debates in reviews

8. **Security Scan** - Vulnerability checking
   - Runs govulncheck for known CVEs
   - Prevents dependency vulnerabilities
   - Runs against Go 1.26

9. **Validate JavaScript** - Frontend validation
   - Checks JavaScript syntax correctness
   - Ensures ES module compliance
   - Catches typos in variable names

10. **Build** - Binary compilation
    - Verifies code compiles cleanly
    - Builds main binary (gitvista) and CLI
    - Ensures no missing imports or syntax errors

11. **Docker Build** - Container image verification
    - Verifies Docker image builds successfully
    - Validates Dockerfile and multi-stage build
    - Caches layers for faster subsequent builds

12. **Dependency Check** - Module hygiene
    - Verifies go.mod/go.sum are tidy
    - Ensures no orphaned or missing dependencies
    - Validates module integrity

**Configuration in GitHub UI:**
- Check "Require status checks to pass before merging"
- Check "Require branches to be up to date before merging"
- Search for and select each status check from the list above

### 3. Require Code Reviews

**Enable:** ✓
**Required number of reviews:** 1
**Require review from code owners:** ✗ (optional)
**Dismiss stale pull request approvals:** ✓
**Restrict who can push to matching branches:** ✗

**Rationale:** At least one code review before merge ensures knowledge sharing and catches issues missed by automation.

### 4. Require Up-to-Date Branches

**Enable:** ✓

**Rationale:** Ensures branch is rebased on latest main before merge, preventing broken main. Combined with required status checks, guarantees main always works.

### 5. Include Administrators

**Enable:** ✗

**Rationale:** Administrators can bypass if needed for emergency fixes, but normal development should follow all rules.

### 6. Restrict Pushes to Matching Branches

**Enable:** ✗ (optional)
**Allow specified actors to bypass:** ⭐ Recommended: Enable and select release automation

**Rationale:** Prevents accidental direct pushes to main. If enabled, allow CI/CD service accounts and release automation to bypass for automated deployments.

### 7. Additional Protections

#### Lock Branch
- Not recommended for active development
- Consider enabling before major releases

#### Require conversation resolution before merging
- Enable: ✓ (recommended)
- Ensures all review comments are addressed

#### Require linear history
- Enable: ✗
- We use squash-and-merge, so linear history isn't necessary

## Testing the Protection Rules

After enabling branch protection:

1. Create a feature branch: `git checkout -b test-branch`
2. Make a minor change (e.g., add a comment)
3. Push the branch and create a PR to main
4. Verify all CI checks appear and are required
5. Verify you cannot merge without passing checks
6. Verify you cannot merge without an approval

## Bypassing Protection (Emergency Only)

In urgent situations (production incident, security patch), administrators can:

1. Temporarily disable branch protection
2. Merge the critical fix
3. Re-enable branch protection immediately
4. Create a post-incident review of what went wrong

**Always document emergency bypasses with a PR comment explaining why.**

## CI Workflow Details

### Jobs and Duration

The CI pipeline consists of 10 independent jobs that run in parallel:

| Job | Duration | Critical |
|-----|----------|----------|
| Format Check | < 10s | ⭐ Yes |
| Vet | < 30s | ⭐ Yes |
| Lint | < 2m | ⭐ Yes |
| Security Scan | < 1m | ⭐ Yes |
| Test | < 3m | ⭐ Yes |
| Integration Tests | < 3m | ⭐ Yes |
| E2E Tests | < 3m | ⭐ Yes |
| Validate JavaScript | < 10s | ✓ Yes |
| Build | < 2m | ✓ Yes |
| Docker Build | < 3m | ✓ Yes |
| Dependency Check | < 30s | ✓ Yes |

**Total Expected Time:** ~3-5 minutes (parallel execution)

### What Each Check Does

#### Format Check (gofmt)
- Ensures all Go code follows standard formatting
- Auto-fixed by pre-commit hooks locally
- Prevents style debates in code review

#### Vet (go vet)
- Detects suspicious code patterns
- Catches pointer errors, unreachable code, unused variables
- Zero false positives, 100% trustworthy

#### Lint (golangci-lint)
- Runs 13+ linters: errcheck, staticcheck, revive, gosec, etc.
- Catches missing error handling, API misuse, security issues
- Configured in `.golangci.yml`

#### Security Scan (govulncheck)
- Checks for known CVEs in dependencies
- Uses Go vulnerability database
- Fails on confirmed exploitable vulnerabilities

#### Test
- Runs all unit tests with race detector
- Generates coverage report (uploaded to Codecov)
- 5-minute timeout for flaky test detection

#### Integration Tests
- Tests component interactions with real file I/O
- Uses full Git repository context
- Separate from unit tests for clarity

#### E2E Tests
- Tests complete workflows end-to-end
- Compares against git baseline
- Validates user-facing functionality

#### Validate JavaScript
- Node.js syntax validation (ast parsing)
- Checks for CommonJS in ES modules
- No linting (that's for pre-commit locally)

#### Build
- Compiles both binaries (gitvista and gitvista-cli)
- Uses minimal flags (-v for verbose only)
- Verifies all imports present

#### Docker Build
- Builds the production Docker image
- Uses multi-stage Dockerfile for minimal size
- Caches intermediate layers

#### Dependency Check
- Verifies go.mod/go.sum are synchronized
- Runs `go mod tidy -check` and `go mod verify`
- Catches accidental edits to module files

## Local Development

To replicate the CI checks locally before pushing:

```bash
# Install pre-commit framework
brew install lefthook  # macOS
# or: apt install lefthook  # Linux

# Install hook dependencies (optional, hooks check if tools exist)
go install github.com/golang/tools/cmd/goimports@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/golang/vuln/cmd/govulncheck@latest

# Install hooks
lefthook install

# Run all pre-commit checks manually
lefthook run pre-commit

# Or run individual checks
make format
make lint
make test
make integration
make e2e
```

## Troubleshooting

### Status Check Not Appearing in GitHub UI

- Wait 5 minutes for GitHub to cache the workflow result
- Check that the workflow file is valid YAML
- Verify the job name matches exactly in the branch protection settings
- Look at workflow run details for error messages

### "Branch protection requires x status checks, but only y exist"

This happens when:
- A required check hasn't run yet (wait for CI to complete)
- A check was renamed but branch protection still references old name
- The workflow file has a syntax error

**Solution:**
1. Check the latest workflow run succeeded
2. Update branch protection rules to match current job names
3. Verify `.github/workflows/ci.yml` has valid YAML

### All Checks Pass But PR Still Can't Merge

- **"Requires up-to-date branch"**: Rebase on main (`git pull --rebase origin main`)
- **"Requires code review"**: Wait for a reviewer to approve
- **"Requires conversation resolution"**: Reply to review comments
- **"Requires 1 approval"**: Check if the approver used "Request changes" instead of "Approve"

## Security Considerations

### Secrets in Logs

The CI workflow accesses no secrets except:
- `CODECOV_TOKEN` (read-only, coverage upload only)
- No AWS credentials, API keys, or production secrets

### Artifact Handling

- Build artifacts are not retained (not needed, rebuild on deploy)
- Coverage reports are uploaded to Codecov (public, safe)
- No sensitive data is output to logs

### Action Versions

All GitHub Actions are pinned to specific commit SHAs, never @latest or @main:
- Prevents supply chain attacks
- Ensures deterministic behavior
- Easy to audit and update

Update actions periodically:
```bash
# Check for updates
# or use Dependabot alerts for action updates
```

## Monitoring

GitHub provides dashboards for branch protection:

1. **Settings → Branches:** View protection status
2. **Insights → Network:** Visualize branch history
3. **Pull Requests:** See which checks are blocking
4. **Workflow runs:** Debug failed checks

Set up notifications for:
- Failing workflow runs in main branch
- Blocked PRs (optionally)
- Workflow timeouts

## FAQ

**Q: Can I merge without passing all checks?**
A: No, unless branch protection is disabled by an administrator (emergency only).

**Q: Why does check X take so long?**
A: Most checks run in parallel. Duration is usually the slowest single check (~3m for full suite).

**Q: Can I skip a failing check?**
A: Only administrators can approve a failed check through the GitHub UI bypass button.

**Q: What if a check is flaky?**
A: Investigate the failure, add retries to the workflow if needed, or fix the underlying test flakiness.

**Q: How do I update the required checks list?**
A: Edit `.github/workflows/ci.yml` and update the `ci-status` job's `needs:` list.

## Related Documentation

- [GitHub Branch Protection Rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/managing-a-branch-protection-rule)
- [GitVista CI Workflow](.github/workflows/ci.yml)
- [Lefthook Pre-commit Configuration](../../lefthook.yml)
- [Project Makefile](../../Makefile)
- [Go Testing Documentation](https://golang.org/pkg/testing/)
