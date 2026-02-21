# GitVista SaaS Architecture Plan

**Date**: February 2026
**Status**: Draft -- Pending Approval
**Authors**: Architect Panel (A: Clone & Serve, B: Object Store Abstraction, C: Hybrid Event-Driven)

---

## Executive Summary

This plan transforms GitVista from a single-user CLI tool into a hosted SaaS where users paste any Git remote URL and get a live, real-time commit graph visualization. The architecture evolves in three phases:

1. **MVP (Weeks 1-6)**: Clone-and-serve on a single Fly.io machine. Bare-clone repos to a volume, point `gitcore.NewRepository()` at them, periodic fetch for updates. Public repos only, SQLite metadata.
2. **v1.0 (Weeks 7-14)**: Production SaaS. Separate web/worker tiers, PostgreSQL, Valkey cache, GitHub OAuth, private repo access, webhook-driven real-time updates.
3. **v2.0 (Weeks 15-22+)**: Multi-forge support (GitLab, Bitbucket), billing via Stripe, team features, and optional `ObjectReader` interface for performance-critical paths.

**Key Principle**: `gitcore` is the crown jewel and should not change for MVP/v1. The entire `internal/gitcore/` package works as-is -- the only change needed is a small fix to `findGitDirectory()` to support bare repos.

---

## Table of Contents

