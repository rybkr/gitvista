# GitVista Feature Development Workflow

> Initialized: 2026-02-18
> Methodology: Architect-selected tasks with parallel branch development, sequential code review and testing

---

## Architect-Selected Tasks (4 Features)

The software-architect agent reviewed TODO.md and selected these 4 tasks for development. Each has a dedicated git branch and clear architectural guidance.

| Priority | Task ID | Title | RICE | Branch | Backend | Frontend | Est. Effort |
|----------|---------|-------|------|--------|---------|----------|------------|
| 1 | **F6** | Structured Logging with slog | 135 | `feature/f6-structured-logging` | ✅ | ❌ | ~2 hrs |
| 2 | **F4** | LRU Cache Eviction | 112 | `feature/f4-lru-cache-eviction` | ✅ | ❌ | ~3 hrs |
| 3 | **A2** | Commit Search and Filtering | 98 | `feature/a2-commit-search` | ❌ | ✅ | ~4 hrs |
| 4 | **A3** | Graph Display Filters | 98 | `feature/a3-graph-filters` | ❌ | ✅ | ~3 hrs |

---

## Parallelization Strategy

### Phase 1 (Parallel)
- **F6 (Structured Logging)** and **F4 (LRU Cache)** can develop in parallel
  - Both are backend-only (Go)
  - No frontend dependencies
  - Can be merged independently
  - F4 benefits from F6's slog integration but doesn't strictly require it

### Phase 2 (Sequential)
- **A2 (Commit Search)** must complete before A3
  - A2 establishes the `filterPredicate` contract and `node.dimmed` renderer changes
  - A3 builds on top of A2's visibility model
  - Both are frontend-only (vanilla JS)

---

## Development Workflow

### For Feature Developers

Each branch is ready for implementation. Architectural guidance is in `/Users/ryanbaker/projects/gitvista/docs/TODO.md` (relevant section reproduced below) and in this file.

**Checkout your task:**
```bash
git checkout feature/f6-structured-logging  # (or other branch)
```

**During development:**
- Commit frequently with clear messages
- Run tests: `make test` (Go) or `node --check web/*.js` (JS)
- Run full CI: `make ci`

**When ready for review:**
- Push to origin: `git push origin feature/f6-structured-logging`
- Do NOT merge to dev — the code-reviewer and test-validator agents will review first

---

### For Code Reviewers

When a feature developer finishes a branch, the **code-reviewer agent** will:
1. Read the architectural guidance for the task
2. Examine all changed files
3. Check for security, performance, style, and correctness issues
4. Flag findings with severity levels (🔴 Critical, 🟠 Important, 🟡 Suggestion, 🟢 Nitpick)
5. Provide actionable feedback

The code-reviewer has deep expertise in Go, vanilla JS, D3.js, WebSockets, and Git internals, and understands GitVista's architecture patterns.

---

### For Test Validators

After code review and any requested changes, the **test-validator agent** will:
1. Verify existing tests still pass
2. Identify missing test coverage
3. Write new tests for the feature
4. Validate edge cases and error scenarios
5. Ensure tests are compatible with the CI pipeline

---

## Task-Specific Architectural Guidance

### F6 — Structured Logging with Configurable Log Levels

**Design Decision:** Hybrid approach—`slog.SetDefault()` for startup, `s.logger *slog.Logger` field on Server for testability.

**Key Architecture Points:**
- **Logger Propagation:** Server struct owns the logger, initialized from `slog.Default()` in `NewServer()`
- **Env Vars:** `GITVISTA_LOG_LEVEL` (default: "info"), `GITVISTA_LOG_FORMAT` (default: "text")
- **Migration Map:**
  - `internal/server/server.go` — 3 log.Printf calls
  - `internal/server/watcher.go` — 4 calls
  - `internal/server/broadcast.go` — 6 calls
  - `internal/server/update.go` — 3 calls
  - `internal/server/websocket.go` — 11 calls
  - `cmd/vista/main.go` — 4 calls (use slog.Default())
