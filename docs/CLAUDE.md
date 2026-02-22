# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the server (serves on http://localhost:8080)
go run ./cmd/vista              # Uses current directory as repo
go run ./cmd/vista -repo /path/to/git/repo
go run ./cmd/vista -port 3000   # Custom port

# Environment variables can also be used
GITVISTA_REPO=/path/to/repo GITVISTA_PORT=3000 go run ./cmd/vista

# Run all unit tests
make test

# Run local CI checks (no Docker or network needed)
make ci-local

# Run all CI checks including Docker build and dep verification
make ci-remote

# Run integration tests
make integration

# Run linter (requires golangci-lint: brew install golangci-lint)
make lint

# Build binary
make build

# Count lines of code
make cloc

# Run a single test by name
go test -v ./internal/gitcore -run TestLoadPackIndexV1

# Run tests with race detection (matches CI)
go test -race -timeout 5m ./...

# Run tests with coverage
go test -v -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Validate JavaScript syntax (no build toolchain needed)
for file in web/*.js; do node --check "$file"; done
```

## Architecture

GitVista is a real-time Git repository visualization tool with a Go backend and vanilla JavaScript frontend using D3.js for force-directed graph rendering.

### Backend (Go)

**`internal/gitcore/`** - Pure Git object parsing without external Git dependencies:
- `repository.go` - Repository initialization, object traversal, and diff calculation between repository states
- `refs.go` - Reference loading: loose refs, packed refs, HEAD resolution (supports symbolic ref chains)
- `objects.go` - Reads loose and packed Git objects (commits, tags, trees), handles zlib decompression
- `pack.go` - Pack file and index parsing (v2 format), delta object reconstruction
- `blame.go` - Per-directory blame via BFS commit walk (max 1000 commits); caching is handled at the server layer via LRU cache
- `diff.go` - Tree diffing (`TreeDiff`) and line-level file diffing (`ComputeFileDiff`); limits: 500 entries, 512KB blobs, 3 context lines. Includes exact-hash rename detection post-processing
- `types.go` - Core types: `Hash`, `Commit`, `Tag`, `Tree`, `TreeEntry`, `Signature`, `RepositoryDelta`, `DiffEntry`, `CommitDiff`, `FileDiff`, `DiffHunk`, `DiffLine`

**`internal/server/`** - HTTP/WebSocket server:
- `server.go` - Main server with HTTP timeouts, graceful shutdown, and client management; serves embedded static files from `assets.go`
- `handlers.go` - REST endpoints; uses LRU caches for blame and diff results (keyed by `commitHash[:dirPath]`)
- `cache.go` - Generic thread-safe LRU cache with bounded size and eviction policy (replaces unbounded sync.Map)
- `websocket.go` - WebSocket lifecycle: initial state sync, ping/pong keepalive (54s/60s timeout)
- `watcher.go` - Filesystem watcher on `.git/` directory with debouncing; triggers repository reload and broadcasts deltas
- `broadcast.go` - Non-blocking delta broadcast to WebSocket clients (256-item buffer, drops on overflow)
- `update.go` - Computes `RepositoryDelta` between old and new repository states
- `status.go` - Server-layer working tree status translation from pure Go gitcore parsing
- **Note:** No git CLI is required. All git operations, including working tree status and diffs, use pure Go parsing. See `internal/gitcore/status.go` and `internal/gitcore/worktree_diff.go` for implementation.
- **Note:** `internal/gitcore/` still uses `log.Printf` in ~10 places (refs.go, objects.go, pack.go). The slog migration (F6) only covers `internal/server/`. These calls bypass `GITVISTA_LOG_LEVEL` filtering.
- `health.go` - Health check endpoint
- `ratelimit.go` - Per-client rate limiting for API endpoints
- `validation.go` - Input validation helpers (path traversal prevention, hash validation)
- `types.go` - Server-specific types and error responses

**`assets.go`** - go:embed for web assets; embeds entire `web/` directory into binary

### API Endpoints

- `GET /api/repository` - Repository metadata (name, gitDir, currentBranch, headHash, counts, tags, remotes)
- `GET /api/tree/{hash}` - Tree object entries
- `GET /api/blob/{hash}` - Blob content (512KB cap, binary detection via first 8KB)
- `GET /api/tree/blame/{commitHash}?path={dirPath}` - Per-file blame for a directory
- `GET /api/commit/diff/{commitHash}` - List of files changed in a commit (`CommitDiff` with stats)
- `GET /api/commit/diff/{commitHash}/file?path={path}` - Line-level unified diff for a specific file (`FileDiff` with hunks)
- `GET /api/working-tree/diff?path={filePath}` - Working tree diff for a specific file (pure Go implementation via `ComputeWorkingTreeFileDiff`)
- `GET /api/ws` - WebSocket upgrade for real-time deltas + working tree status
- `GET /health` - Health check endpoint (returns `{"status": "ok"}`)

### Frontend (JavaScript)

**`web/`** - Vanilla JS with ES modules, D3.js v7.9.0 loaded via ESM `import` from CDN (in graphController.js, graphState.js, forceStrategy.js):
- `app.js` - Entry point: bootstraps graph, sidebar, theme, search, filters, and backend connection; handles `#<commitHash>` URL fragment for deep-linking to commits
- `backend.js` - REST fetch and WebSocket connection management with exponential backoff reconnection (1s initial, 30s max)
- `logger.js` - Lightweight structured logger with info/warn/error levels and ISO timestamps
- `graph.js` - Thin wrapper that creates and exposes the graph controller
- `search.js` - Enhanced commit search with dropdown, qualifier suggestions, recent searches (localStorage), debounced input (200ms), result count badge
- `searchQuery.js` - Pure query parser with qualifiers (`author:`, `hash:`, `after:`, `before:`, `merge:`, `branch:`), relative date shorthand (7d, 2w, 3m, 1y), BFS branch reachability
- `graphFilters.js` - Filter popover with toggles (hideRemotes, hideMerges, hideStashes), branch focus dropdown with BFS reachability, localStorage persistence, active filter badge count
- `themeToggle.js` - Three-state cycle toggle (system/light/dark), sets `data-theme` attribute on `<html>`, persisted to localStorage
- `graph/graphController.js` - Main orchestrator (1225 lines): D3 simulation, zoom/pan, node dragging, delta reconciliation, filter predicate building, commit navigation, ResizeObserver
- `graph/rendering/graphRenderer.js` - Canvas renderer (822 lines): commit circles, merge diamonds, branch/tag pills, stepped cross-lane arrows, spawn animations, progressive zoom detail (message >= 1.5x, author >= 2.0x, date >= 3.0x), dimmed nodes at 15% opacity
- `graph/state/graphState.js` - State factory for commits/branches/nodes/links Maps + layoutMode, searchState, filterState, headHash, tags, stashes
- `graph/layout/layoutManager.js` - Timeline positioning for ForceStrategy: time-based Y sorting, proportional spacing, auto-centering
- `graph/layout/layoutStrategy.js` - Layout strategy interface documentation (JSDoc-only, no runtime code)
- `graph/layout/forceStrategy.js` - D3 force simulation with charge/collision/link forces, uses LayoutManager for timeline, node dragging with fx/fy
- `graph/layout/laneStrategy.js` - Topological column-reuse DAG layout (488 lines): newest-first sort, lane 0 seeded with main/master/trunk, smooth 300ms transition animation, dragging disabled
- `graph/constants.js` - Centralized D3 force parameters, UI dimensions, lane layout constants (LANE_WIDTH=80, LANE_MARGIN=60), progressive zoom thresholds, 8 lane colors
- `graph/types.js` - Frontend type documentation (JSDoc shapes) including lane and filter state types
- `graph/utils/palette.js` - Reads CSS custom properties, returns GraphPalette with fallback defaults
- `graph/utils/time.js` - Human-readable relative time strings, timestamp extraction with fallback chain
- `sidebar.js` - Activity bar (40px icon strip) + collapsible/resizable panel with localStorage persistence; replaced former tab-based `sidebarTabs.js`
- `infoBar.js` - Collapsible repo metadata section (branch, commit count, branches, tags, remotes, description)
- `fileExplorer.js` - Full tree browser (923 lines): lazy fetch with cache, blame annotations, W3C APG TreeView keyboard nav, filter input, breadcrumbs, working-tree status indicators, tree/diff toggle
- `fileIcons.js` - Extension/name-based SVG icon mapping, GitHub Primer-inspired palette, covers 20+ languages/filetypes
- `fileContentViewer.js` - Blob content display with highlight.js 11.9.0 (lazy CDN load), line numbers, binary/truncation notices
- `diffView.js` - Commit-level diff: file list with A/M/D/R status badges, stats bar, generation counter for stale response protection
- `diffContentViewer.js` - Line-level unified diff renderer with dual line number gutters, hunk headers, expand-context buttons, `showFromUrl()` for working-tree diffs
- `indexView.js` - Working tree status with collapsible staged/modified/untracked sections; clickable files trigger working-tree diff view
- `keyboardShortcuts.js` - Global keydown handler: `G` then `H` (500ms timeout) to jump to HEAD, `/` for search, `?` for help overlay, `J`/`K` for commit navigation, `Esc` to dismiss; modifier key suppression; typing guard for INPUT/TEXTAREA
- `keyboardHelp.js` - Modal overlay with shortcut table, self-contained CSS injection, backdrop blur, click-outside dismiss, `role="dialog"` with `aria-modal`
- `toast.js` - Toast notification queue (max 3 visible), bottom-left positioned, auto-dismiss, ARIA `role="status"`
- `tooltips/` - Commit and branch tooltip overlays (extend `baseTooltip.js`); commit tooltip includes hash copy, Prev/Next navigation
- `utils/colors.js` - `getAuthorColor(email)`: djb2 hash to HSL color, memoized in module-level Map
- `utils/format.js` - `shortenHash()`, `formatRelativeTime()`

### Data Flow

1. Server loads repository from `.git/` directory at startup (pure Go parsing: loose objects, pack files, refs, tags, index, working tree status, diffs)
2. Client connects via WebSocket, receives initial state as `RepositoryDelta`
3. Filesystem watcher detects `.git/` changes, debounces, reloads repository
4. Server computes delta between old and new state, broadcasts to all clients
5. Frontend applies delta incrementally to D3 simulation graph
6. Search queries and filter predicates are applied client-side to dim/hide nodes without removing them from the simulation

## Dependencies

Go external dependencies (see `go.mod`):
- `github.com/fsnotify/fsnotify` - Filesystem watcher for `.git/` directory
- `github.com/gorilla/websocket` - WebSocket server implementation

No frontend build toolchain — the JS uses native ES modules loaded directly by the browser.

## Testing

Tests live alongside source code in `internal/gitcore/` and `internal/server/`. Test patterns:
- Table-driven subtests with `t.Run()`
- Hand-constructed binary data for Git object format tests (no fixture files)
- Integration tests in `test/integration/` use build tag `integration`; they start a real server on port 18080 against the current repo
- Server tests cover handlers, validation, rate limiting, status parsing, LRU cache behavior, shutdown lifecycle, and structured logging

**Well-covered areas:** Diff engine (1,191 lines of tests including rename detection), pack file parsing (v1/v2/large offsets/delta application), object parsing, LRU cache (including concurrency), rate limiter, validation, shutdown, status parsing.

**Test gaps:** No unit tests for `refs.go`, `blame.go`, `watcher.go`, `broadcast.go`, `update.go`, or `websocket.go`. Handler tests only cover error cases (invalid method/hash/path), not success paths. `parseUnifiedDiff()` in handlers.go is untested.

## Known Issues

- **golangci-lint config (`.golangci.yml`) uses v1 schema** but CI resolves to v2.x via `version: latest`. The config needs migration to golangci-lint v2 format. `make lint` will fail until fixed.
- **`make test` lacks `-race` flag**, providing weaker guarantees than CI (`go test -race -timeout 5m ./...`).
- **`main` branch is significantly behind `dev`**. All Sprint 1/2 features, design overhaul, and lane layout exist only on `dev`.
- **`internal/gitcore/` logging** uses stdlib `log.Printf` (not slog), bypassing `GITVISTA_LOG_LEVEL` filtering.
- **No delta recursion depth limit** in `pack.go` `readOffsetDelta()` — deeply chained deltas in malicious pack files could cause stack overflow.
- **`rateLimiter.Close()` panics on double-call** — no `sync.Once` guard on channel close.
- **Near-duplicate relative time functions** in `utils/format.js` and `graph/utils/time.js`.

## Environment Variables

- `GITVISTA_REPO` - Default repository path (default: current directory)
- `GITVISTA_PORT` - Server port (default: 8080)
- `GITVISTA_HOST` - Bind host (default: all interfaces)
- `GITVISTA_LOG_LEVEL` - Log verbosity: `debug`, `info`, `warn`, `error` (default: `info`)
- `GITVISTA_LOG_FORMAT` - Log output format: `text` or `json` (default: `text`)
- `GITVISTA_CACHE_SIZE` - LRU cache capacity in entries (default: 500)