1. [Current Architecture](#1-current-architecture)
2. [Target Architecture](#2-target-architecture)
3. [Approach Decision](#3-approach-decision)
4. [System Architecture](#4-system-architecture)
5. [Clone/Fetch Lifecycle](#5-clonefetch-lifecycle)
6. [Storage Strategy](#6-storage-strategy)
7. [Authentication & Authorization](#7-authentication--authorization)
8. [Multi-Tenancy](#8-multi-tenancy)
9. [Real-Time Updates](#9-real-time-updates)
10. [API Design](#10-api-design)
11. [Frontend Changes](#11-frontend-changes)
12. [Data Model](#12-data-model)
13. [Deployment on Fly.io](#13-deployment-on-flyio)
14. [Implementation Phases](#14-implementation-phases)
15. [File Change Map](#15-file-change-map)
16. [Risks & Mitigations](#16-risks--mitigations)
17. [Cost Estimation](#17-cost-estimation)
18. [Future: ObjectReader Interface](#18-future-objectreader-interface)

---

## 1. Current Architecture

```
User runs: gitvista -repo /path/to/repo -port 8080

┌─────────────┐     ┌──────────────────┐     ┌──────────────┐
│   Browser   │────>│  Go HTTP Server  │────>│  gitcore     │
│   (D3.js)   │<────│  + WebSocket     │<────│  (pure Go    │
│             │     │                  │     │   git parser) │
└─────────────┘     └──────────────────┘     └──────┬───────┘
                           │                        │
                    fsnotify watcher          .git/ directory
                           │                   (local disk)
                           v
                    Delta broadcast
                    via WebSocket
```

**Constraints for SaaS transformation**:
- `gitcore` reads exclusively from the local filesystem (`os.Open`, `filepath.Walk`)
- `NewRepository(path)` expects a local `.git/` directory
- `fsnotify` watches local `.git/` for changes -- no network equivalent
- Server holds a single `*Repository` in memory -- no multi-tenancy
- No authentication, no user management, no persistence

---

## 2. Target Architecture

Users can:
- Paste any Git remote URL (GitHub, GitLab, Bitbucket, self-hosted) and see a live visualization
- Sign in with GitHub OAuth (primary), with GitLab/Bitbucket later
- Access private repos they have permission to view
- See near-real-time updates as the remote repo changes
- Free tier for individuals (5 repos), paid tier for teams (50+ repos)

---

## 3. Approach Decision

Three architectural approaches were evaluated:

| Approach | Time to MVP | Code Reuse | Complexity | Latency to First View | Scaling Path |
|----------|-------------|------------|------------|----------------------|--------------|
| **A: Clone & Serve** | 4-6 weeks | ~90% | Low | 5-30s (clone time) | Horizontal (machines + volumes) |
| **B: Object Store Abstraction** | 8-10 weeks | ~70% | High | 3-5s (partial fetch) | Memory-based (in-process cache) |
| **C: Hybrid Event-Driven** | 6-8 weeks | ~85% | Medium-High | 5-30s (clone time) | Best (separate web/worker tiers) |

### Decision: Phased Hybrid

- **MVP**: Architect A's clone-and-serve in a single binary (fastest path to working product)
- **v1.0**: Architect C's separated web/worker tiers with shared cache (production-ready)
- **v2.0**: Architect B's `ObjectReader` interface as an optimization for large repos

**Rationale**: Clone-and-serve maximizes `gitcore` reuse (zero changes to the parser). The worker separation in v1.0 prevents clone operations from blocking web requests. The `ObjectReader` interface is deferred because it requires building a Git smart HTTP v2 protocol client -- significant work that shouldn't block launch.

---

## 4. System Architecture

### Phase 1: MVP (Single Binary)

```
                         INTERNET
                            |
                     ┌──────┴──────┐
                     │  Fly Proxy  │
                     │  (TLS)      │
                     └──────┬──────┘
                            |
                  ┌─────────┴─────────┐
                  │    Single Go      │
                  │    Binary         │
                  │                   │
                  │ ┌───────────────┐ │
                  │ │ HTTP + WS     │ │
                  │ │ API Router    │ │
                  │ └───────┬───────┘ │
                  │         │         │
                  │ ┌───────┴───────┐ │
                  │ │ Repo Manager  │ │
                  │ │               │ │
                  │ │ - Clone queue │ │
                  │ │ - Fetch sched │ │
                  │ │ - LRU evict   │ │
                  │ └───────┬───────┘ │
                  │         │         │
                  │ ┌───────┴───────┐ │
                  │ │ RepoSessions  │ │
                  │ │ (per-repo     │ │
                  │ │  gitcore.Repo │ │
                  │ │  + WS clients)│ │
                  │ └───────────────┘ │
                  └─────────┬─────────┘
                            │
                   ┌────────┴────────┐
                   │  Fly Volume     │
                   │  /data/         │
                   │  ├─repos/       │  <- bare clones
                   │  └─gitvista.db  │  <- SQLite
                   └─────────────────┘
```

### Phase 2: v1.0 (Separated Tiers)

```
                         INTERNET
                            |
                     ┌──────┴──────┐
                     │  Fly Proxy  │
                     └──────┬──────┘
                            |
              ┌─────────────┴────────────┐
              │                          │
       ┌──────┴──────┐            ┌──────┴──────┐
       │  Web Tier   │            │  Web Tier   │
       │  HTTP + WS  │            │  HTTP + WS  │
       │  Auth, API  │            │  Auth, API  │
       └──────┬──────┘            └──────┬──────┘
              │                          │
    ┌─────────┴──────────────────────────┴─────────┐
    │              Shared Services                 │
    │                                              │
    │  ┌────────────┐  ┌────────┐  ┌─────────────┐ │
    │  │ PostgreSQL │  │ Valkey │  │ Valkey      │ │
    │  │ (metadata) │  │ (cache)│  │ (pub/sub)   │ │
    │  └────────────┘  └────────┘  └─────────────┘ │
    └──────────────────────┬───────────────────────┘
                           │
              ┌────────────┴───────────┐
              │                        │
       ┌──────┴──────┐          ┌──────┴──────┐
       │ Worker Tier │          │ Worker Tier │
       │ Clone/Fetch │          │ Clone/Fetch │
       │ Parse/Cache │          │ Parse/Cache │
       └──────┬──────┘          └──────┬──────┘
              │                        │
       ┌──────┴──────┐          ┌──────┴──────┐
       │ Fly Volume  │          │ Fly Volume  │
       │ /data/repos │          │ /data/repos │
       └─────────────┘          └─────────────┘
```

---

## 5. Clone/Fetch Lifecycle

### 5.1 State Machine

```
User requests URL
       |
       v
 ┌──────────┐
 │ PENDING  │  (queued for clone)
 └────┬─────┘
      │ git clone --bare --depth=50 --filter=blob:none
      v
 ┌──────────┐     periodic fetch     ┌──────────┐
 │  READY   │<───────────────────────│ FETCHING │
 │          │───────────────────────>│          │
 └────┬─────┘                        └──────────┘
      │ no activity for 24h
      v
 ┌──────────┐
 │ EVICTED  │  (clone deleted, re-cloneable on demand)
 └──────────┘
```

### 5.2 Clone Operation

1. **Normalize the URL** -- strip `.git` suffix, lowercase hostname, normalize SSH to HTTPS. Hash the canonical URL (SHA-256) for the directory name.
2. **Dedup check** -- if another user already triggered a clone for this URL, attach to the existing job.
3. **Clone** -- `git clone --bare --depth=50 --filter=blob:none <url> /data/repos/{hash}`
   - `--bare`: no working tree (saves ~50% disk, and gitcore doesn't need it for remote repos)
   - `--depth=50`: shallow clone for fast initial load
   - `--filter=blob:none`: skip blob content (fetched on demand for diffs/blame)
4. **Parse** -- `gitcore.NewRepository(bareRepoPath)` loads commits, branches, tags into memory.
5. **Serve** -- frontend receives the initial state via WebSocket.

**Why shell out to `git` for clone/fetch**: The `gitcore` package is a pure *reader*. Git's clone/fetch protocol (pack negotiation, smart HTTP, SSH transport) is a complex *write-path* operation. Using the `git` CLI for these two commands gets us SSH key forwarding, credential helpers, partial clone support, and pack-protocol negotiation for free.

### 5.3 Fetch Strategy (Phase 1)

| Repo State | Fetch Interval | Trigger |
|------------|---------------|---------|
| Active (WebSocket clients connected) | Every 30s | Ticker |
| Warm (accessed in last hour) | Every 5 min | Ticker |
| Cold (no access 1-24h) | Every 30 min | Ticker |
| Abandoned (no access 24h+) | Stop | Eviction candidate |

After each fetch:
1. Reload `gitcore.NewRepository()` on the updated clone
2. Compute delta via existing `Diff()` method
3. Broadcast delta to all subscribed WebSocket clients

### 5.4 Fetch Strategy (Phase 2 -- Webhooks)

```
┌────────────────────────────────────────────────┐
│  Is the remote a known forge with webhook API? │
│     YES: Register webhook (push, create, delete│
│          events) -> immediate FETCH on receipt  │
│     NO:  Fall back to polling (above)           │
└────────────────────────────────────────────────┘
```

Webhook support by forge:
- **GitHub**: `POST /repos/{owner}/{repo}/hooks`, verify via `X-Hub-Signature-256`
- **GitLab**: `POST /projects/{id}/hooks`, verify via `X-Gitlab-Token`
- **Self-hosted / other**: Polling fallback only

### 5.5 Eviction / Cleanup

- Background goroutine every 10 minutes
- Repos with no access in 24h and no active clients are deleted from disk
- Metadata row preserved (transitions to `state='evicted'`) so re-request is fast
- Partial clones (`--filter=blob:none`) are already small, but eviction prevents unbounded growth

### 5.6 Critical gitcore Fix: Bare Repo Support

`findGitDirectory()` in `repository.go` walks up looking for a `.git/` subdirectory. Bare repos don't have one -- the repo root IS the git directory. A small fix is needed:

```go
func findGitDirectory(startPath string) (gitDir string, workDir string, err error) {
    absPath, _ := filepath.Abs(startPath)

    // NEW: Check if startPath itself is a bare repo
    if isBareRepo(absPath) {
        return absPath, absPath, nil
    }

    // ... existing logic for .git/ directory ...
}

func isBareRepo(path string) bool {
    for _, required := range []string{"objects", "refs", "HEAD"} {
        if _, err := os.Stat(filepath.Join(path, required)); err != nil {
            return false
        }
    }
    return true
}
```

This is the **only** change needed in `gitcore` for the MVP. Everything else (pack reading, ref parsing, commit walking, diff computation) works identically on bare repos.

---

## 6. Storage Strategy

### 6.1 Fly Volumes

```
/data/                          <- Fly Volume mount point
  repos/                        <- Bare clones
    a1b2c3d4.../                <- SHA-256 prefix of canonical URL
      HEAD
      objects/
      refs/
      packed-refs
      shallow
  gitvista.db                   <- SQLite (Phase 1 only)
```

### 6.2 Sizing

| Volume Size | Repos Supported | Notes |
|-------------|----------------|-------|
| 10 GB | ~200 repos | Partial clones average 10-100 MB each |
| 50 GB | ~1000 repos | With eviction, handles >1K users |
| 100 GB+ | Scaled setup | Multiple volumes across workers |

### 6.3 Why Not Object Storage (S3/R2)?

`gitcore` does random-access reads into pack files (`readObjectAtOffset`). Object storage is optimized for whole-object reads, not random seeking. Local volumes give native filesystem semantics with zero impedance mismatch.

**When to reconsider**: Multi-region deployment (v2+) with shared storage would need R2/S3 with a local read-through cache.

---

## 7. Authentication & Authorization

### 7.1 OAuth Flow

```
Browser                  GitVista API              GitHub
  │                          │                       │
  │ GET /auth/github/login   │                       │
  │─────────────────────────>│                       │
  │ 302 -> github.com/oauth  │                       │
  │─────────────────────────────────────────────────>│
  │                          │                       │
  │ User authorizes          │                       │
  │<─────────────────────────────────────────────────│
  │ GET /auth/github/callback?code=xxx               │
  │─────────────────────────>│                       │
  │                          │ Exchange code-> token │
  │                          │──────────────────────>│
  │                          │<──────────────────────│
  │                          │ GET /user             │
  │                          │──────────────────────>│
  │                          │<──────────────────────│
  │ Set-Cookie: session=JWT  │                       │
  │<─────────────────────────│                       │
```

### 7.2 Token Storage

- **GitHub access token**: Encrypted at rest in the DB using AES-256-GCM. Encryption key from `GITVISTA_ENCRYPTION_KEY` env var (Fly secret). Never sent to browser.
- **Session token**: Signed JWT (HS256) in an HttpOnly, Secure, SameSite=Strict cookie. Contains `{user_id, github_login, exp}`. 1-hour expiry with sliding refresh.
- **No refresh tokens stored**. If session expires, user re-authenticates via OAuth.

### 7.3 Private Repo Access

For private repos, inject the user's OAuth token into git credential handling:
```bash
GIT_ASKPASS=/usr/local/bin/gitvista-askpass \
GITVISTA_TOKEN=<decrypted_token> \
git clone --bare ...
```

The token is held only in memory for the clone/fetch duration, then zeroed. Never written to the clone's git config.

### 7.4 Authorization Model

| Repo Type | Who Can View | Mechanism |
|-----------|-------------|-----------|
| Public | Any authenticated user | GitHub API check (`GET /repos/{owner}/{repo}` returns 200) |
| Private | Users with valid OAuth token that grants access | Token injected into clone; ACL stored in DB |

Multiple users viewing the same public repo share a single clone. Private repos also share if both users have access (verified via forge API on first request).

---

## 8. Multi-Tenancy

### 8.1 The RepoSession Abstraction

The current `Server` struct holds a single `*Repository`. For SaaS, `RepoSession` is a per-repo mini-server:

```go
type RepoSession struct {
    repoID      string              // SHA-256 of canonical URL
    remoteURL   string
    barePath    string              // /data/repos/{hash}

    repo        *gitcore.Repository
    repoMu      sync.RWMutex

    clients     map[*websocket.Conn]*clientInfo
    clientsMu   sync.RWMutex

    broadcast   chan server.UpdateMessage

    lastAccess  atomic.Value        // time.Time
    fetchTicker *time.Ticker

    blameCache  *server.LRUCache[any]
    diffCache   *server.LRUCache[any]

    ctx         context.Context
    cancel      context.CancelFunc
}
```

### 8.2 Isolation Guarantees

- **Data isolation**: Each `RepoSession` has its own `gitcore.Repository` pointing at its own directory.
- **Network isolation**: WebSocket broadcasts go only to clients subscribed to that `RepoSession`.
- **Authorization isolation**: JWT middleware + ACL check runs before a connection is attached to a session.

### 8.3 Shared Clone Deduplication

Two users viewing `github.com/torvalds/linux` share the same bare clone and `RepoSession`. This is safe because Git objects are content-addressed and immutable. Authorization is checked per-request, not per-clone.

---

## 9. Real-Time Updates

### Phase 1: Periodic Fetch

Replace `fsnotify` with a per-repo fetch ticker. After each `git fetch`:
1. Call `gitcore.NewRepository()` to reload
2. Compute delta via `newRepo.Diff(oldRepo)` (existing method)
3. Broadcast delta to subscribed WebSocket clients

### Phase 2: Webhook + Poll Hybrid

```
Webhook available?
  YES -> Register webhook, fetch on push event (~seconds latency)
  NO  -> Poll with adaptive interval (30s-15min based on activity)
```

**Adaptive polling**:
- Active viewers (WebSocket connected): every 30s
- Recent activity (push in last hour): every 2 min
- Dormant (no viewers, no recent push): every 15 min
- Abandoned (no viewers 24h+): stop polling

### Phase 2: Cross-Node Fan-Out (Valkey Pub/Sub)

When running multiple web nodes:
1. Worker fetches repo, computes delta, publishes to `repo:{id}:delta` channel
2. All web nodes subscribed to that channel receive the delta
3. Each web node forwards to its local WebSocket clients viewing that repo

---

## 10. API Design

### 10.1 New Endpoints

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `GET` | `/auth/github/login` | No | Initiate GitHub OAuth |
| `GET` | `/auth/github/callback` | No | OAuth callback |
| `POST` | `/auth/logout` | Yes | Clear session |
| `GET` | `/api/user` | Yes | Current user info |
| `POST` | `/api/repos` | Yes | Register a repo URL (enqueue clone) |
| `GET` | `/api/repos` | Yes | List user's repos |
| `GET` | `/api/repos/:id/status` | Yes | Clone/fetch status |
| `DELETE` | `/api/repos/:id` | Yes | Stop tracking a repo |
| `POST` | `/webhooks/github` | Signature | GitHub webhook receiver |

### 10.2 Existing Endpoints -- Repo-Scoped

| Current Path | SaaS Path |
|-------------|-----------|
| `/api/repository` | `/api/repos/:id/repository` |
| `/api/tree/:hash` | `/api/repos/:id/tree/:hash` |
| `/api/blob/:hash` | `/api/repos/:id/blob/:hash` |
| `/api/tree/blame/:hash` | `/api/repos/:id/tree/blame/:hash` |
| `/api/commit/diff/:hash` | `/api/repos/:id/commit/diff/:hash` |
| `/api/ws` | `/api/repos/:id/ws` |
| `/api/working-tree/diff` | **Removed** (bare clones have no working tree) |

### 10.3 Request/Response Examples

**POST /api/repos**
```json
// Request
{ "url": "https://github.com/torvalds/linux.git" }

// Response (201 Created -- new clone started)
{
  "id": "a1b2c3d4",
  "url": "https://github.com/torvalds/linux",
  "state": "cloning",
  "createdAt": "2026-02-21T10:00:00Z"
}

// Response (200 OK -- already exists, user has access)
{
  "id": "a1b2c3d4",
  "url": "https://github.com/torvalds/linux",
  "state": "ready",
  "commitCount": 1247834
}
```

**GET /api/repos/:id/status**
```json
{
  "id": "a1b2c3d4",
  "state": "cloning",
  "progress": "Receiving objects: 72% (1450/2013)",
  "startedAt": "2026-02-21T10:00:00Z"
}
```

### 10.4 Middleware Chain

```
Request -> CORS -> Rate Limiter (per-user) -> Auth (JWT) -> Repo (extract ID, verify access) -> Handler
```

---

## 11. Frontend Changes

### 11.1 New Routes

```
/               -> Landing page (URL input + login)
/dashboard      -> User's repo list
/r/:repoID      -> Repo visualization (existing UI)
```

### 11.2 New Components (Vanilla JS, consistent with existing stack)

- **Landing page**: URL input, GitHub OAuth button, recent repos from session
- **Dashboard**: Grid of user's repos with state badges, last accessed time, delete button
- **Loading overlay**: Progress bar during clone (`Receiving objects: 67%`)

### 11.3 Changes to Existing Files

**`web/backend.js`** -- scope API calls to repo ID:
```javascript
// Before
const response = await fetch("/api/repository");
const wsUrl = `${protocol}://${host}/api/ws`;

// After
const repoId = window.location.pathname.split("/r/")[1];
const response = await fetch(`/api/repos/${repoId}/repository`);
const wsUrl = `${protocol}://${host}/api/repos/${repoId}/ws`;
```

**`web/app.js`** -- add routing:
- If URL is `/r/:id`, render the existing graph UI with scoped backend URLs
- If URL is `/dashboard`, render the repo list
- Otherwise, render the landing page
- Hide working tree status panel (bare clones have no working tree)

### 11.4 What Does NOT Change

The entire visualization layer is untouched:
- `graph.js` and all D3 visualization code
- `sidebar.js`, `infoBar.js`, `search.js`, `keyboardShortcuts.js`
- `tooltips/`, `graph/layout/`, `graph/rendering/`
- `diffView.js`, `fileExplorer.js`
- The WebSocket message format (`{delta, status, head}`)

---

## 12. Data Model

### 12.1 Phase 1: SQLite

```sql
CREATE TABLE users (
    id              TEXT PRIMARY KEY,
    github_id       INTEGER UNIQUE NOT NULL,
    github_login    TEXT NOT NULL,
    email           TEXT,
    display_name    TEXT,
    avatar_url      TEXT,
    access_token    BLOB NOT NULL,           -- AES-256-GCM encrypted
    plan            TEXT DEFAULT 'free',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE repos (
    id              TEXT PRIMARY KEY,        -- SHA-256 prefix of canonical URL
    canonical_url   TEXT UNIQUE NOT NULL,
    display_url     TEXT NOT NULL,
    is_private      BOOLEAN DEFAULT FALSE,
    state           TEXT DEFAULT 'pending',  -- pending, cloning, ready, error, evicted
    error_message   TEXT,
    disk_path       TEXT,
    disk_bytes      INTEGER DEFAULT 0,
    commit_count    INTEGER DEFAULT 0,
    last_fetch_at   DATETIME,
    last_access_at  DATETIME,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE repo_access (
    repo_id         TEXT NOT NULL REFERENCES repos(id),
    user_id         TEXT NOT NULL REFERENCES users(id),
    role            TEXT NOT NULL DEFAULT 'viewer',
    granted_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (repo_id, user_id)
);

CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,
    repo_id         TEXT NOT NULL REFERENCES repos(id),
    type            TEXT NOT NULL,           -- 'clone', 'fetch'
    state           TEXT DEFAULT 'pending',  -- pending, running, done, failed
    progress        TEXT,
    started_at      DATETIME,
    finished_at     DATETIME,
    error_message   TEXT
);
```

### 12.2 Phase 2: Migrate to PostgreSQL

Same logical schema, adding:
- `subscriptions` table (Stripe integration)
- `oauth_connections` table (multi-forge support)
- `webhook_registrations` table
- `SKIP LOCKED` job claiming for worker pool
- Session table (replace JWT-only with revocable sessions)

---

## 13. Deployment on Fly.io

### 13.1 Phase 1: Single Machine

```toml
# fly.toml
app = "gitvista-saas"
primary_region = "iad"

[build]

[env]
  GITVISTA_DATA_DIR = "/data"
  GITVISTA_MODE = "saas"
  GITVISTA_LOG_FORMAT = "json"

[mounts]
  source = "gitvista_data"
  destination = "/data"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = false       # Must stay running for fetch tickers
  auto_start_machines = true
  min_machines_running = 1

  [http_service.concurrency]
    type = "connections"
    hard_limit = 500
    soft_limit = 400

[[vm]]
  memory = "1gb"
  cpu_kind = "shared"
  cpus = 2
```

### 13.2 Dockerfile Changes

```dockerfile
FROM golang:1.24-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gitvista ./cmd/vista

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata git openssh-client
COPY --from=build /gitvista /usr/local/bin/gitvista
RUN mkdir -p /data/repos
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["gitvista"]
CMD ["-host", "0.0.0.0"]
```

Key additions: `git` and `openssh-client` in the runtime image for clone/fetch operations.

### 13.3 Secrets

```bash
fly secrets set \
  GITHUB_CLIENT_ID=xxx \
  GITHUB_CLIENT_SECRET=xxx \
  GITVISTA_ENCRYPTION_KEY=xxx \    # 32-byte hex for AES-256
  GITVISTA_JWT_SECRET=xxx          # HMAC signing key
```

### 13.4 Phase 2: Separated Tiers

| Component | Fly Resource | Spec | Est. Cost |
|-----------|-------------|------|-----------|
| Web Tier (2x) | Fly Machine | shared-cpu-2x, 512MB | ~$15/mo |
| Worker Tier (1-2x) | Fly Machine + Volume | shared-cpu-4x, 1GB, 10GB vol | ~$25/mo |
| PostgreSQL | Fly Postgres (managed) | Shared, 1GB | ~$15/mo |
| Valkey | Upstash Redis | Pay-per-request, 256MB | ~$10/mo |
| **Total** | | | **~$65/mo** |

---

## 14. Implementation Phases

### Phase 1: MVP (Weeks 1-6) -- Public Repos, Single Binary

**Goal**: Paste a public Git URL, see the commit graph. No auth required.

| Week | Work |
|------|------|
| 1 | `gitcore` bare repo fix (`isBareRepo`). RepoManager: clone queue (bounded worker pool), directory management, eviction goroutine |
| 2 | RepoSession: extract per-repo server from current `Server`, scoped WebSocket broadcast, fetch ticker |
| 3 | SQLite schema + `modernc.org/sqlite` integration (pure Go, no CGO). Repo state machine. New `/api/repos` endpoints |
| 4 | API router: repo-scoped handlers, middleware chain. Progress reporting via WebSocket during clone |
| 5 | Frontend: landing page with URL input, clone progress overlay, redirect to `/r/:id`. Update `backend.js` for repo-scoped URLs |
| 6 | Integration testing. Deploy to Fly.io staging. Volume setup. End-to-end validation |

**Deferred**: Auth, private repos, webhooks, billing, multi-node.

### Phase 2: Auth + Private Repos (Weeks 7-10)

| Week | Work |
|------|------|
| 7 | GitHub OAuth: login/callback handlers, JWT session management, user table |
| 8 | Token encryption (AES-256-GCM). Private repo cloning with credential injection. ACL enforcement |
| 9 | Auth middleware. Per-user rate limiting. User dashboard (repo list, access management) |
| 10 | Error handling hardening. Clone failures, network errors, disk full, auth expiry |

### Phase 3: Production SaaS (Weeks 11-14)

| Week | Work |
|------|------|
| 11 | Separate web/worker tiers. PostgreSQL migration. Valkey cache integration |
| 12 | GitHub webhook receiver. Webhook registration via API. Immediate fetch on push |
| 13 | Monitoring: structured logs, Prometheus metrics, health check with repo stats. Alerting |
| 14 | Freemium enforcement (repo limits by tier). Landing/marketing page. Custom domain + TLS. Production deploy |

### Phase 4: Growth (Weeks 15-22+)

| Work | Description |
|------|-------------|
| GitLab/Bitbucket OAuth | Additional forge support with same patterns |
| Stripe billing | Subscription management, tier enforcement |
| Team features | Shared repo lists, team-level quotas, invite flow |
| Multi-region | Fly.io multi-region with `fly-replay` header routing |
| ObjectReader interface | Performance optimization for large repos (see Section 18) |

---

## 15. File Change Map

### Modified Files

| File | Change | Description |
|------|--------|-------------|
| `internal/gitcore/repository.go` | Minor | Add `isBareRepo()` check in `findGitDirectory()` |
| `cmd/vista/main.go` | Modified | New startup path for SaaS mode: init RepoManager, SQLite, auth config |
| `internal/server/server.go` | Modified | Top-level HTTP router; delegates repo-specific work to RepoSession |
| `internal/server/handlers.go` | Modified | Extract repo from context instead of `s.cached.repo` |
| `internal/server/websocket.go` | Modified | Route WebSocket connections to correct RepoSession |
| `internal/server/watcher.go` | Replaced | fsnotify watcher replaced by periodic fetch ticker |
| `internal/server/update.go` | Minor | `updateRepository()` logic moves into RepoSession |
| `web/backend.js` | Modified | Repo-scoped API URLs |
| `web/app.js` | Modified | Router: dashboard vs repo view |
| `fly.toml` | Modified | Volume mount, increased resources, auto_stop disabled |
| `Dockerfile` | Modified | Add git, openssh-client to runtime image |

### Removed

| File | Reason |
|------|--------|
| `internal/server/status.go` | Working tree status N/A for bare clones (endpoint hidden, not deleted) |

### New Files

| File | Purpose |
|------|---------|
| `internal/repomanager/manager.go` | Clone queue, fetch scheduler, LRU eviction |
| `internal/repomanager/session.go` | Per-repo server with gitcore.Repository + WS clients |
| `internal/repomanager/clone.go` | Git clone/fetch operations via `os/exec` |
| `internal/repomanager/scheduler.go` | Adaptive fetch scheduling |
| `internal/auth/oauth.go` | GitHub OAuth handlers |
| `internal/auth/jwt.go` | JWT creation/validation |
| `internal/auth/middleware.go` | Auth middleware |
| `internal/store/sqlite.go` | SQLite operations |
| `internal/store/migrations.go` | Schema migrations |
| `internal/store/crypto.go` | AES-256-GCM encrypt/decrypt |
| `web/dashboard.js` | Dashboard UI |
| `web/landing.js` | Landing page UI |

---

## 16. Risks & Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| **Disk exhaustion** | High | Partial clones (`--filter=blob:none`), 24h eviction, volume monitoring alerts, per-user repo limits |
| **Clone time for large repos** | Medium | Progress reporting via WebSocket, async clone, `--depth=50` for initial, deepen on demand |
| **Memory pressure** | Medium | `gitcore.NewRepository()` loads all commits into memory. Cap at ~50K commits per repo. Idle session eviction (15 min) |
| **Stale data** | Low | 30s fetch for active repos. Webhooks in v1 reduce to seconds |
| **Git binary dependency** | Low | Only for clone/fetch (2 commands). All reading is pure Go via gitcore |
| **Single point of failure (Phase 1)** | Medium | Single Fly machine. Mitigate with volume snapshots. Resolved in Phase 3 with multi-node |
| **Token compromise** | High | AES-256-GCM encryption at rest. Key in Fly secrets. Tokens scoped to minimum permissions. Never logged |
| **Malicious/huge repos** | Medium | Clone timeout (5 min), disk size check post-clone, per-tier size limits |
| **OAuth scope concern** | Medium | `repo` scope grants broad access. Document clearly. Consider GitHub Apps (fine-grained) in v2 |

---

## 17. Cost Estimation

### Per-Repo Costs

| Repo Size | Clone Disk | Clone Time | Fetch Time |
|-----------|-----------|------------|------------|
| Small (<1K commits) | ~5 MB | 2-5s | 1-3s |
| Medium (10K commits) | ~50 MB | 10-30s | 3-10s |
| Large (100K+ commits) | ~500 MB | 60-300s | 10-60s |

### Monthly Infrastructure by Scale

| Users | Active Repos | Infrastructure | Est. Cost |
|-------|-------------|---------------|-----------|
| 50 | 100 | 1 machine, 5GB vol | ~$30/mo |
| 200 | 500 | 2 machines, 20GB vol | ~$80/mo |
| 1,000 | 2,000 | 4 web + 3 worker, PG, Valkey | ~$200/mo |

At 1,000 users: **~$0.20/user/month** infrastructure cost. A $7/month Pro tier provides healthy margin.

---

## 18. Future: ObjectReader Interface (v2+)

For repos where clone time is unacceptable (monorepos, 1M+ commit repos), a future phase introduces an `ObjectReader` interface in `gitcore`:

```go
type ObjectReader interface {
    ReadObjectData(ctx context.Context, id Hash) (data []byte, objectType byte, err error)
    HasObject(ctx context.Context, id Hash) (bool, error)
    Close() error
}

type RefReader interface {
    ReadRefs(ctx context.Context) (map[string]Hash, error)
    ReadHEAD(ctx context.Context) (symref string, hash Hash, err error)
}
```

This enables a `RemoteObjectReader` that speaks Git smart HTTP v2 protocol to fetch objects on demand without cloning. Benefits:
- **3-5 second** time to first visualization (vs 30-120s for clone)
- **Zero disk usage** (in-memory cache only)
- Higher multi-tenant density

Trade-off: Building a Git protocol v2 client is significant work (~6-8 weeks). The packfile parser already exists in `pack.go`, but the transport layer, pkt-line framing, and pack negotiation are new. This is deferred because clone-and-serve works well for 95% of repos, and the `ObjectReader` interface can be added without breaking any existing code (the local path becomes `LocalObjectReader`, which is a pure refactor).

---

## Appendix: Architect Approach Comparison

| Dimension | A: Clone & Serve | B: Object Store | C: Hybrid |
|-----------|-----------------|----------------|-----------|
| **Code reuse** | ~90% | ~70% | ~85% |
| **gitcore changes** | 1 function fix | Major refactor | None |
| **Time to MVP** | 4-6 weeks | 8-10 weeks | 6-8 weeks |
| **First view latency** | 5-30s | 3-5s | 5-30s |
| **Disk per repo** | 5-500 MB | 0 (in-memory) | 5-500 MB |
| **Memory per session** | 10-100 MB | 10-256 MB | 10-100 MB (cache in Valkey) |
| **Scaling bottleneck** | Disk I/O | Memory | Infra complexity |
| **Best for** | MVP, fast ship | Large repos, latency-sensitive | Production SaaS at scale |

**Final architecture**: A -> C -> B (pragmatic MVP, production separation, then performance optimization).