- **Data Flow:** Parse GITVISTA_LOG_LEVEL/FORMAT in main.go → create slog.Handler → set Default → Server reads it
- **Structured Fields:** Use `slog.Info("event", "key", value)` instead of format strings for JSON log mode

**Files to Modify:**
- `cmd/vista/main.go` (logger initialization)
- `internal/server/server.go` (add logger field)
- `internal/server/handlers.go` (replace log.Printf with s.logger)
- `internal/server/watcher.go` (same)
- `internal/server/broadcast.go` (same)
- `internal/server/update.go` (same)
- `internal/server/websocket.go` (same)
- `internal/server/types.go` (delete ANSI prefix constants, lines 8–13)

**Tests:** Verify `s.logger` can be injected with a null writer in tests.

---

### F4 — LRU Cache Eviction for Blame and Diff Results

**Design Decision:** Entry-count-based LRU using `sync.Mutex`, generic `LRUCache[V any]`, no memory-budget tracking.

**Key Architecture Points:**
- **Cache Implementation:** New file `internal/server/cache.go` with `LRUCache[V any]` struct
- **Eviction Policy:** Entry count (default 500 per cache, configurable via `GITVISTA_CACHE_SIZE`)
- **Thread Safety:** `sync.Mutex` (not RWMutex—every Get is also a write due to LRU reordering)
- **Invalidation:** No cache clearing on repository reload (content-addressed hashes ensure correctness)
- **Env Var:** `GITVISTA_CACHE_SIZE` (default: 500 entries total, shared between blame and diff)

**Data Flow:**
```
Server.NewServer()
  → blameCache: NewLRUCache[any](cacheSize)
  → diffCache:  NewLRUCache[any](cacheSize)

handlers.go
  → handleCommitDiff() calls c.Get(key), c.Put(key, result)
  → handleFileBlame() calls c.Get(key), c.Put(key, result)
```

**Files to Modify:**
- `internal/server/cache.go` (new file—LRUCache implementation)
- `internal/server/server.go` (replace `blameCache`, `diffCache` sync.Map with *LRUCache)
- `internal/server/handlers.go` (update Get/Put calls for type-safe cache API)

**Tests:** Table-driven tests for LRU eviction, concurrent Put/Get, cache hits/misses.

---

### A2 — Commit Search and Filtering

**Design Decision:** Opacity-based dimming (not node removal), shared `filterPredicate` model for A2+A3.

**Key Architecture Points:**
- **Visibility Model:** `node.dimmed` boolean set by `filterPredicate`
- **Search UI:** New component `web/search.js` (debounced input, outputs matching hashes)
- **Predicate Contract:** `filterPredicate: (node) => boolean` in graphState—composable with future filters
- **Keyboard Shortcut:** `/` key dispatch logic in `keyboardShortcuts.js` (context-aware for search vs. file explorer)
- **Rendering Impact:** `graphRenderer.js` respects `node.dimmed`, reduces alpha and hides labels for dimmed nodes
- **Data:** All search fields already exist (`node.commit.message`, `node.commit.author.name`, `node.commit.author.email`, `node.id`)

**Data Flow:**
```
search.js (input)
  → matchingHashes: Set<string>
  → graphController.updateGraph()
    → buildFilterPredicate(search)
    → for node in reconcileCommitNodes: node.dimmed = !predicate(node)
    → renderNormalCommit() checks node.dimmed
```

**Files to Modify:**
- `web/search.js` (new file—search bar component)
- `web/graph/graphState.js` (add `filterPredicate` field)
- `web/graph/graphController.js` (call `buildFilterPredicate`, set `node.dimmed` in updateGraph)
- `web/graph/rendering/graphRenderer.js` (check `node.dimmed`, reduce alpha, skip labels)
- `web/keyboardShortcuts.js` (context-aware `/` dispatch)
- `web/app.js` (import and init search.js)

**Tests:** Focus on predicate composition, debounce timing, search hit detection.

---

### A3 — Configurable Graph Display Filters

