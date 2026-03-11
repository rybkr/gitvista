# Security Best Practices Report

## Executive Summary

I found seven security issues or notable security cut corners in this repository.

- Two are high severity: the hosted clone workflow is unauthenticated and can be used for server-side resource abuse, and bearer repo tokens are accepted in URL query strings and are also logged client-side.
- Four are medium severity: bearer tokens are stored in plaintext in Postgres, the bulk diff stats endpoint allows expensive CPU-heavy work per request, WebSocket connections are not capped per repository/session, and repo bearer tokens are persisted in browser storage.
- One is low severity: hosted repo URLs may use plaintext `http://`, which weakens clone and fetch integrity.

The most urgent fixes are to add admission control to repo creation and remove query-string token support for long-lived channels.

## High Severity

### SBP-001
- Rule ID: GO-AUTHZ-001
- Severity: High
- Location: [/Users/ryanbaker/projects/gitvista/internal/server/server.go:374](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L374), [/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go:39](/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go#L39), [/Users/ryanbaker/projects/gitvista/internal/server/hosted_store.go:19](/Users/ryanbaker/projects/gitvista/internal/server/hosted_store.go#L19)
- Evidence:
```go
// internal/server/server.go
func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	accountSlug := DefaultHostedAccountSlug
	switch r.Method {
	case http.MethodPost:
		s.handleAddRepo(w, r, accountSlug)
```
```go
// internal/server/repo_handlers.go
func (s *Server) handleAddRepo(w http.ResponseWriter, r *http.Request, accountSlug string) {
	...
	hostedRepo, err := s.hostedStore.AddRepo(accountSlug, req.URL)
```
```go
// internal/server/hosted_store.go
const DefaultHostedAccountSlug = "personal"
```
- Impact: Any unauthenticated visitor who can reach the hosted app can enqueue server-side clones and consume the shared global repo budget, disk, CPU, network, and background fetch capacity.
- Fix: Require authenticated callers for `POST /api/repos` and account-scoped repo creation, then enforce per-user/account quotas and per-account repo caps instead of a single anonymous global pool.
- Mitigation: Until full auth exists, gate repo creation behind a shared secret, signed invite token, IP allowlist, or edge auth; also lower `MaxRepos` and tighten rate limits for creation endpoints.
- False positive notes: If an upstream proxy already enforces authentication for all hosted routes, verify that it covers `POST /api/repos` and `/api/accounts/*/repos`.

### SBP-002
- Rule ID: GO-AUTH-002
- Severity: High
- Location: [/Users/ryanbaker/projects/gitvista/internal/server/hosted_auth.go:13](/Users/ryanbaker/projects/gitvista/internal/server/hosted_auth.go#L13), [/Users/ryanbaker/projects/gitvista/web/apiBase.js:21](/Users/ryanbaker/projects/gitvista/web/apiBase.js#L21), [/Users/ryanbaker/projects/gitvista/web/backend.js:45](/Users/ryanbaker/projects/gitvista/web/backend.js#L45), [/Users/ryanbaker/projects/gitvista/web/site/repoLoadingView.js:187](/Users/ryanbaker/projects/gitvista/web/site/repoLoadingView.js#L187), [/Users/ryanbaker/projects/gitvista/web/landing/repoBrowser.js:51](/Users/ryanbaker/projects/gitvista/web/landing/repoBrowser.js#L51)
- Evidence:
```go
// internal/server/hosted_auth.go
func hostedRepoToken(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get(hostedRepoTokenHeader))
	if token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get(hostedRepoTokenQuery))
}
```
```js
// web/apiBase.js
export function wsUrl() {
    const url = new URL(`${protocol}://${location.host}${base}/ws`);
    if (repoToken) {
        url.searchParams.set("access_token", repoToken);
    }
    return url.toString();
}
```
```js
// web/backend.js
logger?.info("Opening WebSocket connection", url);
```
```js
// web/site/repoLoadingView.js
url.searchParams.set("access_token", access.accessToken);
progressStream = new EventSource(url.toString());
```
- Impact: Repo bearer tokens can leak through browser devtools logs, reverse proxy/request logs, network monitoring, crash telemetry, and other URL-capture paths, allowing anyone who gets the token to access that hosted repo.
- Fix: Stop accepting `access_token` in the query string. For WebSockets and SSE, switch to short-lived server-minted channel tokens, authenticated cookies, or a one-time bootstrap exchange that upgrades to an opaque connection-specific secret. Remove client logging of token-bearing URLs.
- Mitigation: If query support must remain temporarily, redact query strings in all frontend/server logging and add explicit `Referrer-Policy` and cache controls everywhere token-bearing URLs are used.
- False positive notes: The server request logger currently logs only `r.URL.Path`, which reduces one leak path. It does not eliminate browser-side, proxy-side, or observability-side URL leakage.

## Medium Severity

### SBP-003
- Rule ID: GO-SECRETS-001
- Severity: Medium
- Location: [/Users/ryanbaker/projects/gitvista/internal/server/hosted_store_postgres.go:201](/Users/ryanbaker/projects/gitvista/internal/server/hosted_store_postgres.go#L201), [/Users/ryanbaker/projects/gitvista/internal/server/hosted_store_postgres.go:267](/Users/ryanbaker/projects/gitvista/internal/server/hosted_store_postgres.go#L267)
- Evidence:
```go
row := s.db.QueryRowContext(ctx, `
	SELECT id, account_slug, managed_repo_id, url, display_name, access_token, created_at
	FROM hosted_repositories
	WHERE account_slug = $1 AND id = $2 AND access_token = $3
`, account.Slug, repoID, accessToken)
```
```sql
CREATE TABLE IF NOT EXISTS hosted_repositories (
	...
	access_token TEXT NOT NULL UNIQUE,
	...
)
```
- Impact: A read-only database compromise exposes active bearer tokens directly, which gives immediate repository access without any further cracking or offline work.
- Fix: Store only a salted hash of repo access tokens, compare using the hash at authorization time, and return the raw token only once at creation.
- Mitigation: Rotate tokens on suspicious access, shorten token lifetime, and scope tokens to a specific repo plus short expiry.
- False positive notes: This does not matter for the in-memory store, but it is a real issue for Postgres-backed deployments.

### SBP-004
- Rule ID: GO-DOS-001
- Severity: Medium
- Location: [/Users/ryanbaker/projects/gitvista/internal/server/handlers_diff.go:108](/Users/ryanbaker/projects/gitvista/internal/server/handlers_diff.go#L108), [/Users/ryanbaker/projects/gitvista/internal/server/handlers_diff.go:145](/Users/ryanbaker/projects/gitvista/internal/server/handlers_diff.go#L145), [/Users/ryanbaker/projects/gitvista/internal/server/handlers_diff.go:214](/Users/ryanbaker/projects/gitvista/internal/server/handlers_diff.go#L214)
- Evidence:
```go
limit := parseBulkDiffStatsLimit(r)
...
sem := make(chan struct{}, 10)
...
entries, err := gitcore.TreeDiff(repo, parentTreeHash, c.Tree, "")
```
```go
const (
	defaultLimit = 3000
	maxLimit     = 20000
)
```
- Impact: A single authorized caller can force the server to compute diffs for thousands of commits on a large repository, driving sustained CPU and memory pressure and degrading the service for other users.
- Fix: Lower the maximum limit substantially, add execution budgeting/timeouts, require pagination, and precompute/cache summaries instead of allowing one request to fan out across up to 20,000 commits.
- Mitigation: Put this endpoint behind a stricter per-route rate limit and reject requests on repositories above a configured size threshold.
- False positive notes: The cache helps repeated identical requests, but the first request on a large repo is still expensive.

### SBP-005
- Rule ID: GO-WS-001
- Severity: Medium
- Location: [/Users/ryanbaker/projects/gitvista/internal/server/websocket.go:58](/Users/ryanbaker/projects/gitvista/internal/server/websocket.go#L58), [/Users/ryanbaker/projects/gitvista/internal/server/session_websocket.go:165](/Users/ryanbaker/projects/gitvista/internal/server/session_websocket.go#L165), [/Users/ryanbaker/projects/gitvista/internal/server/session_websocket.go:69](/Users/ryanbaker/projects/gitvista/internal/server/session_websocket.go#L69)
- Evidence:
```go
// internal/server/websocket.go
if !s.rateLimiter.allow(ip) {
	http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
	return
}
...
go session.clientReadPump(conn, done)
go session.clientWritePump(conn, done, writeMu)
```
```go
// internal/server/session_websocket.go
func (rs *RepoSession) registerClient(conn *websocket.Conn) *sync.Mutex {
	rs.clientsMu.Lock()
	rs.clients[conn] = writeMu
	clientCount := len(rs.clients)
	rs.clientsMu.Unlock()
```
```go
// internal/server/session_websocket.go
for conn, mu := range snapshot {
	...
	err2 = conn.WriteMessage(websocket.TextMessage, payload)
```
- Impact: Any token holder can accumulate large numbers of long-lived WebSocket connections over time; every broadcast is then fanned out to all of them, amplifying CPU, memory, and network cost per repository.
- Fix: Add a hard concurrent-connection cap per repo token/session and per client IP, and evict or reject excess connections.
- Mitigation: Reduce `pingPeriod` fan-out cost with stricter idle eviction and enforce connection quotas at the reverse proxy/load balancer.
- False positive notes: The current rate limiter only caps upgrade rate; it does not cap total concurrent WebSocket count.

### SBP-006
- Rule ID: JS-STORAGE-001
- Severity: Medium
- Location: [/Users/ryanbaker/projects/gitvista/web/site/hostedAccess.js:5](/Users/ryanbaker/projects/gitvista/web/site/hostedAccess.js#L5), [/Users/ryanbaker/projects/gitvista/web/site/hostedAccess.js:16](/Users/ryanbaker/projects/gitvista/web/site/hostedAccess.js#L16), [/Users/ryanbaker/projects/gitvista/web/site/hostedAccess.js:22](/Users/ryanbaker/projects/gitvista/web/site/hostedAccess.js#L22)
- Evidence:
```js
const raw = sessionStorage.getItem(STORAGE_KEY)
...
sessionStorage.setItem(STORAGE_KEY, JSON.stringify(value))
...
current[id] = {
    id,
    url: typeof url === "string" ? url : "",
    accessToken,
    savedAt: Date.now(),
}
```
- Impact: Hosted repo bearer tokens are persisted in browser storage for the lifetime of the tab, which enlarges the blast radius of any DOM XSS, malicious extension, or local browser compromise.
- Fix: Keep repo access tokens in memory only, or move to a server-managed `HttpOnly` session/capability model so browser JavaScript never handles long-lived bearer material directly.
- Mitigation: Shorten token lifetime, rotate tokens after sensitive actions, and scope tokens tightly to a single repo plus expiry.
- False positive notes: This is weaker than a direct XSS sink, but it is still a meaningful hardening gap for a bearer-token design.

## Low Severity

### SBP-007
- Rule ID: GO-TRANSPORT-001
- Severity: Low
- Location: [/Users/ryanbaker/projects/gitvista/internal/repomanager/clone.go:74](/Users/ryanbaker/projects/gitvista/internal/repomanager/clone.go#L74), [/Users/ryanbaker/projects/gitvista/internal/repomanager/clone.go:109](/Users/ryanbaker/projects/gitvista/internal/repomanager/clone.go#L109)
- Evidence:
```go
scheme := strings.ToLower(parsed.Scheme)
if scheme != "https" && scheme != "http" && scheme != "ssh" {
	return "", fmt.Errorf("unsupported scheme: %s", scheme)
}
...
return scheme + "://" + hostPart + path, nil
```
- Impact: Allowing `http://` clone URLs permits plaintext transport to allowed hosts, which weakens integrity and confidentiality for clone and fetch traffic in less-trusted networks.
- Fix: Reject `http://` remotes and require `https://` or `ssh://` only.
- Mitigation: If `http://` must remain for a controlled environment, gate it behind an explicit configuration flag that defaults off.
- False positive notes: If every allowed host immediately upgrades to HTTPS and the deployment network is fully trusted, the practical exposure is reduced. It is still a weaker default.

## Assumptions and Gaps

- I treated this as a static code audit of the repo. I did not verify whether an external proxy or platform auth layer protects the hosted routes.
- I did not find evidence of classic SQL injection, command injection, or path traversal in the reviewed server and repo-manager paths; the repo URL normalization and path sanitization are generally solid.
- I found a docs HTML trust boundary in [/Users/ryanbaker/projects/gitvista/internal/server/docs.go:94](/Users/ryanbaker/projects/gitvista/internal/server/docs.go#L94) and [/Users/ryanbaker/projects/gitvista/web/site/docsView.js:193](/Users/ryanbaker/projects/gitvista/web/site/docsView.js#L193), but I did not promote it to a primary finding because the docs content is repository-controlled rather than user-supplied in the current design.
