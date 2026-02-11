# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the server (serves on http://localhost:8080, port is hardcoded)
go run ./cmd/vista -repo /path/to/git/repo

# Limit commit traversal depth (default 1000, 0 = unlimited)
go run ./cmd/vista -repo /path/to/git/repo -depth 500

# Run all tests
make test

# Run a single test
go test -v -run TestParseCommitBody ./internal/gitcore

# Generate coverage report
make cover-html
```

## Architecture

GitVista is a real-time Git repository visualization tool with a Go backend and vanilla JavaScript frontend using D3.js for force-directed graph rendering.

### Backend (Go)

**`internal/gitcore/`** - Pure Git object parsing without external Git dependencies:
- `repository.go` - Repository initialization, object traversal, and diff calculation between repository states
- `refs.go` - Reference loading: loose refs, packed refs, HEAD resolution (symbolic and detached)
- `objects.go` - Reads loose and packed Git objects (commits, tags, trees), handles zlib decompression
- `pack.go` - Pack file and index parsing (v2 format), delta object reconstruction
- `types.go` - Core types: `Hash`, `Commit`, `Tag`, `Tree`, `TreeEntry`, `Signature`, `RepositoryDelta`

**`internal/server/`** - HTTP/WebSocket server:
- `server.go` - Main server setup, static file serving for `web/`
- `handlers.go` - REST endpoints: `/api/repository` (metadata), `/api/tree/{hash}` (tree objects)
- `websocket.go` - WebSocket lifecycle: initial state sync (chunked), ping/pong keepalive
- `watcher.go` - Filesystem watcher on `.git/` directory with debouncing; triggers repository reload
- `update.go` - Computes `RepositoryDelta` between old and new repository states
- `broadcast.go` - Client connection management and delta broadcast to all WebSocket clients
- `status.go` - Working tree status via `git status --porcelain` (staged/modified/untracked files)

### Frontend (JavaScript)

**`web/`** - Vanilla JS with ES modules, D3.js loaded from CDN:
- `app.js` - Entry point: bootstraps sidebar, indexView, graph, and backend; wires callbacks between them
- `backend.js` - REST fetch and WebSocket connection with `onDelta` and `onStatus` callbacks
- `sidebar.js` - Collapsible/resizable sidebar with localStorage persistence
- `indexView.js` - Working tree status UI (staged/modified/untracked file lists)
- `graph/graphController.js` - D3 force simulation, zoom/pan, node dragging, delta application
- `graph/rendering/graphRenderer.js` - Canvas-based rendering of commits, branches, tags, trees, links
- `graph/layout/layoutManager.js` - Force simulation configuration and chronological timeline positioning
- `graph/state/graphState.js` - Central state factory (commits Map, branches Map, nodes/links arrays)
- `graph/constants.js` - Rendering constants (node radius, link distance, zoom bounds, etc.)
- `tooltips/` - Tooltip overlays for commits, branches, and blob nodes; managed by `TooltipManager`

### Data Flow

1. Server loads repository from `.git/` directory at startup
2. Client connects via WebSocket, receives initial state as chunked `RepositoryDelta`
3. Filesystem watcher detects `.git/` changes, debounces, reloads repository
4. Server computes delta between old and new state, broadcasts to all clients
5. Frontend applies delta incrementally to D3 simulation graph
6. Working tree status is sent alongside deltas and displayed in the sidebar

## Testing

Tests are unit tests in `internal/gitcore/` that use synthetic in-memory Git objects (no fixture repos). Coverage is limited to gitcore parsing logic; no server or frontend tests exist.
