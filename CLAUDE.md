# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the server (serves on http://localhost:8080)
go run ./cmd/vista -repo /path/to/git/repo

# Run all tests
make test

# Generate coverage report
make cover-html
```

## Architecture

GitVista is a real-time Git repository visualization tool with a Go backend and vanilla JavaScript frontend using D3.js for force-directed graph rendering.

### Backend (Go)

**`internal/gitcore/`** - Pure Git object parsing without external Git dependencies:
- `repository.go` - Repository initialization, ref loading, object traversal, and diff calculation between repository states
- `objects.go` - Reads loose and packed Git objects (commits, tags, trees), handles zlib decompression
- `pack.go` - Pack file and index parsing (v2 format), delta object reconstruction
- `types.go` - Core types: `Hash`, `Commit`, `Tag`, `Tree`, `TreeEntry`, `Signature`, `RepositoryDelta`

**`internal/server/`** - HTTP/WebSocket server:
- `server.go` - Main server with client connection management and broadcast channel
- `handlers.go` - REST endpoints: `/api/repository` (metadata), `/api/tree/{hash}` (tree objects)
- `websocket.go` - WebSocket lifecycle: initial state sync, ping/pong keepalive
- `watcher.go` - Filesystem watcher on `.git/` directory with debouncing; triggers repository reload and broadcasts deltas
- `update.go` - Computes `RepositoryDelta` between old and new repository states

### Frontend (JavaScript)

**`web/`** - Vanilla JS with ES modules loaded from CDN (D3.js):
- `app.js` - Entry point: bootstraps graph and backend connection
- `backend.js` - REST fetch and WebSocket connection management
- `graph/graphController.js` - D3 force simulation, zoom/pan, node dragging, delta application
- `graph/rendering/graphRenderer.js` - Canvas-based rendering of commits, branches, links
- `tooltips/` - Commit and branch tooltip overlays

### Data Flow

1. Server loads repository from `.git/` directory at startup
2. Client connects via WebSocket, receives initial state as `RepositoryDelta`
3. Filesystem watcher detects `.git/` changes, debounces, reloads repository
4. Server computes delta between old and new state, broadcasts to all clients
5. Frontend applies delta incrementally to D3 simulation graph
