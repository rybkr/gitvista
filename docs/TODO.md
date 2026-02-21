# GitVista Feature Roadmap

> Last updated: 2026-02-20 (Status audit: fixed B1/A1/B4/F6/A8 statuses, added infra issues, updated next sprint priorities)
> Methodology: Three parallel codebase audits covering Graph/Navigation, Diff/Code Understanding, and Infrastructure/Metadata themes. All ideas verified against the actual source files; corrections noted inline.
> RICE Score = (Reach × Impact × Confidence) / Effort. All metrics on a 1–10 scale where Effort 10 = very high effort.

---

## COMPETITIVE ANALYSIS — Git Explorer Landscape

### What GitVista does well

- **Pure Go git parsing** — reads loose objects, pack files (v1/v2), refs, tags, stashes directly with no libgit2 dependency
- **Real-time updates** — fsnotify watcher + WebSocket broadcasting on every `.git` change (unique differentiator)
- **Two graph layouts** — lane-based (git-style swimlanes) and force-directed (D3 physics simulation)
- **File explorer** with lazy tree loading, keyboard navigation (W3C APG TreeView), and blame annotations
- **Unified diff viewer** with file-level and commit-level views, rename detection, and configurable context
- **Commit search**, graph filtering, dark/light/system theme, working tree status

### Feature comparison with other git explorers

| Feature | GitKraken / Sourcetree | gitk / tig | GitVista |
|---|---|---|---|
| Full commit history graph | Yes | Yes | Yes |
| File/tree browsing | Yes | Yes | Yes |
| Blame (line-level) | Yes | Yes | Entry-level only |
| Log filtering (author, date range, path) | Yes | Yes | Message/author/hash only |
| Submodule exploration | Yes | Partial | No |
| Reflog viewer | Yes | Yes | No |
| Worktree support | Some | Yes | No |
| Stash contents/diff | Yes | Yes | Listed only |
| Interactive rebase visualization | GitKraken | No | No |
| Commit signature/GPG verification | Yes | Yes | No |
| Multi-repo | Some | No | No |
| File history (follow renames) | Yes | Yes | No |
| **Real-time live updates** | No | No | **Yes** |
| **Web-based (zero install for viewers)** | No | No | **Yes** |
| **Dual graph layout strategies** | No | No | **Yes** |

### Key gaps to close for "most comprehensive explorer"

1. **Line-level blame** (C4) — the most common form of blame; current entry-level blame is a starting point
2. **File history with rename following** — not yet on the roadmap; would require commit-walk per file path
3. **Reflog viewer** — not yet on the roadmap; reflog data is partially parsed for stashes
4. **Richer log filtering** (author, date range, path) — current search covers message/author/hash only
5. **Stash contents/diff viewing** — stashes are listed but their contents cannot be explored

### Bottom line

GitVista is a **strong git explorer** with a unique real-time web angle that most tools lack. Its pure-Go parser and live-updating WebSocket architecture are genuinely differentiating. For "most comprehensive explorer," the biggest gaps are line-level blame, file history, reflog viewing, and richer log filtering.

---

## SHIPPED

The following were listed as future work in the original roadmap but are **fully implemented**:

### Originally Shipped
| Feature | Where it lives |
|---------|---------------|
| **Commit permalink deep links** (`#<hash>` in URL) | `getHashFromUrl()` + `history.replaceState` in `web/app.js` (lines 13–19, 101–106, 160–168) |
| **Configurable context lines in diff view** | Backend: `?context=N` in `handleFileDiff` (`internal/server/handlers.go` lines 334–341). Frontend: expand buttons, `CONTEXT_EXPAND_STEP`, and URL re-fetch in `web/diffContentViewer.js` lines 17–164 |

### Feature Sprint 1 (Commit 992bc92+)
The following high-RICE roadmap items have been completed and merged to main:

| Priority | ID | Feature | RICE | Status | Commit |
|----------|----|---------|----|--------|--------|
| 1 | A1 | **Wire `getAuthorColor` into renderer** | 560 | ⚠️ PARTIAL | 05b7a91. Author colors used for merge diamonds and lane mode (`laneColor`), but regular commits in force mode still use `this.palette.node`. `renderNormalCommit()` at `graphRenderer.js:433` uses `node.laneColor \|\| this.palette.node`, not `getAuthorColor()`. |
| 2 | F1 | **HTTP Timeouts and Graceful Shutdown** | 252 | ✅ SHIPPED | (merged via b63c6e0) |
| 3 | F3 | **Theme-Pinning Toggle** (light/dark/system) | 243 | ✅ SHIPPED | 311442f |
| 5 | A4 | **Progressive Commit Detail at Zoom** | 158 | ✅ SHIPPED | 9a1637e |
| 6 | B2 | **Rename Detection in Diffs** | 149 | ✅ SHIPPED | (merged via b63c6e0) |

