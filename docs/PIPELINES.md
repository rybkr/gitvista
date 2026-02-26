# CI/CD and Release Pipelines

This document describes the three GitHub Actions workflows that handle continuous integration,
continuous deployment, and versioned releases for GitVista, along with the supporting tooling
for smoke testing, dependency management, and local development parity.

## Table of Contents

1. [Overview](#overview)
2. [CI Pipeline](#ci-pipeline)
3. [Deploy Pipeline](#deploy-pipeline)
4. [Release Pipeline](#release-pipeline)
5. [Smoke Tests](#smoke-tests)
6. [Dependabot](#dependabot)
7. [Local Commands](#local-commands)
8. [Setup Requirements](#setup-requirements)

---

## Overview

Three workflows cover the full software delivery lifecycle:

| Workflow | File | Trigger | Purpose |
|----------|------|---------|---------|
| CI | `.github/workflows/ci.yml` | Push to `main`/`dev`, PRs to `main` | Quality gate: formatting, linting, tests, build |
| Deploy | `.github/workflows/deploy.yml` | Push to `main` | Staged rollout: staging → smoke tests → production |
| Release | `.github/workflows/release.yml` | Push of `v*` tag | Versioned artifacts, Docker images, production deploy |

The workflows compose into a delivery chain:

```
Pull Request
     |
     v
  CI runs (all 11 parallel jobs + ci-status gate)
     |
     v  [merge to main]
     |
     +-----> Deploy workflow (triggered by push to main)
     |              |
     |              v
     |         CI gate (polls for ci-status check)
     |              |
     |              v
     |         Deploy to staging (Fly.io: gitvista-staging)
     |              |
     |              v
     |         Smoke tests against staging
     |              |
     |              v
     |         Deploy to production (Fly.io: gitvista) [environment approval]
     |              |
     |              v
     |         Smoke tests against production
     |
     +-----> Release workflow (triggered by git tag vX.Y.Z)
                    |
                    v
               GoReleaser (cross-platform binaries + GitHub Release)
               Docker build and push to GHCR (linux/amd64, linux/arm64)
                    |
                    v
               Deploy to production (Fly.io) [environment approval]
                    |
                    v
               Smoke tests against production
```

The Deploy and Release pipelines both require the `production` GitHub environment, which
enforces any configured protection rules (reviewers, wait timers) before the production
deploy job runs.

---

## CI Pipeline

**File:** `.github/workflows/ci.yml`
**Triggers:** Push to `main` or `dev`; pull requests targeting `main`

### Concurrency

Only one CI run per PR executes at a time. Newer pushes to the same PR cancel the previous
in-progress run:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

### Jobs

All 11 quality jobs run in parallel on `ubuntu-latest` with Go 1.26 and module caching
enabled. The `ci-status` gate job collects their results.

```
format ----+
vet -------+
lint ------+
security --+
test ------+----> ci-status (gate)
integration+
e2e -------+
validate-js+
build -----+
docker-build+
dependencies+
```

#### format

Runs `gofmt -l` against the repository (excluding `vendor/` and `web/`) and fails if any
Go file differs from the canonical format. Fix locally with `make format`.

#### vet

Runs `go vet ./...` for static analysis of suspicious constructs.

#### lint

Installs `golangci-lint` v2.10.1 from source and runs it with the repository's
`.golangci.yml` configuration, with a 10-minute timeout.

#### security

Installs `govulncheck` and runs it against all packages to detect known CVEs in
dependencies. Results are uploaded to the repository's Security tab
(`security-events: write` permission).

#### test

Runs the full unit test suite with the race detector enabled:

```
go test -v -race -timeout 5m \
  -covermode=atomic \
  -coverprofile=test/cover/coverage.out \
  -coverpkg=./internal/... ./...
```

Coverage is uploaded to Codecov using `CODECOV_TOKEN`. A Codecov failure does not block CI
(`fail_ci_if_error: false`).

#### integration

Runs tests tagged `integration` against `./test/integration/...` with full git history
(`fetch-depth: 0`, required because the integration suite starts a real server on port
18080 and exercises it against the live repository).

#### e2e

Runs tests tagged `e2e` against `./test/e2e/...` with full git history. These tests build
`gitvista-cli` and compare its output against `git`.

#### validate-js

Uses Node.js 20 to:

1. Run `node --check` on every `.js` file under `web/` to catch syntax errors.
2. Grep for `module.exports` and `require(` in `web/**/*.js` to enforce ES module
   conventions. Files that intentionally use CommonJS must include a `// @allow-commonjs`
   comment on the offending line.

#### build

Compiles both binaries and verifies the resulting ELF files are present:

- `gitvista` from `./cmd/vista`
- `gitvista-cli` from `./cmd/gitcli`

#### docker-build

Builds the Docker image with layer caching via GitHub Actions cache (`type=gha`) but does
not push (`push: false`). This verifies `Dockerfile` correctness on every PR without
consuming a registry push.

#### dependencies

Runs `go mod tidy -diff` to detect whether the module graph is clean. A dirty `go.mod` or
`go.sum` (i.e., one that would be changed by `go mod tidy`) fails this job.

### ci-status Gate Job

`ci-status` runs with `if: always()` so it executes even when upstream jobs are cancelled
or failed. It has an explicit `needs` list covering all 11 jobs and exits 1 if any of them
did not succeed:

```yaml
needs: [format, vet, lint, security, test, integration, e2e, validate-js, build, docker-build, dependencies]
```

**This is the single check to configure in branch protection rules.** Require `CI Status`
rather than requiring each individual job — this handles the case where new jobs are added
without updating the branch protection ruleset.

---

## Deploy Pipeline

**File:** `.github/workflows/deploy.yml`
**Trigger:** Push to `main`

### Concurrency

Only one deploy runs at a time, and a new deploy does not cancel an in-progress one. This
prevents a race between two rapid merges from corrupting a partially deployed environment:

```yaml
concurrency:
  group: deploy
  cancel-in-progress: false
```

### Flow

```
push to main
     |
     v
[ci] Poll GitHub check-runs API for "CI Status" = success
     |  (polls every 10s, up to 60 attempts = ~10 minutes)
     |
     v
[deploy-staging] flyctl deploy --config fly.staging.toml --app gitvista-staging
     |  requires: "staging" GitHub environment
     |
     v
[smoke-test-staging] bash scripts/smoke-test.sh https://gitvista-staging.fly.dev
     |
     v
[deploy-production] flyctl deploy --app gitvista
     |  requires: "production" GitHub environment (manual approval if configured)
     |
     v
[smoke-test-production] bash scripts/smoke-test.sh https://gitvista.fly.dev
```

### CI Gate

The `ci` job does not simply check whether CI passed before this workflow was queued — it
actively waits for it. This handles the race condition where the Deploy workflow is
triggered by the same commit push that also queues CI, and CI has not yet finished.

The poll loop queries the GitHub check-runs API:

```bash
gh api repos/${{ github.repository }}/commits/${{ github.sha }}/check-runs \
  --jq '[.check_runs[] | select(.name == "CI Status")] | first | .conclusion // "pending"'
```

If CI concludes as `failure` or `cancelled`, the deploy aborts immediately. If it does not
conclude within 600 seconds (60 × 10s), the deploy times out and fails.

### Staging Configuration

The staging environment is defined by `fly.staging.toml`:

| Setting | Value |
|---------|-------|
| App name | `gitvista-staging` |
| Region | `iad` |
| CPU | 1 shared vCPU |
| Memory | 512 MB |
| Log level | `debug` |
| Concurrency soft limit | 200 connections |
| Concurrency hard limit | 250 connections |
| Persistent volume | `gitvista_staging_data` mounted at `/data` |

### Production Configuration

The production environment is defined by `fly.toml`:

| Setting | Value |
|---------|-------|
| App name | `gitvista` |
| Region | `iad` |
| CPU | 2 shared vCPUs |
| Memory | 1024 MB |
| Log level | `info` |
| Concurrency soft limit | 400 connections |
| Concurrency hard limit | 500 connections |
| Persistent volume | `gitvista_data` mounted at `/data` |

Both environments set `auto_stop_machines = 'off'` and `min_machines_running = 1` to
prevent cold starts.

---

## Release Pipeline

**File:** `.github/workflows/release.yml`
**Trigger:** Push of any tag matching `v*` (e.g., `v1.2.0`, `v0.9.0-rc.1`)

**Permissions required:** `contents: write` (to create the GitHub Release), `packages: write`
(to push to GHCR).

### Jobs

The `release` and `docker` jobs run in parallel. The `deploy-production` job waits for
both to succeed before deploying.

```
git tag v1.2.0 && git push origin v1.2.0
     |
     +-----> [release] GoReleaser
     |             - go mod tidy
     |             - Cross-compile gitvista + gitvista-cli
     |             - Create GitHub Release with changelog
     |             - Upload .tar.gz / .zip archives
     |
     +-----> [docker] Docker Build + Push to GHCR
                   - linux/amd64 and linux/arm64
                   - Tags: v1.2.0, v1.2 (major.minor), latest
                   |
                   v
     [deploy-production] flyctl deploy --app gitvista
          requires: "production" GitHub environment
                   |
                   v
     [smoke tests] bash scripts/smoke-test.sh https://gitvista.fly.dev
```

### GoReleaser

Configuration is in `.goreleaser.yml`. GoReleaser v2 is used (`version: '~> v2'`).

**Binaries produced:**

| Binary | Source | Platforms |
|--------|--------|-----------|
| `gitvista` | `./cmd/vista` | linux, darwin, windows × amd64, arm64 |
| `gitvista-cli` | `./cmd/gitcli` | linux, darwin, windows × amd64, arm64 |

All binaries are built with `CGO_ENABLED=0` (static, no libc dependency) and stripped with
`-s -w` to reduce binary size. Build metadata is embedded via ldflags:

```
-X main.version={{.Version}}
-X main.commit={{.ShortCommit}}
-X main.buildDate={{.Date}}
```

**Archives:**

- Linux and macOS: `.tar.gz` named `gitvista_<version>_<os>_<arch>.tar.gz`
- Windows: `.zip`

**Changelog:** GoReleaser generates the release changelog from commit messages. Commits
with prefixes `docs:`, `test:`, or `chore:` are excluded from the changelog.

**Docker (GoReleaser):** GoReleaser also publishes a Docker image to GHCR as part of the
release job:

```
ghcr.io/rybkr/gitvista:<version>
ghcr.io/rybkr/gitvista:latest
```

### Docker Job

The `docker` job runs independently of GoReleaser and publishes a multi-platform image via
`docker/build-push-action`:

- Platforms: `linux/amd64`, `linux/arm64`
- Tags derived from `docker/metadata-action`:
  - `ghcr.io/<repository>:<full-semver>` (e.g., `1.2.0`)
  - `ghcr.io/<repository>:<major>.<minor>` (e.g., `1.2`)
  - `ghcr.io/<repository>:latest` (only when the tag is on the default branch)

Layer caching uses GitHub Actions cache (`type=gha`) to speed up repeated builds.

### Creating a Release

```bash
# Tag a release (follow semver)
git tag -a v1.2.0 -m "Release v1.2.0"
git push origin v1.2.0
```

The tag push triggers the Release workflow automatically. No manual workflow dispatch is
needed.

---

## Smoke Tests

**File:** `scripts/smoke-test.sh`

The smoke test script is a minimal post-deploy health check. It is not a substitute for
the integration or e2e test suites — its purpose is to confirm that the deployed binary
started correctly and is serving traffic.

### What It Checks

| Check | Method | Pass Condition |
|-------|--------|---------------|
| `GET /health` returns 200 | `curl -w '%{http_code}'` | HTTP 200 |
| `GET /api/repository` returns valid JSON | `curl` + `python3 -c "json.load()"` | Parses without error |
| WebSocket endpoint is reachable | `curl` with Upgrade headers | HTTP 101 or 400 |

The WebSocket check accepts both 101 (successful upgrade) and 400 (server received and
rejected the plain HTTP upgrade attempt) as a pass, because `curl` cannot complete a
proper WebSocket handshake. Either status confirms the endpoint is live.

### Startup Wait

Before running checks, the script polls `GET /health` until the service responds or the
timeout expires:

```
Waiting for https://gitvista-staging.fly.dev to become healthy (timeout: 30s)...
```

This accommodates the delay between `flyctl deploy` returning and the new VM accepting
traffic.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SMOKE_TIMEOUT` | `30` | Seconds to wait for the health endpoint before aborting |
| `SMOKE_INTERVAL` | `5` | Seconds between health poll attempts |

### Running Locally

```bash
# Against staging
$ bash scripts/smoke-test.sh https://gitvista-staging.fly.dev

# Against production
$ bash scripts/smoke-test.sh https://gitvista.fly.dev

# Against a local server
$ go run ./cmd/vista -repo /path/to/repo &
$ bash scripts/smoke-test.sh http://localhost:8080

# With a longer startup wait (e.g., cold environment)
$ SMOKE_TIMEOUT=60 bash scripts/smoke-test.sh https://gitvista-staging.fly.dev
```

Exit code 0 indicates all checks passed. Exit code 1 indicates one or more checks failed
or the service did not become healthy within the timeout.

### Example Output

```
Running smoke tests against https://gitvista-staging.fly.dev
---
Waiting for https://gitvista-staging.fly.dev to become healthy (timeout: 30s)...
Service is healthy after 10s

Running checks...
  PASS: GET /health returns 200
  PASS: GET /api/repository returns valid JSON
  PASS: WebSocket endpoint is reachable

---
RESULT: 3 passed, 0 failed
```

---

## Dependabot

**File:** `.github/dependabot.yml`

Dependabot opens automated pull requests to keep three dependency ecosystems current.
All updates are scheduled weekly on Monday.

| Ecosystem | Directory | Commit prefix | PR label(s) | Max open PRs |
|-----------|-----------|---------------|-------------|-------------|
| Go modules (`gomod`) | `/` | `deps:` | `dependencies`, `go` | 5 |
| GitHub Actions (`github-actions`) | `/` | `ci:` | `dependencies`, `ci` | 5 |
| Docker base images (`docker`) | `/` | `docker:` | `dependencies`, `docker` | 3 |

### PR Conventions

Dependabot PRs use standardized commit message prefixes that align with the project's
conventional commit style. These prefixes are also used by GoReleaser's changelog filter —
`chore:` prefixed commits (which Dependabot does not use) are excluded from release notes,
but `deps:`, `ci:`, and `docker:` commits are included.

All Dependabot PRs must pass the full CI pipeline (`ci-status` check) before they can be
merged, the same as any other PR.

---

## Local Commands

The `Makefile` provides targets for running deploy and smoke test operations locally.
These require `flyctl` to be installed and authenticated (`flyctl auth login`).

### deploy-staging

```bash
$ make deploy-staging
```

Equivalent to:

```bash
flyctl deploy --config fly.staging.toml --app gitvista-staging
```

Deploys the current working tree to the staging Fly.io app. The `Dockerfile` at the
repository root is used as the build context.

### deploy-production

```bash
$ make deploy-production
```

Equivalent to:

```bash
flyctl deploy --app gitvista
```

Deploys to production using `fly.toml`. Use with caution — this bypasses the smoke test
gate that the automated pipeline enforces. Run `make smoke-test URL=https://gitvista.fly.dev`
after a manual production deploy.

### smoke-test

```bash
$ make smoke-test URL=https://gitvista-staging.fly.dev
$ make smoke-test URL=https://gitvista.fly.dev
$ make smoke-test URL=http://localhost:8080
```

Delegates to `bash scripts/smoke-test.sh <URL>`. The `URL` variable is required; running
`make smoke-test` without it will produce a usage error from the script.

### ci-local

```bash
$ make ci-local
```

Runs the full local CI sequence: format check, import organization, vet, lint, security
scan, unit tests, integration tests, e2e tests, JS validation, JS tests, and binary build.
Does not include Docker build or dependency tidy check (use `make ci-remote` for those).

---

## Setup Requirements

The following configuration is required before the pipelines will function correctly.

### GitHub Environments

Two environments must be created in the repository settings
(**Settings > Environments**):

| Environment | Recommended protection rules |
|-------------|------------------------------|
| `staging` | None required (automated deployment) |
| `production` | Required reviewers (at least one approver) or a wait timer |

The `production` environment gate applies to both the Deploy pipeline
(`deploy-production` job) and the Release pipeline (`deploy-production` job). Without
this environment configured, both pipelines will deploy to production immediately after
staging smoke tests pass.

### Repository Secrets

| Secret | Used by | Description |
|--------|---------|-------------|
| `FLY_API_TOKEN` | Deploy, Release | Fly.io API token with deploy permissions for both `gitvista` and `gitvista-staging` apps |
| `CODECOV_TOKEN` | CI (`test` job) | Codecov upload token. Optional — CI does not fail if this is missing (`fail_ci_if_error: false`) |

`GITHUB_TOKEN` is provided automatically by GitHub Actions. It is used for GoReleaser
(creating GitHub Releases) and for GHCR authentication (pushing Docker images).

### Fly.io Apps

Two Fly.io apps must exist before the pipelines can deploy:

```bash
# Create the staging app
$ flyctl apps create gitvista-staging

# Create the staging persistent volume
$ flyctl volumes create gitvista_staging_data --region iad --size 10 --app gitvista-staging

# Create the production app
$ flyctl apps create gitvista

# Create the production persistent volume
$ flyctl volumes create gitvista_data --region iad --size 50 --app gitvista
```

The apps are configured to run in the `iad` (Ashburn, VA) region as specified in
`fly.staging.toml` and `fly.toml`. To use a different region, update the `primary_region`
field in both files and create volumes in the target region.

### Branch Protection

Configure branch protection for `main` in **Settings > Branches**:

- Require status checks to pass before merging
- Add `CI Status` as a required status check (this is the `ci-status` gate job)
- Require branches to be up to date before merging
- Require pull request reviews (recommended: at least 1 approver)

Do not add individual job names (`format`, `test`, etc.) as required checks — only
`CI Status` is needed because it aggregates all jobs.

### GHCR Visibility

Docker images are pushed to GHCR under the repository owner's namespace
(`ghcr.io/rybkr/gitvista`). If the repository is private, the package visibility
defaults to private. To make images publicly pullable:

1. Navigate to the package in **github.com > Packages**.
2. Under **Package settings > Danger Zone**, change visibility to **Public**.

Alternatively, configure this via the GitHub API or `gh` CLI after the first release push
creates the package.