**Design Decision:** Filter panel above canvas (not in sidebar), branch reachability via client-side BFS, compose with A2's predicate.

**Key Architecture Points:**
- **Filter State:** `filterState = { hideRemotes, hideMerges, hideStashes, focusBranch }` in localStorage
- **Reachability Computation:** BFS from selected branch tip using `state.commits` parent pointers (O(n), <1ms for 1000 commits)
- **Composition with A2:** AND semantics—node must pass both search and filter predicates to be fully visible
- **UI Placement:** New `.graph-controls-filters` div above canvas (parallel to existing controls)
- **Predicate Extension:** `buildFilterPredicate(filterState, searchQuery)` in graphController—single predicate function for both A2 and A3

**Data Flow:**
```
graphFilters.js (panel UI)
  → filterState changes → localStorage + graphController.updateGraph()
    → buildFilterPredicate(filterState, searchQuery)
    → for node: node.dimmed = !predicate(node)
    → renderer respects node.dimmed
```

**Branch Reachability:**
```javascript
function getReachableCommits(branchTip, allCommits) {
  // BFS from branchTip through parent pointers
  const visited = new Set();
  const queue = [branchTip];
  while (queue.length > 0) {
    const hash = queue.shift();
    if (visited.has(hash)) continue;
    visited.add(hash);
    const commit = allCommits.get(hash);
    queue.push(...(commit.parents || []));
  }
  return visited;
}
```

**Files to Modify:**
- `web/graphFilters.js` (new file—filter panel component)
- `web/graph/graphController.js` (extend `buildFilterPredicate` to handle filters, add branch-reachability BFS)
- `web/graph/graphState.js` (add `filterState` field)
- `web/app.js` (import graphFilters.js, add panel to DOM)

**Tests:** Filter composition, branch reachability correctness, state persistence.

---

## Review & Merge Workflow

1. **Developer → Code-Reviewer:** Feature branch pushed, code-reviewer agent analyzes
2. **Code-Reviewer → Developer:** Feedback issued; developer makes fixes if needed (new commits)
3. **Developer → Test-Validator:** Code review approved; test-validator runs new tests
4. **Test-Validator → Developer:** Test coverage report; developer adds tests if needed
5. **All Green → Merge:** When both code review and tests pass, branch merges to `dev`
6. **dev → main:** After all 4 tasks merge to dev, a final integration is tested, then `dev` merges to `main`

---

## Branching Diagram

```
main (stable)
  │
  └─ dev (integration branch)
       │
       ├─ feature/f6-structured-logging
       ├─ feature/f4-lru-cache-eviction
       ├─ feature/a2-commit-search
       └─ feature/a3-graph-filters
```

Each feature branch branches off dev. After code review and test validation, it merges back to dev via:
```bash
git checkout dev
git pull origin dev
git merge --no-ff feature/f6-structured-logging -m "Merge feature/f6-structured-logging"
git push origin dev
```

---

## Status Tracking

- [ ] **F6** — Architecture complete, awaiting implementation
- [ ] **F4** — Architecture complete, awaiting implementation
- [ ] **A2** — Architecture complete, awaiting implementation (blocks A3)
- [ ] **A3** — Architecture complete, awaiting implementation (depends on A2)

---

## Quick Reference: Architectural Decisions

| Task | Key Decision | Rationale |
|------|--------------|-----------|
| **F6** | Hybrid slog init (SetDefault + Server.logger field) | Balance between startup simplicity and test injectability |
| **F4** | Entry-count LRU, not memory budget | Simpler to reason about; sufficient for GitVista's scale |
| **A2** | Opacity-based dimming, not node removal | Preserves D3 position simulation; composes with A3 |
| **A3** | Client-side branch reachability BFS | O(n) computation is <1ms; avoids backend changes |

---

**Next Steps:**
1. Feature developers checkout their branches
2. Implement according to architectural guidance
3. Push to origin when ready for review
4. Code-reviewer and test-validator agents analyze
5. Merge to dev after approval
