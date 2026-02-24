# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run server (default: current directory on :8080)
go run ./cmd/vista -repo /path/to/git/repo

# Build binaries
make build                # gitvista + gitvista-cli

# Testing
make test                 # unit tests with -race and -cover (60s timeout)
make integration          # integration tests (build tag: integration)
make e2e                  # end-to-end tests (build tag: e2e)

# Run a single test by name
go test -v -race ./internal/gitcore -run TestLoadPackIndexV1

# Linting and formatting
make lint                 # golangci-lint (requires: brew install golangci-lint)
make format               # auto-format with gofmt
make check-imports        # goimports (requires: go install golang.org/x/tools/cmd/goimports@latest)
make validate-js          # check JS syntax + no CommonJS in ES modules

# Full local CI (format, imports, vet, lint, security, tests, build)
make ci-local

# Coverage
make cover-html           # generates and opens HTML coverage report
```

## Architecture

GitVista is a real-time Git repository visualization tool. Go backend parses `.git/` directly (no git CLI dependency), serves a vanilla JS frontend over HTTP/WebSocket.

### Backend (Go)

**`cmd/vista/main.go`** — Two startup modes:
- **Local mode:** `-repo` flag → single `gitcore.Repository` → `LocalServer`
- **SaaS mode:** no `-repo` → `RepoManager` for multi-repo session management

**`internal/gitcore/`** — Pure Go git object parser (~9K lines). Reads loose objects, pack files (v2 index, delta reconstruction), refs (loose + packed + symbolic chains), blame (BFS, max 1000 commits), tree/file diffs (rename detection), working tree status, and index parsing. Core types: `Repository`, `Commit`, `Tag`, `Tree`, `Hash`, `DiffEntry`, `FileDiff`.

**`internal/server/`** — HTTP + WebSocket server (~4.7K lines). Key pieces:
- `handlers.go` / `repo_handlers.go` — REST endpoints with LRU caches for blame/diff results
- `websocket.go` + `broadcast.go` — WebSocket lifecycle with ping/pong keepalive; non-blocking delta broadcast (256-item buffer)
- `watcher.go` + `update.go` — fsnotify on `.git/`, debounced reload, computes `RepositoryDelta` between old/new states
- `cache.go` — Generic thread-safe LRU cache with bounded size and eviction
- `validation.go` — Path traversal prevention, hash validation
- `ratelimit.go` — Per-client rate limiting

**`internal/repomanager/`** — SaaS multi-repo lifecycle: clone queue, LRU eviction, adaptive fetch scheduling.

**`assets.go`** — `go:embed` embeds entire `web/` directory into the binary.

### Frontend (Vanilla JS)

**`web/`** — ES modules with no build toolchain. D3.js v7.9.0 via ESM CDN import. Canvas-based rendering.

Two layout strategies:
- **`forceStrategy.js`** — D3 force simulation with charge/collision/link forces, timeline Y-positioning, node dragging
- **`laneStrategy.js`** — Topological column-reuse DAG layout, lane 0 seeded with main/master/trunk, deterministic positioning, 300ms transition animation

Key modules: `graphController.js` (main orchestrator, 1225 lines), `graphRenderer.js` (canvas renderer, 822 lines with progressive zoom detail), `fileExplorer.js` (tree browser with blame, 923 lines), `searchQuery.js` (qualifier parser: `author:`, `hash:`, `after:`, `before:`, `merge:`, `branch:`).

### Data Flow

1. Server loads repository from `.git/` at startup (pure Go parsing)
2. Client connects via WebSocket → receives initial state as `RepositoryDelta`
3. fsnotify detects `.git/` changes → debounced reload → delta computation → WebSocket broadcast
4. Frontend applies delta incrementally to D3 simulation; search/filters dim nodes client-side

### API Endpoints

- `GET /api/repository` — Repo metadata
- `GET /api/tree/{hash}` — Tree entries
- `GET /api/blob/{hash}` — Blob content (512KB cap)
- `GET /api/tree/blame/{commitHash}?path={dirPath}` — Per-file blame for directory
- `GET /api/commit/diff/{commitHash}` — Files changed in commit
- `GET /api/commit/diff/{commitHash}/file?path={path}` — Line-level unified diff
- `GET /api/working-tree/diff?path={filePath}` — Working tree diff
- `GET /api/ws` — WebSocket for real-time deltas + working tree status
- `GET /health` — Health check

## Dependencies

Go: only `fsnotify` (filesystem watcher) and `gorilla/websocket`. No frontend build toolchain.

## Testing Patterns

- Table-driven subtests with `t.Run()`; hand-constructed binary data for Git object tests (no fixture files)
- Integration tests (`test/integration/`) use build tag `integration`, start a real server on port 18080
- E2E tests (`test/e2e/`) use build tag `e2e`
- Well-covered: diff engine, pack file parsing, object parsing, LRU cache, rate limiter, validation
- Gaps: no unit tests for `refs.go`, `blame.go`, `watcher.go`, `broadcast.go`, `update.go`, `websocket.go`; handler tests cover only error cases

## Known Issues

- `internal/gitcore/` uses stdlib `log.Printf` in ~10 places (not slog), bypassing `GITVISTA_LOG_LEVEL` filtering
- No delta recursion depth limit in `pack.go` `readOffsetDelta()`
- `rateLimiter.Close()` panics on double-call (no `sync.Once` guard)
- Near-duplicate relative time functions in `utils/format.js` and `graph/utils/time.js`

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GITVISTA_REPO` | `.` | Repository path |
| `GITVISTA_DATA_DIR` | `/data/repos` | SaaS repo storage directory |
| `GITVISTA_PORT` | `8080` | Server port |
| `GITVISTA_HOST` | all interfaces | Bind address |
| `GITVISTA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `GITVISTA_LOG_FORMAT` | `text` | `text` or `json` |
| `GITVISTA_CACHE_SIZE` | `500` | LRU cache capacity (entries) |
