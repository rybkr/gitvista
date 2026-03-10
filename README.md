# GitVista

**Real-time Git repository visualization in your browser.**

GitVista is a lightweight local tool that renders your Git history as an interactive graph, with live updates as you commit, branch, and merge. The installable `gitvista` binary now ships only the localhost product. The public `gitvista.io` site is built separately.

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

## Configuration

All options can be set via CLI flags or environment variables.

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-repo` | `GITVISTA_REPO` | `.` | Path to git repository |
| `-port` | `GITVISTA_PORT` | `8080` | Server port |
| `-host` | `GITVISTA_HOST` | `127.0.0.1` | Bind address |
| | `GITVISTA_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| | `GITVISTA_LOG_FORMAT` | `text` | Log format: `text`, `json` |
| | `GITVISTA_CACHE_SIZE` | `500` | LRU cache capacity (blame + diff entries) |

## Architecture

```
cmd/vista/              Local GitVista binary
cmd/site/               Hosted gitvista.io binary
internal/gitcore/       Pure Go git object parsing (no git CLI)
internal/server/        Shared HTTP + WebSocket server components
internal/app/           App composition for local and hosted products
web/gitvista/           Shared GitVista repository experience
web/local + web/site/   Thin local and hosted shells
```

**Backend**: Go 1.24, two dependencies (`fsnotify`, `gorilla/websocket`). Parses `.git/` directly — loose objects, pack files, refs, tags, blame, working tree status, and diffs. No git CLI required for core parsing.

**Frontend**: No build toolchain. ES modules loaded natively by the browser. D3.js v7.9.0 via ESM CDN import. Canvas-based graph rendering with two layout strategies (force-directed and topological lane DAG). highlight.js 11.9.0 for syntax coloring.

**Real-time pipeline**: `fsnotify` watches `.git/` → debounced reload → delta computation → WebSocket broadcast to all connected clients.

## Deploy Local Binary

```bash
make build
./gitvista
```

## Deploy Hosted Site

```bash
make build-site
fly launch --no-deploy
fly deploy
```

The included `fly.toml` and `Dockerfile` target the hosted deployment path. The local binary remains the default release artifact for end users.

## Development

```bash
# Run tests (with race detector)
make test

# Run linter
make lint

# Run integration tests (starts a real server)
make integration

# All local CI checks (no Docker or network needed)
make ci-local
```

## License

[Apache License 2.0](LICENSE)
