# Security Best Practices Report

## Executive Summary

I reviewed the full repository with a security focus across the Go backend, hosted repo-management flows, local/hosted WebSocket behavior, and the browser frontend.

The highest-risk issues are:

1. Hosted mode accepts credential-bearing Git URLs and then stores, logs, and re-exposes them.
2. Hosted mode has no visible authentication or ownership model, so any anonymous caller can enumerate, inspect, and delete repositories added by other users.
3. The frontend contains a DOM XSS sink that interpolates repository-controlled values into `innerHTML`.

I did not see hard-coded secrets in the repository itself. I did not run a live deployment test or external penetration test; findings below are grounded in repository code only.

## Critical

### SBP-001: Credential-bearing clone URLs are stored, logged, and exposed back to clients

- Rule ID: `SBP-001`
- Severity: Critical
- Location:
  - [internal/repomanager/clone.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/clone.go#L32)
  - [internal/repomanager/manager.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/manager.go#L246)
  - [internal/repomanager/manager.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/manager.go#L431)
  - [internal/repomanager/manager.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/manager.go#L500)
  - [internal/repomanager/clone.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/clone.go#L150)
  - [internal/server/repo_handlers.go](/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go#L54)
  - [internal/server/repo_handlers.go](/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go#L63)
  - [internal/server/repo_handlers.go](/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go#L88)
- Evidence:
  - `normalizeURL` says it “removes embedded credentials”, but that sanitized value is only used for hashing/deduplication.
  - `AddRepo` stores the original `rawURL` in `ManagedRepo.URL`.
  - `processClone` logs `repoURL := managed.URL` and passes that raw URL to `git clone`.
  - `handleAddRepo` and `handleListRepos` return the raw URL to clients.
- Impact: A user can submit `https://user:token@host/repo.git` and that credential can be retained in memory, logged, cloned with, and disclosed to any client that can call the hosted repo APIs.
- Fix: Reject credential-bearing URLs entirely, or strip `UserInfo` before persistence, logging, API responses, and clone execution. Store a redacted display URL separately from the effective transport URL if needed.
- Mitigation: Redact URL fields in logs immediately and treat any historical logs as potentially containing secrets.
- False positive notes: This is only non-exploitable if hosted mode is never exposed to untrusted users and credential-bearing URLs are operationally forbidden outside the codebase.

### SBP-002: Hosted mode has no visible authentication or ownership enforcement

- Rule ID: `SBP-002`
- Severity: Critical
- Location:
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L238)
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L247)
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L392)
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L424)
  - [internal/server/repo_handlers.go](/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go#L79)
  - [internal/server/repo_handlers.go](/Users/ryanbaker/projects/gitvista/internal/server/repo_handlers.go#L133)
- Evidence:
  - Hosted endpoints are wired directly behind rate limiting and optional CORS, with no auth middleware or caller identity checks visible in app code.
  - `GET /api/repos` lists all managed repos.
  - `DELETE /api/repos/{id}` deletes a repo without any ownership check.
  - Session-scoped repo routes accept any repo ID and expose repository data if the session can be created.
- Impact: Any anonymous internet client can enumerate repositories, access repository content by ID, observe clone progress, and delete repositories added by other users.
- Fix: Add authentication plus per-repository ownership or capability checks before listing, opening, streaming, or deleting hosted repos.
- Mitigation: If anonymous access is intentionally required, scope repositories to short-lived opaque share tokens rather than global IDs, and disable global listing/delete for unauthenticated callers.
- False positive notes: This finding assumes the hosted service is multi-user or publicly reachable. If an external gateway enforces auth, that control is not visible in this repository and should be verified separately.

### SBP-003: Repository-controlled analytics fields are inserted with `innerHTML`

- Rule ID: `SBP-003`
- Severity: Critical
- Location:
  - [web/analyticsView.js](/Users/ryanbaker/projects/gitvista/web/analyticsView.js#L1586)
  - [internal/server/analytics.go](/Users/ryanbaker/projects/gitvista/internal/server/analytics.go#L633)
  - [internal/server/analytics.go](/Users/ryanbaker/projects/gitvista/internal/server/analytics.go#L745)
- Evidence:
  - `renderHotspots` builds rows with:
    - `${row.path || "unknown"}`
    - `${row.topAuthor || "unknown"}`
    - `${row.recommendation || "Monitor changes."}`
  - Those values originate from repository file paths and commit author metadata gathered in analytics generation.
- Impact: A crafted repository path or author string can inject arbitrary HTML/JS into the hosted or local frontend when analytics hotspots are rendered.
- Fix: Replace the `innerHTML` row template with DOM construction using `textContent` and safe attribute setters.
- Mitigation: Add a strict CSP to reduce XSS impact, but do not treat CSP as a substitute for fixing the sink.
- False positive notes: This becomes exploitable whenever an attacker can influence repository contents or commit metadata viewed in GitVista.

## High

### SBP-004: Unknown repo IDs can escape the managed data directory during recovery

- Rule ID: `SBP-004`
- Severity: High
- Location:
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L400)
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L424)
  - [internal/repomanager/manager.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/manager.go#L271)
  - [internal/repomanager/manager.go](/Users/ryanbaker/projects/gitvista/internal/repomanager/manager.go#L296)
- Evidence:
  - `handleRepoRoutes` accepts the first URL segment as `id` without format validation.
  - `GetRepo` falls back to `recoverRepoFromDisk` when the ID is unknown.
  - `recoverRepoFromDisk` uses `filepath.Join(rm.cfg.DataDir, id)` directly.
- Impact: An `id` such as `..` resolves outside the managed repo directory. If the parent path is a valid Git repository, the server can open and expose it.
- Fix: Validate hosted repo IDs against the expected hash format before any lookup or recovery, and enforce that resolved paths stay within `DataDir`.
- Mitigation: Disable disk recovery for unknown IDs in hosted mode unless a validated registry entry exists.
- False positive notes: Exploitability depends on surrounding filesystem layout, but the traversal boundary break is real in code.

## Medium

### SBP-005: Third-party browser code and styles are loaded from public CDNs without integrity pinning

- Rule ID: `SBP-005`
- Severity: Medium
- Location:
  - [web/site/index.html](/Users/ryanbaker/projects/gitvista/web/site/index.html#L18)
  - [web/local/index.html](/Users/ryanbaker/projects/gitvista/web/local/index.html#L10)
  - [web/workbench.js](/Users/ryanbaker/projects/gitvista/web/workbench.js#L1)
  - [web/hljs.js](/Users/ryanbaker/projects/gitvista/web/hljs.js#L18)
  - [web/hljs.js](/Users/ryanbaker/projects/gitvista/web/hljs.js#L114)
- Evidence:
  - `dockview-core` CSS and ESM are fetched from jsDelivr.
  - `highlight.js` JS and CSS are fetched from cdnjs.
  - No SRI attributes or self-hosted pinning are present.
- Impact: A CDN compromise, tampering event, or trust-boundary failure would execute attacker-controlled code in client sessions with full origin privileges.
- Fix: Vendor or self-host these assets, or apply integrity-checked loading where the platform supports it.
- Mitigation: Add a CSP that narrows allowed script/style origins and review whether dynamic script injection can be eliminated.
- False positive notes: Version pinning reduces accidental drift but does not protect against compromised CDN responses.

### SBP-006: Security headers are not set in visible app code despite active third-party script/style loading

- Rule ID: `SBP-006`
- Severity: Medium
- Location:
  - [internal/server/middleware.go](/Users/ryanbaker/projects/gitvista/internal/server/middleware.go#L114)
  - [internal/server/server.go](/Users/ryanbaker/projects/gitvista/internal/server/server.go#L362)
  - [web/site/index.html](/Users/ryanbaker/projects/gitvista/web/site/index.html#L1)
  - [web/local/index.html](/Users/ryanbaker/projects/gitvista/web/local/index.html#L1)
- Evidence:
  - The visible middleware sets CORS headers only.
  - No CSP, `X-Content-Type-Options`, `Referrer-Policy`, or framing policy is emitted in app code.
  - The frontend loads remote scripts/styles, increasing the value of browser-side hardening.
- Impact: Browser exploit blast radius is larger than necessary, especially for XSS and third-party script compromise scenarios.
- Fix: Set a production header baseline in app code or explicitly document that the edge injects them and test that behavior in deployment.
- Mitigation: At minimum, add CSP and `X-Content-Type-Options: nosniff`; verify framing policy at the edge if embedding is not intended.
- False positive notes: These headers may be injected by Fly or another reverse proxy, but that is not visible in this repository and should be verified at runtime.

## Low

### SBP-007: Local-mode WebSocket origin policy trusts any loopback origin

- Rule ID: `SBP-007`
- Severity: Low
- Location:
  - [internal/server/websocket.go](/Users/ryanbaker/projects/gitvista/internal/server/websocket.go#L20)
- Evidence:
  - Local mode accepts any `Origin` whose hostname is `localhost` or a loopback IP, regardless of port or application.
- Impact: Another application already running on the victim’s machine and serving a loopback page can open a WebSocket to the local GitVista server and observe repository update traffic.
- Fix: Restrict local-mode origins to the exact bound host/port, or require a nonce/capability token for WebSocket upgrades.
- Mitigation: If permissive loopback access is intentional for development, document that tradeoff and keep it disabled in any broader local exposure mode.
- False positive notes: This is primarily relevant on developer machines running multiple local web applications.

## Recommended Fix Order

1. Fix `SBP-001` before any hosted deployment handling untrusted input.
2. Add an authentication and ownership model for `SBP-002`.
3. Fix the XSS sink in `SBP-003`.
4. Validate repo IDs and disable traversal in `SBP-004`.
5. Then address browser hardening and supply-chain exposure in `SBP-005` through `SBP-007`.

## Notes

- I did not modify protected files.
- I did not apply fixes in this pass; this report is intended to drive a one-finding-at-a-time remediation flow.