### Feature Sprint 2 (dev branch)
| Priority | ID | Feature | RICE | Status | Notes |
|----------|----|---------|----|--------|-------|
| 12 | F4 | **LRU Cache Eviction** | 112 | ✅ SHIPPED | Bounded LRU cache in `internal/server/cache.go` replaces unbounded sync.Map. Configurable via `GITVISTA_CACHE_SIZE` env var (default: 500). |
| 8 | F6 | **Structured Logging** | 135 | ✅ SHIPPED (server only) | `log/slog` in `internal/server/`. `internal/gitcore/` still has ~10 `log.Printf` calls in refs.go, objects.go, pack.go that bypass slog/level filtering. |
| 13 | A2 | **Commit Search and Filtering** | 98 | ✅ SHIPPED | Debounced search bar in `web/search.js`. Searches message, author, email, hash. Opacity-based dimming of non-matching nodes. `/` keyboard shortcut. |
| 14 | A3 | **Configurable Graph Display Filters** | 98 | ✅ SHIPPED | Filter panel in `web/graphFilters.js`. Hide remote branches, merge commits, stashes. Branch focus selector with BFS reachability. Compound filter predicates with search. State persisted to localStorage. |
| — | — | **Lane-Based Layout Strategy** | — | ✅ SHIPPED | Dual layout system: force-directed (original) and lane-based DAG layout. Strategy pattern in `web/graph/layout/`. Toolbar buttons for switching. Persisted to localStorage. |
| 4 | B1 | **Inline Syntax Highlighting in Diff View** | 168 | ⬜ NOT STARTED | highlight.js is loaded in `fileContentViewer.js` but `diffContentViewer.js` has zero hljs integration — all diff lines render as plain text via `textContent`. |

