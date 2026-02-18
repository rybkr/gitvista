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

# Run all CI checks (tests, lint, integration tests, build)
make ci

# Run integration tests
make integration

# Run linter (requires golangci-lint: brew install golangci-lint)
make lint

# Build binary
make build

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
- `blame.go` - Per-directory blame via BFS commit walk (max 1000 commits), cached by `commitHash:dirPath` in sync.Map
- `diff.go` - Tree diffing (`TreeDiff`) and line-level file diffing (`ComputeFileDiff`); limits: 500 entries, 512KB blobs, 3 context lines
- `types.go` - Core types: `Hash`, `Commit`, `Tag`, `Tree`, `TreeEntry`, `Signature`, `RepositoryDelta`, `DiffEntry`, `CommitDiff`, `FileDiff`, `DiffHunk`, `DiffLine`

**`internal/server/`** - HTTP/WebSocket server:
- `server.go` - Main server, client management; serves embedded static files from `assets.go`
- `handlers.go` - REST endpoints; holds `sync.Map` caches for blame and diff results (keyed by `commitHash[:dirPath]`)
- `websocket.go` - WebSocket lifecycle: initial state sync, ping/pong keepalive (54s/60s timeout)
- `watcher.go` - Filesystem watcher on `.git/` directory with debouncing; triggers repository reload and broadcasts deltas
- `broadcast.go` - Non-blocking delta broadcast to WebSocket clients (256-item buffer, drops on overflow)
- `update.go` - Computes `RepositoryDelta` between old and new repository states
- `status.go` - Working tree status via `git status --porcelain` (this is the one place the git CLI is used)
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
- `GET /api/ws` - WebSocket upgrade for real-time deltas + working tree status

### Frontend (JavaScript)

**`web/`** - Vanilla JS with ES modules, D3.js v7.9.0 loaded from CDN:
- `app.js` - Entry point: bootstraps graph, sidebar, tabs, and backend connection; handles `#<commitHash>` URL fragment for deep-linking to commits
- `backend.js` - REST fetch and WebSocket connection management
- `logger.js` - Lightweight structured logger used throughout the frontend
- `graph.js` - Thin wrapper that creates and exposes the graph controller
- `graph/graphController.js` - D3 force simulation, zoom/pan, node dragging, delta application
- `graph/rendering/graphRenderer.js` - Canvas-based rendering of commits, branches, links
- `graph/state/graphState.js` - State factory for commits/branches/nodes/links Maps + zoom
- `graph/layout/layoutManager.js` - Chronological commit positioning and viewport management
- `graph/constants.js` - Centralized D3 force parameters and UI dimensions
- `graph/types.js` - Frontend type documentation (JSDoc shapes)
- `graph/utils/` - Graph-specific utilities: `palette.js` (color assignment), `time.js` (date formatting)
- `sidebar.js` - Collapsible, resizable sidebar with localStorage persistence
- `sidebarTabs.js` - Tab switching for sidebar panels (Repository / File Explorer)
- `infoBar.js` - Repository info header (branch, commit count, tags, remotes)
- `fileExplorer.js` - Lazy-loading file browser with keyboard navigation (W3C APG TreeView pattern) and ARIA accessibility
- `fileIcons.js` - File extension to icon/color mapping for the file explorer
- `fileContentViewer.js` - File content display panel
- `diffView.js` - Commit diff view: lists changed files for a commit
- `diffContentViewer.js` - Line-level unified diff renderer with hunk display
- `indexView.js` - Working tree status view (staged/modified/untracked sections)
- `keyboardShortcuts.js` - Global keyboard handler; supports single-key and two-key sequences (e.g. `G→H` to jump to HEAD)
- `keyboardHelp.js` - Keyboard shortcut help overlay
- `toast.js` - Transient toast notification system
- `tooltips/` - Commit, branch, and blob tooltip overlays (extend `baseTooltip.js`)
- `utils/` - Shared utilities: `colors.js`, `format.js`

### Data Flow

1. Server loads repository from `.git/` directory at startup (pure Go, no git CLI)
2. Client connects via WebSocket, receives initial state as `RepositoryDelta`
3. Filesystem watcher detects `.git/` changes, debounces, reloads repository
4. Server computes delta between old and new state, broadcasts to all clients
5. Frontend applies delta incrementally to D3 simulation graph

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
- Server tests cover handlers, validation, rate limiting, and status parsing

## Environment Variables

- `GITVISTA_REPO` - Default repository path (default: current directory)
- `GITVISTA_PORT` - Server port (default: 8080)
- `GITVISTA_HOST` - Bind host (default: all interfaces)
