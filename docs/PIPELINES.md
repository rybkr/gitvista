# Pipelines

This file is the practical reference for how CI, deploys, and releases actually work in this repository.

## Workflows

| Workflow | File | Trigger | Purpose |
| --- | --- | --- | --- |
| CI | `.github/workflows/ci.yml` | Push to `main`/`dev`, PR to `main`, manual dispatch | Quality checks, tests, build validation |
| Deploy | `.github/workflows/deploy.yml` | `workflow_run` of CI on `main` (success), manual dispatch | Staging deploy + smoke test, then production deploy + smoke test |
| Release | `.github/workflows/release.yml` | Push tag `v*` | GoReleaser release and GHCR image publish |

## CI (`ci.yml`)

Concurrency:
- Single in-flight CI run per branch/PR (`cancel-in-progress: true`).

Jobs:
1. `quality`:
- Installs tooling (`goimports`, `golangci-lint`, `govulncheck`, `gosec`).
- Runs:
  - `make format-check`
  - `make imports-check`
  - `make vet`
  - `make deps-check`
  - `make lint`
  - `make security`
  - `make validate-js`
  - `make test-js`
2. `tests`:
- Runs Go test suites with race detector:
  - unit coverage run (`go test ... -coverprofile=test/cover/coverage.out`)
  - integration tests (`-tags=integration`)
  - e2e tests (`-tags=e2e`)
- Uploads coverage artifact and publishes to Codecov (`CODECOV_TOKEN`, non-blocking).
3. `build`:
- Runs `make build` and `make docker-build`.
- Uploads tarball artifact containing `gitvista` + `gitvista-cli`.
4. `ci-status`:
- Gate job (`if: always()`) requiring `quality`, `tests`, and `build` to be successful.

Branch protection:
- Require `CI Status` (not individual component jobs).

## Deploy (`deploy.yml`)

Trigger behavior:
- Automatic deploy path: runs when the `CI` workflow completes on `main` and succeeds.
- Manual override path: `workflow_dispatch`.

Concurrency:
- Single deploy at a time (`cancel-in-progress: false`).

Jobs:
1. `deploy-staging` (environment: `staging`)
- `flyctl deploy --config fly.staging.toml --app gitvista-staging`
2. `smoke-test-staging`
- `bash scripts/smoke-test.sh https://gitvista-staging.fly.dev`
3. `deploy-production` (environment: `production`)
- `flyctl deploy --app gitvista`
4. `smoke-test-production`
- `bash scripts/smoke-test.sh https://gitvista.fly.dev`

Notes:
- Deploy checks out `github.event.workflow_run.head_sha` when running from CI completion.
- Production protection is controlled by the GitHub `production` environment rules.

## Release (`release.yml`)

Trigger:
- Git tag push matching `v*`.

Jobs:
1. `release` (`goreleaser release --clean`)
- Produces GitHub release artifacts via GoReleaser.
2. `docker`
- Builds and pushes multi-arch image to GHCR (`linux/amd64`, `linux/arm64`).

Important:
- This workflow does not deploy to Fly.

## Required Secrets and Environments

Secrets:
- `FLY_API_TOKEN` for deploy workflow.
- `CODECOV_TOKEN` for CI coverage upload (pipeline does not fail if upload fails).
- `HOMEBREW_TAP_TOKEN` for GoReleaser tap publishing (if configured in `.goreleaser.yml`).

Environments:
- `staging` (deploy-staging gate).
- `production` (deploy-production gate).

## Local Parity Commands

Use these before opening a PR:

- `make test`:
  - unit + integration + e2e + JS tests
- `make ci-local`:
  - local CI approximation (format/imports/vet/lint/security-local/test/validate-js/build)
- `make ci-remote`:
  - full CI approximation including docker build and strict dependency checks

## Troubleshooting

1. CI fails in `quality`:
- Run `make ci-local` and fix the first failing target.
2. CI fails in `tests`:
- Reproduce with `make test`.
- Run a subset while iterating:
  - `go test ./...`
  - `go test -tags=integration ./test/integration/...`
  - `go test -tags=e2e ./test/e2e/...`
3. Deploy fails:
- Check whether `CI` completed successfully for the same SHA.
- Validate Fly credentials and app names in `fly.toml` / `fly.staging.toml`.
4. Release fails:
- Verify the tag format is `vX.Y.Z` and that GHCR/GitHub release permissions are available.
