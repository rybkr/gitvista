GitVista configuration is split between local usage and the hosted site. In both modes, command-line flags take precedence over environment variables, and environment variables take precedence over built-in defaults.

## Local mode variables

Use these when you run GitVista against a repository on your own machine.

| Variable | Default | Purpose |
| --- | --- | --- |
| `GITVISTA_REPO` | `.` | Default repository path for local launches |
| `GITVISTA_PORT` | `8080` | Default local server port |
| `GITVISTA_HOST` | `127.0.0.1` | Default bind host for local access |
| `GITVISTA_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `GITVISTA_LOG_FORMAT` | `text` | Log output format: `text` or `json` |
| `GITVISTA_CACHE_SIZE` | `500` | Cache capacity for diff and blame data |

## Hosted mode variables

Use these when you run the hosted GitVista site instead of the local desktop-style flow.

| Variable | Default | Purpose |
| --- | --- | --- |
| `GITVISTA_DATA_DIR` | `/data/repos` | Storage path for managed hosted repositories |
| `GITVISTA_PORT` | `8080` | HTTP port for the hosted server |
| `GITVISTA_HOST` | `` | Bind host for the hosted server |
| `GITVISTA_CORS_ORIGINS` | unset | Comma-separated list of allowed browser origins |
| `GITVISTA_CLONE_ALLOWED_HOSTS` | unset | Comma-separated allowlist for clone hostnames |
| `GITVISTA_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `GITVISTA_LOG_FORMAT` | `text` | Log output format: `text` or `json` |

## Precedence

- Use flags for per-invocation overrides.
- Use environment variables for machine or deployment defaults.
- Fall back to built-in defaults when neither is set.

## Examples

```bash
GITVISTA_PORT=3000 git vista open
GITVISTA_LOG_FORMAT=json git vista doctor
GITVISTA_DATA_DIR=/srv/gitvista/repos go run ./cmd/site
```