### Frontend Design Upgrade (Commit 992bc92)
**Refined Geometric Modernism** visual overhaul:
- Typography system: Geist for UI, JetBrains Mono for code elements
- Color palette refinement: vibrant teal primary (#0ea5e9) with better contrast
- Component design: improved shadows, rounded corners, spacing, and transitions
- Interactive enhancements: smooth hover states, backdrop blur, focus rings
- Accessibility: WCAG AA compliance, better visual hierarchy, larger targets

---

## BRAINSTORM

### Theme A — Graph Visualization & Interaction

---

**A1. Wire `getAuthorColor` into the Canvas Renderer** *(completing existing scaffolding)*

`web/utils/colors.js` exports `getAuthorColor(email)` using a djb2 hash mapped to `hsl(H, 65%, 55%)`, with results memoized in a module-level `colorCache` Map. This function is **never imported anywhere** — it is dead code. The graph renderer in `web/graph/rendering/graphRenderer.js` paints all commit nodes with `this.palette.node` (a uniform theme color). The wiring is a one-line change in `renderNormalCommit`. No design or backend work needed.

**User problem solved:** All commit nodes look identical; you cannot visually distinguish authorship at a glance.

---

**A2. Commit Search and Filtering**

A debounced search bar (author, message, hash prefix) that highlights matching nodes and dims non-matching ones. All data — `Message`, `Author.Name`, `Author.Email`, `ID` — is already present on every graph node via `node.commit` in `graphController.js`.

**Correction from original:** A naive approach that removes non-matching entries from `state.commits` would discard D3 node positions on clear. The correct implementation uses a derived `visibleCommits` set to control what `reconcileCommitNodes` iterates over while leaving `state.commits` untouched, preserving positions. Alternatively, opacity-based hiding (nodes stay in simulation, rendered transparent) avoids the position-preservation problem entirely and is lower effort.

The `/` keyboard shortcut in `keyboardShortcuts.js` currently targets the file explorer filter — this feature would share or redirect that shortcut.

**User problem solved:** Cannot locate a specific commit in a dense graph of 50+ commits.

---

**A3. Configurable Graph Display Filters**

A filter panel with toggles: show/hide remote branches, stash nodes, merge commits (detected via `Parents.length > 1`), and a branch selector to highlight only commits reachable from a chosen branch.

**Correction from original:** The filter hook must sit at the `reconcileCommitNodes` boundary (inside `updateGraph`), not at render time — `graphRenderer.js` renders everything in `nodes[]` unconditionally with no visibility flag support.

**User problem solved:** Visual clutter in repos with many remotes or stashes.

---

**A4. Progressive Commit Detail at Zoom Levels**

The zoom-threshold pattern for showing the commit message first-line already exists in `graphRenderer.js` (`renderCommitLabel`, around `COMMIT_MESSAGE_ZOOM_THRESHOLD = 1.5` from `constants.js`). Extend it: at zoom ≥ 2.0 show `node.commit.author.name` below the message; at zoom ≥ 3.0 show the relative date from `node.commit.author.when`. Pure frontend, no backend.

**User problem solved:** Users zooming into a bug window cannot read author or date context without clicking each node individually.

---

**A5. Export Graph as PNG**

`canvas.toDataURL("image/png")` on the existing canvas in `graphController.js`. The `devicePixelRatio` scaling already applied in `resize()` means the export captures full physical resolution on HiDPI screens. A toolbar button triggers a download.

**Correction from original:** PNG and SVG should be treated as separate features. PNG is trivial (~1 hour). SVG requires a parallel renderer (no SVG path exists anywhere in the codebase today) and is low priority.

**User problem solved:** Cannot share the commit graph in documentation, PRs, or presentations.

---

**A6. Keyboard Branch Jump (`b` key)**

Pressing `b` opens a lightweight overlay that fuzzy-filters branch names from `state.branches` (a `Map<name, hash>` in `graphState.js`). Selecting a branch calls `graph.centerOnCommit(targetHash)`. Pattern mirrors the file explorer filter. `keyboardShortcuts.js` has a clean extension point. No backend needed.

**User problem solved:** Cannot quickly jump to a specific branch in the graph without scanning the canvas.

---

**A7. Minimap Navigation Panel**

A small fixed overlay (≈160×120px) rendering a scaled-down view of the full commit graph with a highlighted viewport rectangle. Clicking/dragging on the minimap translates the main `zoomTransform`. A second `<canvas>` element, re-rendered using the existing node positions at a fixed small scale.

**User problem solved:** Users lose spatial context when zoomed into a dense region and cannot tell where they are in the overall graph.

---

**A8. Graph Layout Mode Toggle: Force-Directed vs. DAG Lanes**

A toggle between the current layout and a structured Sugiyama-style layout where branches render as parallel vertical lanes (like `git log --graph`).

**Correction from original:** The existing `LayoutManager` already applies a chronological Y-position (`applyTimelineLayout`) at startup — the current layout is a hybrid, not purely force-directed. A true DAG lane mode requires computing branch-lane X assignments, which must handle merge commits and crossing minimization. This is a significant algorithmic problem, not a class rename. The `Rebalance` button already resets to force-directed mode.

**User problem solved:** Force-directed horizontal positions are non-deterministic; parallel branches are hard to follow.

---

**A9. Interactive Rebase Preview** *(long-horizon)*

A read-only "what-if" mode: drag a branch pill onto another commit, render ghosted shadow nodes showing the post-rebase topology. Purely visual, no git commands executed.

**Note:** GitVista is read-only by design (no write-capable backend). Computing a correct rebase preview requires new DAG-rewrite logic and is blocked by A8 (DAG layout) being useful first.

---

### Theme B — Diff & Code Review

---

**B1. Inline Syntax Highlighting in Diff View**

Apply highlight.js to diff content in `diffContentViewer.js`.

**Correction from original:** Highlight.js 11.9.0 is **already loaded** in `fileContentViewer.js` (lazy CDN load, lines 15–38) and applied to blob content. The CDN dependency and integration pattern are already solved. The remaining work is extending it to the diff renderer: tokenize each `DiffLine.content` through `hljs.highlight()` per-line (or wrap hunks in `<code>` elements) and detect language from the file path extension (already done in `fileContentViewer.js`). Prism.js is not needed.

**User problem solved:** Raw diff text without syntax color is harder to parse for structure and intent.

---

**B2. Rename Detection in Diffs**

After collecting deleted and added entries in `TreeDiff` (`internal/gitcore/diff.go`), a post-processing pass matches pairs where `OldHash == NewHash` (exact-match rename detection) and sets `DiffStatus = DiffStatusRenamed` and `OldPath`.

**Key finding:** `DiffStatusRenamed` and `DiffEntry.OldPath` already exist in `internal/gitcore/types.go` (line 223, 244). In the frontend, `diffView.js` and `diffContentViewer.js` already have `"renamed"` badge handling stubbed out. The backend exact-match pass is the only missing piece.

**User problem solved:** Renamed files appear as a deletion + addition, obscuring intent in refactoring commits.

---

**B3. Diff Search / Find in Diff**

A text search input within `diffContentViewer.js` that highlights matching substrings across all visible diff lines. Enter cycles through matches; a counter shows "3 of 12." Pure frontend — `DiffLine.content` strings are already in the DOM.

**User problem solved:** Cannot find a specific identifier or string in a large diff without scrolling manually.

---

**B4. Working Tree Diff Context Expansion**

The expand-context mechanism in `diffContentViewer.js` (already shipping for commit diffs) now works for working-tree diffs. `ComputeWorkingTreeFileDiff` in `internal/gitcore/worktree_diff.go` accepts a `contextLines` parameter that controls the number of unchanged lines included around each hunk. The server handler reads `?context=N` parameter and passes it to the gitcore function.

**User problem solved:** Cannot expand context in working-tree diffs despite the UI control existing. **[COMPLETED via Phase 3: Pure Working Tree Diff]**

---

**B5. Diff Statistics Flame Bar per File**

In `diffView.js`'s file list, show a proportional green/red bar and `+N / -M` counts per changed file (the GitHub PR flame bar pattern).

**Note:** `DiffEntry` does not currently include per-file insertion/deletion counts — only aggregate `DiffStats` at the `CommitDiff` level. This requires calling `ComputeFileDiff` per entry at the list stage, which adds latency for large commits. A reasonable approach: compute counts only for text files under `maxBlobSize`.

**User problem solved:** Cannot triage which files were most impacted in a commit without opening each diff individually.

---

**B6. Split-Pane Side-by-Side Diff View**

A "Split" toggle in the diff toolbar that renders the same `FileDiff`/`DiffHunk` data in a two-column layout (old on left, new on right) instead of unified. All data (`OldLine`, `NewLine`, `type`) is already in the API response. A pairing pass is needed to align deletion lines with their corresponding additions.

**User problem solved:** Unified diff is harder to follow for structural changes.

---

**B7. Commit Range Diff (Two-Commit Comparison)**

New `GET /api/commit/range-diff?from={hash}&to={hash}` in `handlers.go`. The backend primitive (`TreeDiff` with two arbitrary tree hashes) already exists in `diff.go`.

**Correction from original:** Both the server route registration in `server.go` and the frontend state machine in `diffView.js` (which currently only handles single-commit diffs) require changes, not just a new handler.

**User problem solved:** Cannot compare two arbitrary commits to understand the total delta across a feature branch.

---

### Theme C — Code Understanding & Blame

---

**C1. Clickable Blame Hash → Graph Navigation**

In the file explorer's blame column, `entry.blame.commitHash.substring(0, 7)` is rendered as plain text (`fileExplorer.js` line 588). Make it a clickable link that calls a `navigateToCommit(hash)` method on the graph controller, centering the graph on the responsible commit. A callback pattern (same as `diffContentViewer.onBack()`) is the cleanest integration.

**User problem solved:** Blame data is dead — you can see which commit touched a file but cannot act on it.

---

**C2. Annotated Tag Details Panel**

Clicking a tag pill on a commit node opens a tooltip showing the tagger, date, and full message (for annotated tags).

**Correction from original:** No `tagTooltip.js` exists. The `TooltipManager` in `web/tooltips/index.js` only registers `commit` and `branch` tooltips. No `GET /api/tag/{name}` endpoint exists. However, the `Tag` struct in `types.go` (lines 84–91) is fully parsed in memory with `Tagger`, `Message`, and `Object`. The backend data is available; both the endpoint and the frontend tooltip are absent and must be built. Canvas hit-testing for tag pill click areas also needs to be wired.

**User problem solved:** Cannot see tag message, tagger identity, or creation date — only the tag name label.

---

**C3. Submodule Awareness and Navigation**

Detect mode `160000` entries in the file explorer (already identified by `isSubmodule()` in `diff.go`), show a distinct icon, and display submodule URL and pinned commit hash on click via a new `GET /api/submodules?commit={hash}` endpoint that parses `.gitmodules`.

**User problem solved:** Submodule entries in the file explorer are dead-ends — clicking them does nothing.

---

**C4. Line-Level Blame in File Viewer** *(strategic, validate first)*

Per-line blame showing which commit last changed each line, with a toggle in `fileContentViewer.js`.

**Correction from original:** `GetLineBlame()` does not exist anywhere in the codebase. The existing `GetFileBlame()` operates at directory-entry granularity (which file changed), not line granularity. True line-level blame requires a fundamentally different algorithm — repeatedly comparing blob diffs across commit ancestors. The project has committed to pure-Go parsing (no git CLI). A custom line-blame algorithm or cached blame computation is the intended path forward.

**User problem solved:** Cannot see who wrote each line of a file — the most common form of blame.

---

### Theme D — Navigation & Deep Linking

---

**D1. Compound URL Deep Links for Diff and File State**

Extend the existing `#<commitHash>` fragment scheme (already shipping) to encode sidebar state: `#<commitHash>/diff/<filePath>` or `#<commitHash>/tree/<filePath>`. On load, restore the active tab and open file/diff. `sidebarTabs.js` already supports programmatic tab switching.

**User problem solved:** Cannot share a URL that opens a specific file diff. The recipient always lands on a blank sidebar.

---

**D2. Backend Full-Text Search Index** *(depends on D1 proving insufficient)*

An in-memory trigram/token inverted index over commit messages and author names, rebuilt on `loadObjects()` and updated incrementally in `update.go`. Exposed as `GET /api/search/commits?q=<query>&limit=50`.

**Note:** Ship A2 (frontend commit search) first and evaluate. The entire `state.commits` Map with all messages is already in memory on the frontend — backend indexing is only necessary at ~10k+ commits where JS Map iteration becomes slow.

**User problem solved:** Frontend commit search breaks down at scale.

---

### Theme E — Repository Metadata & Statistics

---

**E1. Worktree Support and Navigation**

Enumerate linked worktrees by reading `.git/worktrees/`, expose via `GET /api/worktrees`, and surface a worktree switcher in `infoBar.js`.

**Correction from original:** Partial implementation already exists — `handleGitFile` in `repository.go` (lines 317–340) already resolves `.git` file pointers, meaning GitVista correctly loads a linked worktree if you point `-repo` at it directly. The missing piece is automatic enumeration of sibling worktrees from the primary `.git/worktrees/` directory.

**User problem solved:** Cannot navigate between linked worktrees.

---

**E2. Repository Statistics Dashboard**

A "Statistics" sidebar tab: commit frequency histogram, top contributors by commit count, file churn heatmap, merge ratio. A new `GET /api/stats` endpoint computes aggregations from the in-memory `r.commits` map. D3.js is already loaded.

**Correction from original:** Computing per-file churn stats requires running `TreeDiff` across all commits — an O(n × avg_files_changed) scan that could take seconds on first load for large repos. Results must be cached and computed asynchronously. The `Repository.Commits()` method also allocates a new map copy on every call — the handler should call it once and hold the reference.

**User problem solved:** No high-level view of commit velocity, contributor activity, or file churn.

---

### Theme F — Infrastructure & Developer Experience

---

**F1. HTTP Timeouts and Graceful Shutdown**

Replace the bare `http.ListenAndServe(s.addr, nil)` call in `server.go` (line 83, acknowledged with `//nolint:gosec` comment) with an `http.Server{ReadTimeout, WriteTimeout, IdleTimeout}` struct. Add `os/signal` SIGINT/SIGTERM handling in `main.go` that calls `s.Shutdown()` — which is fully implemented (lines 87–103 in `server.go`) but never wired to any signal.

**User problem solved:** A hung blob request can tie up the server indefinitely. Ctrl-C kills the process without gracefully closing WebSocket connections or draining the watcher goroutine.

---

**F2. Configurable Watcher Debounce and Polling Fallback**

The debounce is hard-coded as `const debounceTime = 100 * time.Millisecond` in `watcher.go` (line 11). `GITVISTA_POLL_INTERVAL` does not exist. On Linux with Docker bind-mounts or NFS, `fsnotify` fires unreliably or not at all. A polling path that re-reads `.git/HEAD` and `packed-refs` modification timestamps on a configurable interval would serve as a fallback when `fsnotify.NewWatcher()` fails.

**User problem solved:** GitVista silently goes stale in Docker bind-mount deployments — a growing use case.

---

**F3. Theme-Pinning Toggle (Light / Dark / System)**

A three-state theme toggle persisted to `localStorage`. `styles.css` uses `@media (prefers-color-scheme: dark)` with a complete CSS custom property system (`--bg-color`, `--surface-color`, etc.) for both themes, but provides no user override. Adding `:root.theme-dark` and `:root.theme-light` class blocks alongside the media query blocks is the full implementation. The canvas renderer reads CSS variables at render time, so class-based overrides propagate automatically.

**User problem solved:** Users who want dark mode while their OS is in light mode (or vice versa) have no recourse.

---

**F4. LRU Eviction for Blame and Diff Caches**

`server.go` holds `blameCache sync.Map` and `diffCache sync.Map` (lines 34–35). Both grow without bound — every `commitHash:dirPath` and `commitHash:filePath:ctxN` key is stored forever. For large repos browsed over extended sessions this is an unbounded memory leak. A small bespoke LRU implementation in `internal/server/cache.go` (to avoid new dependencies) with a configurable max size (`GITVISTA_CACHE_SIZE`) should replace both maps.

**User problem solved:** Long-running GitVista sessions on large repos accumulate unbounded cache memory.

---

**F5. Graph Keyboard Navigation (Accessibility)**

The graph canvas (`<canvas>` with `touch-action: none`) is not keyboard-focusable or navigable. `fileExplorer.js` implements the W3C APG TreeView keyboard pattern and `indexView.js` handles Enter/Space — but the primary visualization surface has no keyboard access. Add `tabindex="0"` and `role="application"` to the canvas wrapper, implement keydown handlers in `graphController.js` to track a "focused commit" in `graphState.js`, and render a focus ring in `graphRenderer.js` (following the existing HEAD ring pattern).

**User problem solved:** Keyboard-only and assistive-technology users cannot interact with the core feature.

---

**F6. Structured Logging with Configurable Log Levels**

Replace scattered `log.Printf` calls (in `server.go`, `watcher.go`, `broadcast.go`, `update.go`, `websocket.go`) and the ANSI-colored prefix constants in `types.go` (lines 8–13) with Go 1.21's stdlib `log/slog`. Add `GITVISTA_LOG_LEVEL` (debug/info/warn/error) and `GITVISTA_LOG_FORMAT` (text/json) env vars parsed in `main.go`. Zero new dependencies — `log/slog` is stdlib.

**User problem solved:** ANSI escape codes pollute piped logs; no way to suppress debug noise or pipe structured output to a log aggregator in CI/CD dashboard deployments.

---

**F7. Mobile and Narrow-Viewport Responsive Layout**

`styles.css` has no `@media` width breakpoints — all layout rules are fixed-pixel. The sidebar uses a fixed width from `localStorage`. On narrow viewports (tablets, small laptops at 100% zoom), the sidebar and graph compete for space. Add CSS breakpoints so the sidebar collapses to a drawer on viewports < 768px. D3 touch zoom in `graphController.js` also needs explicit touch event verification on mobile.

**User problem solved:** GitVista is unusable on tablet or secondary devices due to rigid fixed-width layout.

---

## PRIORITIZED ROADMAP

### RICE Scoring

- **Reach (1–10):** Share of users affected. 10 = all users always.
- **Impact (1–10):** Quality of experience improvement for affected users. 10 = transformative.
- **Confidence (1–10):** Certainty of Reach and Impact estimates and implementation feasibility.
- **Effort (1–10):** Engineering cost. 10 = multiple weeks of non-trivial work. 1 = hours.
- **RICE = (Reach × Impact × Confidence) / Effort**

Strategic flags: items with outsized long-term value that may score lower due to effort.

---

| Priority | ID | Feature | Reach | Impact | Conf | Effort | RICE | Status | Notes |
|----------|----|---------:|:-----:|:------:|:----:|:------:|:----:|:------:|-------|
| 1 | A1 | **Wire `getAuthorColor` into renderer** | 8 | 7 | 10 | 1 | **560** | ⚠️ PARTIAL | Used for merge diamonds and lane mode, but `renderNormalCommit()` in force mode uses `node.laneColor \|\| this.palette.node` — not `getAuthorColor()`. |
| 2 | F1 | **HTTP Timeouts and Graceful Shutdown** | 8 | 7 | 9 | 2 | **252** | ✅ DONE | `Server.Shutdown()` fully implemented and just needs an OS signal handler in `main.go`. Swap `ListenAndServe` for `http.Server{...}`. Correctness fix, not a feature. |
| 3 | F3 | **Theme-Pinning Toggle** | 9 | 6 | 9 | 2 | **243** | ✅ DONE | CSS custom property system is complete for both themes. Class-based override (`data-theme`) alongside existing media query. Pure frontend, no backend. |
| 4 | B1 | **Inline Syntax Highlighting in Diff View** | 8 | 7 | 9 | 3 | **168** | ⬜ NOT STARTED | highlight.js loaded in `fileContentViewer.js` but `diffContentViewer.js` renders all lines as plain text via `textContent`. Zero hljs integration in diffs. |
| 5 | A4 | **Progressive Commit Detail at Zoom** | 7 | 5 | 9 | 2 | **158** | ✅ DONE | Zoom threshold pattern exists in `renderCommitLabel`. Author name and date on `node.commit.author`. Zero backend changes. |
| 6 | B2 | **Rename Detection in Diffs** | 7 | 8 | 8 | 3 | **149** | ✅ DONE | `DiffStatusRenamed`, `DiffEntry.OldPath`, and frontend badge stubs already exist. Backend exact-hash post-processing pass is the only new work. |
| 7 | B4 | **Working Tree Diff Context Expansion** | 5 | 6 | 9 | 2 | **135** | ✅ DONE | Pure Go implementation in `ComputeWorkingTreeFileDiff()` supports context parameter. No git CLI shell-out. |
| 8 | F6 | **Structured Logging** | 5 | 6 | 9 | 2 | **135** | ✅ DONE (server) | `log/slog` in `internal/server/`. `internal/gitcore/` still has ~10 `log.Printf` calls that bypass slog. |
| 9 | A5 | **Export Graph as PNG** | 6 | 5 | 9 | 2 | **135** | `canvas.toDataURL("image/png")` — DPR scaling already correct. Add a download button to the graph toolbar. |
| 10 | C1 | **Clickable Blame Hash → Graph Navigation** | 7 | 8 | 9 | 4 | **126** | Pure frontend wiring. Expose `navigateToCommit(hash)` from `graphController.js`; call it from `fileExplorer.js` blame column click handler. |
| 11 | F2 | **Watcher Polling Fallback** | 7 | 8 | 8 | 4 | **112** | Docker bind-mount is a primary deployment target. `GITVISTA_POLL_INTERVAL` env var entirely absent. Debounce constant needs to become a config field. |
| 12 | F4 | **LRU Cache Eviction** | 6 | 7 | 8 | 3 | **112** | ✅ DONE | Bounded LRU in `internal/server/cache.go`. `GITVISTA_CACHE_SIZE` env var (default: 500). |
| 13 | A2 | **Commit Search and Filtering** | 8 | 7 | 7 | 4 | **98** | ✅ DONE | `web/search.js` with opacity-based dimming. `/` keyboard shortcut. |
| 14 | A3 | **Configurable Graph Display Filters** | 7 | 6 | 7 | 3 | **98** | ✅ DONE | `web/graphFilters.js` with compound filter predicates. localStorage persistence. |
| 15 | A6 | **Keyboard Branch Jump** | 7 | 6 | 7 | 3 | **98** | `b` key + overlay branch picker. `state.branches` Map has all data. Follows file-explorer filter pattern. |
| 16 | B3 | **Diff Search / Find in Diff** | 6 | 7 | 8 | 4 | **84** | Pure frontend. `DiffLine.content` strings are in the DOM. Precedent in `fileExplorer.js` filter. |
| 17 | D1 | **Compound URL Deep Links** | 6 | 7 | 7 | 4 | **74** | Commit-level permalink already ships. Extend to sidebar state (tab + file + diff). Requires restoration sequencing across async loads. |
| 18 | F5 | **Graph Keyboard Navigation** | 6 | 8 | 7 | 5 | **67** | Primary surface is keyboard-inaccessible. Canvas focus + keydown routing + focus ring in `graphRenderer.js`. |
| 19 | E1 | **Worktree Support** | 5 | 8 | 7 | 5 | **56** | `.git` file pointer resolution already exists in `repository.go`. Remaining: `.git/worktrees/` enumeration + `infoBar.js` switcher. |
| 20 | B6 | **Split-Pane Side-by-Side Diff** | 6 | 7 | 7 | 5 | **59** | Re-render existing `DiffHunk` data in two columns. Deletion/addition pairing pass needed. No backend changes. |
| 21 | C2 | **Annotated Tag Details Panel** | 5 | 6 | 7 | 4 | **52** | Tag data in memory. Neither `GET /api/tag/{name}` endpoint nor `tagTooltip.js` exists. Canvas hit-testing for tag pill adds effort. |
| 22 | A7 | **Minimap Navigation Panel** | 6 | 7 | 6 | 5 | **50** | Second canvas + scaled render of all nodes. Viewport rect from `zoomTransform`. Useful for large repos. |
| 23 | E2 | **Repository Statistics Dashboard** | 7 | 7 | 6 | 7 | **42** | Per-commit diff traversal at scale is expensive — needs caching strategy. D3 charting is straightforward once data is available. |
| 24 | B5 | **Diff Statistics Flame Bar** | 6 | 5 | 7 | 5 | **42** | `ComputeFileDiff` per file at list stage adds latency. Frontend change is minimal once counts are in the response. |
| 25 | B7 | **Commit Range Diff** | 5 | 7 | 6 | 6 | **35** | `TreeDiff` primitive exists. New route + frontend state machine changes in `diffView.js`. Strategic: enables PR-style review. |
| 26 | F7 | **Mobile Responsive Layout** | 4 | 6 | 6 | 5 | **29** | No CSS breakpoints exist. Sidebar drawer pattern on narrow viewports. D3 touch zoom needs verification. |
| 27 | D2 | **Backend Full-Text Search Index** | 5 | 6 | 5 | 8 | **19** | Only needed when A2 (frontend search) proves insufficient. Ship A2 first. |
| 28 | C3 | **Submodule Awareness** | 3 | 5 | 6 | 6 | **15** | `isSubmodule()` detection exists. `.gitmodules` parsing, endpoint, and explorer UX all new. |
| 29 | A8 | **Graph Layout Mode Toggle (DAG)** | 5 | 6 | 4 | 8 | **15** | ✅ DONE | Shipped as lane-based layout in `web/graph/layout/laneStrategy.js`. Topological column-reuse algorithm with smooth transitions. |
| 30 | C4 | **Line-Level Blame** | 4 | 7 | 4 | 8 | **14** | `GetLineBlame()` does not exist. Requires new algorithm. Pure-Go implementation planned. **Strategic.** |
| 31 | A5b | **Export Graph as SVG** | 4 | 5 | 4 | 8 | **10** | No SVG rendering path exists. Would require a parallel renderer. Low priority relative to PNG. |
| 32 | A9 | **Interactive Rebase Preview** | 3 | 5 | 3 | 9 | **5** | Requires DAG-rewrite simulation and depends on A8. Architecture is read-only by design. Long-horizon. |

---

## Top Priorities for Next Sprint

### ✅ Sprint 1 Completed (5 features)
- A1: Wire `getAuthorColor` into renderer (**partial** — merge diamonds and lane mode only; force-mode regular commits still use palette.node)
- F1: HTTP Timeouts and Graceful Shutdown
- F3: Theme-Pinning Toggle (light/dark/system)
- A4: Progressive Commit Detail at Zoom Levels
- B2: Rename Detection in Diffs

### ✅ Sprint 2 Completed (5 features)
- F4: LRU Cache Eviction (bounded cache with configurable size)
- F6: Structured Logging (slog in server; gitcore still uses log.Printf)
- A2: Commit Search and Filtering (debounced search with qualifier syntax, opacity dimming)
- A3: Configurable Graph Display Filters (hide remotes/merges/stashes, branch focus)
- A8/Lane-Based Layout Strategy (dual force/lane layout with toolbar switching)

### 🎨 Frontend Design System Completed
A comprehensive visual overhaul with Geist typography, vibrant teal accent color (#0ea5e9), refined shadows, improved spacing, and smooth micro-interactions across all UI components.

### Infrastructure Issues (should fix before next feature sprint)

1. **golangci-lint v2 migration**: `.golangci.yml` uses v1 schema but CI resolves to v2.x. `make lint` and CI lint job fail. Config needs full migration.
2. **Merge dev to main**: `main` is 41+ commits behind `dev`. All shipped features exist only on `dev`.
3. **`make test` missing `-race`**: Local tests provide weaker guarantees than CI.
4. **Stale files**: `WORKFLOW.md` is fully stale (all tasks complete). `INTEGRATION_COMPLETE.md` has stale line-number references. `docs/web-dev/` HTML test files have broken relative paths.
5. **Stale remote branches**: `origin/rybkr/feat/minimap` (3 months), `origin/feature/large-graph` (9 days) should be evaluated for pruning.

### Next Recommended Items

**1. Finish A1: Author Colors in Force Mode (RICE 560, trivial)**
In `renderNormalCommit()` at `graphRenderer.js:433`, replace `node.laneColor || this.palette.node` with `getAuthorColor(node.authorEmail)` (or fall back to palette). One-line change to complete the highest-RICE item.

**2. Inline Syntax Highlighting in Diff View (B1, RICE 168) — NOT STARTED**
Apply highlight.js to diff content in `diffContentViewer.js`. The library is already loaded in `fileContentViewer.js`; just extend the same pattern to diffs. Tokenize each `DiffLine.content` through `hljs.highlight()` per-line or wrap hunks in `<code>` elements. Detect language from file path extension (already done in `fileContentViewer.js`).

**3. Working Tree Diff Context Expansion (B4, RICE 135) — COMPLETED**
Pure Go implementation via `ComputeWorkingTreeFileDiff()` in `internal/gitcore/worktree_diff.go` accepts a `contextLines` parameter. The server handler reads `r.URL.Query().Get("context")`, parses it as an integer, and passes it to the gitcore function. No git CLI shell-out required.

**4. Export Graph as PNG (A5, RICE 135)**
`canvas.toDataURL("image/png")` — DPR scaling already correct. Add a download button to the graph toolbar.

**5. Clickable Blame Hash → Graph Navigation (C1, RICE 126)**
Pure frontend wiring. Expose `navigateToCommit(hash)` from `graphController.js`; call it from `fileExplorer.js` blame column click handler.

**6. Watcher Polling Fallback (F2, RICE 112)**
Docker bind-mount is a primary deployment target. `GITVISTA_POLL_INTERVAL` env var entirely absent. Debounce constant needs to become a config field.

---

## Architectural Notes for Implementers

### Adding a New Backend Endpoint
1. Add the handler method to `internal/server/handlers.go`
2. Register the route in `internal/server/server.go`'s `Start()`, wrapping with `s.rateLimiter.middleware(...)`
3. Validate inputs using helpers in `internal/server/validation.go` (path traversal prevention, hash validation)
4. Add table-driven tests in `internal/server/handlers_test.go`

### Adding a New Frontend Module
1. Create an ES module file in `web/`
2. Import it in `web/app.js` with a relative `./` path
3. No bundler needed — ES modules load natively
4. CDN dependencies are ESM `import` statements (D3.js) or lazy `<script>` injection (highlight.js), not npm packages

### Extending `gitcore` with New Git Object Parsing
1. Add the parsing function to the appropriate file in `internal/gitcore/`
2. For `.git/` directory reads, follow the `os.ReadFile` pattern with `//nolint:gosec` and a comment explaining why the path is controlled
3. Add table-driven tests using hand-constructed binary data (see `pack_test.go`, `objects_test.go`)
4. Run `make test` to verify no regressions

### WebSocket Delta Extensions
To push new data types to the frontend in real-time, extend `UpdateMessage` in `internal/server/types.go` and add a handler in `web/backend.js`'s message dispatch switch. The broadcast infrastructure in `broadcast.go` handles delivery to all connected clients.
