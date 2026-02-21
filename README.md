# GitVista

**Real-time Git repository visualization in your browser.**

GitVista is a lightweight, self-hosted tool that renders your Git history as an interactive graph — with live updates as you commit, branch, and merge. No electron, no desktop app, no git CLI dependency for core parsing. Just a single binary that serves a web UI.

## Features

- **Live commit graph** — Force-directed and lane-based layouts, zoomable canvas with progressive detail (message at 1.5x, author at 2x, date at 3x)
- **Real-time updates** — Filesystem watcher on `.git/` broadcasts changes over WebSocket as you work
- **File explorer** — Lazy-loaded tree browser with blame annotations and keyboard navigation (W3C APG TreeView)
- **Syntax-highlighted diffs** — Unified diff view with dual line number gutters, expand-context, and highlight.js coloring
- **Author-colored nodes** — Distinct colors per contributor across both graph layouts
- **Search and filter** — Qualifier syntax (`author:`, `hash:`, `after:`, `before:`, `merge:`, `branch:`), debounced with recent search history
- **Working tree status** — Staged, modified, and untracked files with inline diffs
- **Dark / Light / System theme** — Three-state toggle with full CSS custom property system
- **Pure Go git parsing** — Reads loose objects, pack files (v2), refs, and tags directly. No libgit2 or git CLI for core operations
- **Two external dependencies** — `fsnotify` and `gorilla/websocket`. That's it.

## Quick Start

### From release binary

```bash
# Download the latest release for your platform
# https://github.com/rybkr/gitvista/releases

# Run against the current directory
gitvista

# Or point at a specific repo
gitvista -repo /path/to/your/repo
```

### From source

```bash
git clone https://github.com/rybkr/gitvista.git
cd gitvista
make build
./gitvista -repo /path/to/your/repo
```

Then open [http://localhost:8080](http://localhost:8080).

### Docker

```bash
docker build -t gitvista .

# Mount a local repo (read-only is fine)
docker run --rm -p 8080:8080 -v /path/to/repo:/repo:ro gitvista -repo /repo
```

## Configuration

All options can be set via CLI flags or environment variables.

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-repo` | `GITVISTA_REPO` | `.` | Path to git repository |
| `-port` | `GITVISTA_PORT` | `8080` | Server port |
| `-host` | `GITVISTA_HOST` | *(all interfaces)* | Bind address |
| | `GITVISTA_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| | `GITVISTA_LOG_FORMAT` | `text` | Log format: `text`, `json` |
| | `GITVISTA_CACHE_SIZE` | `500` | LRU cache capacity (blame + diff entries) |

## Architecture

```
cmd/vista/          Entry point, CLI parsing, signal handling
internal/gitcore/   Pure Go git object parsing (no git CLI)
internal/server/    HTTP + WebSocket server, caching, rate limiting
web/                Vanilla JS frontend (ES modules, D3.js, Canvas)
```

**Backend**: Go 1.24, two dependencies (`fsnotify`, `gorilla/websocket`). Parses `.git/` directly — loose objects, pack files, refs, tags, blame, working tree status, and diffs. No git CLI required for core parsing.

**Frontend**: No build toolchain. ES modules loaded natively by the browser. D3.js v7.9.0 via ESM CDN import. Canvas-based graph rendering with two layout strategies (force-directed and topological lane DAG). highlight.js 11.9.0 for syntax coloring.

**Real-time pipeline**: `fsnotify` watches `.git/` → debounced reload → delta computation → WebSocket broadcast to all connected clients.

## Deploy to Fly.io

```bash
fly launch --no-deploy
fly deploy
```

The included `fly.toml` and `Dockerfile` are ready for deployment. Set `GITVISTA_REPO` to the path of a git repo inside the container, or mount one via volume.

## Development

```bash
# Run tests (with race detector)
make test

# Run linter
make lint

# Run integration tests (starts a real server)
make integration

# All CI checks
make ci
```

## License

[Apache License 2.0](LICENSE)
