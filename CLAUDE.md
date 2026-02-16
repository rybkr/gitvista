# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the server (serves on http://localhost:8080)
go run ./cmd/vista -repo /path/to/git/repo

# Run all tests
make test

# Run a single test by name
go test -v ./internal/gitcore -run TestLoadPackIndexV1

# Generate coverage report (covers internal/gitcore only)
make cover-html
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
- `types.go` - Core types: `Hash`, `Commit`, `Tag`, `Tree`, `TreeEntry`, `Signature`, `RepositoryDelta`

**`internal/server/`** - HTTP/WebSocket server:
- `server.go` - Main server, client management; static files served from `./web/` directory (not embedded)
- `handlers.go` - REST endpoints (see API section below)
- `websocket.go` - WebSocket lifecycle: initial state sync, ping/pong keepalive (54s/60s timeout)
- `watcher.go` - Filesystem watcher on `.git/` directory with debouncing; triggers repository reload and broadcasts deltas
- `broadcast.go` - Non-blocking delta broadcast to WebSocket clients (256-item buffer, drops on overflow)
- `update.go` - Computes `RepositoryDelta` between old and new repository states
- `status.go` - Working tree status via `git status --porcelain` (this is the one place the git CLI is used)

### API Endpoints

- `GET /api/repository` - Repository metadata (name, gitDir)
- `GET /api/tree/{hash}` - Tree object entries
- `GET /api/blob/{hash}` - Blob content (512KB cap, binary detection via first 8KB)
- `GET /api/tree/blame/{commitHash}?path={dirPath}` - Per-file blame for a directory
- `GET /api/ws` - WebSocket upgrade for real-time deltas + working tree status

### Frontend (JavaScript)

**`web/`** - Vanilla JS with ES modules, D3.js v7.9.0 loaded from CDN:
- `app.js` - Entry point: bootstraps graph, sidebar, tabs, and backend connection
- `backend.js` - REST fetch and WebSocket connection management
- `graph/graphController.js` - D3 force simulation, zoom/pan, node dragging, delta application
- `graph/rendering/graphRenderer.js` - Canvas-based rendering of commits, branches, links
- `graph/state/graphState.js` - State factory for commits/branches/nodes/links Maps + zoom
- `graph/layout/layoutManager.js` - Chronological commit positioning and viewport management
- `graph/constants.js` - Centralized D3 force parameters and UI dimensions
- `sidebar.js` - Collapsible, resizable sidebar with localStorage persistence
- `fileExplorer.js` - Lazy-loading file browser with keyboard navigation (W3C APG TreeView pattern) and ARIA accessibility
- `fileContentViewer.js` - File content display panel
- `indexView.js` - Working tree status view (staged/modified/untracked sections)
- `tooltips/` - Commit, branch, and blob tooltip overlays

### Data Flow

1. Server loads repository from `.git/` directory at startup (pure Go, no git CLI)
2. Client connects via WebSocket, receives initial state as `RepositoryDelta`
3. Filesystem watcher detects `.git/` changes, debounces, reloads repository
4. Server computes delta between old and new state, broadcasts to all clients
5. Frontend applies delta incrementally to D3 simulation graph

## Testing

Tests live alongside source code in `internal/gitcore/`. Test patterns:
- Table-driven subtests with `t.Run()`
- Hand-constructed binary data for Git object format tests (no fixture files)
- Coverage only measures `internal/gitcore` package (`make cover-html`)
