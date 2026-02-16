# File Explorer Redesign - Architecture Plan

## Problem Statement

The current file browser lives in a right sidebar, uses breadcrumb-based navigation (replacing
the view when entering a directory), and only shows the immediate children of one tree at a time.
The goal is to replace it with a full project tree explorer in the left sidebar that follows
standard IDE conventions: disclosure triangles, nested indentation, folder/file icons, and
per-file "last modified" commit information.

## Current State Summary

**Left sidebar** (`sidebar.js` + `indexView.js`): Collapsible panel showing "Working Tree" status
(staged, modified, untracked files). Has resize handle, collapse/expand, localStorage persistence.

**Right sidebar** (`fileSidebar.js` + `fileBrowser.js`): Opens when a commit's tree icon is clicked
in the graph. Uses breadcrumb navigation to walk tree objects. Fetches `/api/tree/{hash}` to list
entries and `/api/blob/{hash}` to view file contents. Starts collapsed, opens on demand.

**Backend**: `handleTree` returns a tree object's entries (mode, name, hash, type). `handleBlob`
returns blob content with binary detection and truncation. No endpoint exists for per-file
last-modified info. The `Repository` struct loads all commits and refs but does not build
path-to-commit mappings.

**Trigger**: Clicking the folder icon on a commit node in the graph calls
`options.onCommitTreeClick(commit)`, which passes `{hash, tree, message, ...}` to the file browser
and expands the right sidebar.


---


## Architecture Overview

```
                          Left Sidebar
    ┌───────────────────────────────────────────────┐
    │  [Tab: Working Tree]  [Tab: File Explorer]    │
    ├───────────────────────────────────────────────┤
    │                                               │
    │  When "File Explorer" tab active:             │
    │  ┌─────────────────────────────────────────┐  │
    │  │ Commit: abc1234 - "Fix login bug"       │  │
    │  ├─────────────────────────────────────────┤  │
    │  │ > cmd/                      3d ago  e2f │  │
    │  │ v internal/                 1d ago  a1b │  │
    │  │   v gitcore/                1d ago  a1b │  │
    │  │     objects.go              1d ago  a1b │  │
    │  │     pack.go                 5d ago  c3d │  │
    │  │     types.go                3d ago  e2f │  │
    │  │   > server/                 2d ago  b4c │  │
    │  │ v web/                      1d ago  a1b │  │
    │  │   app.js                    1d ago  a1b │  │
    │  │   styles.css                2d ago  b4c │  │
    │  │ go.mod                      7d ago  f0a │  │
    │  │ go.sum                      7d ago  f0a │  │
    │  └─────────────────────────────────────────┘  │
    │                                               │
    │  When file clicked -> content viewer panel    │
    │  appears below or replaces tree temporarily   │
    └───────────────────────────────────────────────┘
```

The right sidebar (`fileSidebar.js`) is removed entirely.


---


## Detailed Design

### 1. Left Sidebar Tab System

The left sidebar gains a tab bar with two tabs: **Working Tree** and **File Explorer**.

**Component**: `sidebarTabs.js` (new file)

```
createSidebarTabs() -> { el, showTab(name), onTabChange(callback) }
```

The tab bar renders as a horizontal strip below the sidebar header. Each tab is a button. The
active tab gets a bottom border highlight. Only one content panel is visible at a time.

Tabs:
- "Working Tree" - shows the existing `indexView` content
- "File Explorer" - shows the new `fileExplorer` content

When no commit has been selected for the File Explorer, the tab content shows a placeholder:
"Click a commit's tree icon to browse files."

**Changes to `sidebar.js`**: The `content` div is replaced by a tab container that manages two
content panels. The sidebar header retains its collapse toggle button.

**Changes to `app.js`**: Remove `fileSidebar` and `fileBrowser` imports. Instead, create the
`fileExplorer` component and mount it into the left sidebar's "File Explorer" tab. When
`onCommitTreeClick` fires, switch to the File Explorer tab and load that commit's tree.


### 2. File Explorer Component (Tree View)

**Component**: `fileExplorer.js` (new file, replaces `fileBrowser.js`)

Core differences from the current `fileBrowser.js`:

| Aspect             | Current (fileBrowser)               | New (fileExplorer)                    |
|--------------------|--------------------------------------|---------------------------------------|
| Navigation         | Breadcrumb, replaces list            | In-place expand/collapse              |
| Directory depth    | One level at a time                  | Full nested tree visible              |
| Last-modified info | None                                 | Commit hash + relative time per file  |
| Content viewer     | Inline in same panel                 | Separate panel (bottom or overlay)    |

**State model**:

```javascript
{
    commitHash: string,           // Currently browsed commit
    commitMessage: string,        // For display in header
    rootTreeHash: string,         // Root tree hash of the commit
    expandedDirs: Set<string>,    // Set of expanded directory paths (e.g. "internal/gitcore")
    treeCache: Map<string, entries[]>,  // tree hash -> entries, avoids re-fetching
    lastModifiedCache: Map<string, info>,  // path -> {commitHash, commitMessage, when}
    selectedFile: { path, blobHash } | null,  // Currently selected file
}
```

**Rendering**:

The tree renders as a flat list of visible entries. A directory entry has a disclosure triangle
(chevron) that rotates when expanded. Indentation is computed from depth level (16px per level).
Entries are sorted: directories first (alphabetically), then files (alphabetically).

Each visible entry is a row:

```
[indent] [chevron?] [icon] [name]                    [age] [hash]
```

- `indent`: `padding-left: depth * 16px`
- `chevron`: Only for directories. Right-pointing when collapsed, down-pointing when expanded.
  Clicking the chevron or the directory name toggles expand/collapse.
- `icon`: Folder icon for directories, file icon for files (reuse existing SVGs).
- `name`: File or directory name.
- `age`: Relative time of last modification ("3d ago", "2h ago"). Fetched lazily.
- `hash`: Abbreviated commit hash (7 chars) of last modification.

**Expand/collapse behavior**:

When a directory is expanded:
1. Add its path to `expandedDirs`.
2. Fetch `/api/tree/{treeHash}` if not cached.
3. Insert child entries into the rendered list below the directory entry.
4. Fetch `/api/tree/blame/{commitHash}?path={dirPath}` for last-modified info of new entries.

When collapsed:
1. Remove path from `expandedDirs`.
2. Remove all descendant entries from the rendered list.
3. No network requests needed.

**Rendering strategy**:

Rather than DOM-per-entry with nested containers, use a flat array approach:

1. Walk `expandedDirs` recursively from root, building a flat `visibleEntries[]` array.
2. Each entry has: `{ path, name, depth, isDir, treeHash, blobHash, mode }`.
3. Render this flat list as a series of `<div>` rows. This avoids nested DOM complexity and
   makes virtual scrolling straightforward if needed later.

**File click behavior**:

Clicking a file entry:
1. Sets `selectedFile` and highlights the row.
2. Opens a file content viewer panel. Two options considered:

   - **Option A**: Split the sidebar vertically - tree on top, content on bottom.
   - **Option B**: Replace the tree with the content view plus a "back to tree" button (like current).
   - **Option C**: Open a floating panel or overlay that can be closed.

   **Recommendation**: Option A (vertical split) because users want to see the tree and file
   content simultaneously, and it mirrors VS Code behavior. The split uses a draggable divider
   at 50/50 by default. If the sidebar is too narrow, fall back to Option B.

   For the initial implementation, use **Option B** (replace tree with content + back button) since
   it is simpler and matches the existing pattern. Option A can be added later as an enhancement.

The content viewer reuses the existing rendering logic from `fileBrowser.js` (line numbers, binary
detection, truncation notice) but is extracted into a standalone `fileContentViewer.js` module.


### 3. Backend: Per-File Last-Modified API

#### New Endpoint

```
GET /api/tree/blame/{commitHash}?path={dirPath}
```

**Request**: `commitHash` is the commit whose tree to examine. `path` is the directory path
relative to the repository root (empty string or `/` for root).

**Response**:

```json
{
    "entries": {
        "README.md": {
            "commitHash": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
            "commitMessage": "Update documentation",
            "authorName": "Ryan Baker",
            "when": "2026-02-10T14:30:00Z"
        },
        "go.mod": {
            "commitHash": "f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9",
            "commitMessage": "Initial commit",
            "authorName": "Ryan Baker",
            "when": "2026-02-03T09:15:00Z"
        }
    }
}
```

The response only includes entries for the immediate children of `path`, not recursive
descendants. The frontend fetches blame info lazily as directories are expanded.

#### Algorithm: Computing Last-Modified Per File

This is the most computationally interesting part of the design. There are several approaches.

**Approach 1: Walk commit history, compare trees (recommended)**

For a given commit and directory path, walk backward through commit history (following first
parents for simplicity, or all parents for accuracy). At each commit, compare the tree entry
hashes between the current commit and its parent. If a file's blob hash differs, that commit
is the last modifier.

```
func (r *Repository) GetFileBlame(commitHash Hash, dirPath string) (map[string]BlameEntry, error)

Algorithm:
1. Resolve commitHash -> commit -> tree
2. Navigate tree to dirPath (split path, walk subtrees)
3. Get current entries: map[name] -> blobHash
4. Walk backward through commit history (BFS/DFS on parents)
5. For each ancestor commit:
   a. Navigate to same dirPath in ancestor's tree
   b. Compare each entry's hash with current known state
   c. When an entry's hash differs (or entry doesn't exist in ancestor),
      the CHILD commit (one step closer to our target) is the last modifier
6. Entries not found in any ancestor were introduced in the earliest commit examined
```

**Complexity**: O(C * D) where C = number of commits walked, D = number of directory entries.
For typical repos this is fast. For very large repos, we add a depth limit (e.g., walk at most
1000 commits) and mark remaining entries as "unknown".

**Optimization**: Cache results keyed by `(commitHash, dirPath)`. Since commit hashes are
immutable, cached results never go stale.

**Approach 2: Use `git log` subprocess (rejected)**

We could shell out to `git log --format=... -- <path>` for each file. This is what `status.go`
does for working tree status. However:
- It requires one subprocess per file (N subprocesses per directory listing).
- It breaks the `gitcore` package's principle of no external Git dependency.
- It would be slow for directories with many files.

**Approach 3: Build a full path-to-commit index at startup (rejected)**

Walk all commits at repository load time and build a complete mapping of every file path to its
last-modifying commit. This gives O(1) lookups but:
- Startup time becomes O(commits * files), which is very expensive for large repos.
- Memory usage grows proportionally.
- Most paths are never viewed, so the work is wasted.

**Decision**: Approach 1 with caching. Compute on-demand when a directory is expanded, cache
the result, and return it quickly on subsequent requests.


#### New Backend Types

In `internal/gitcore/types.go` or a new `internal/gitcore/blame.go`:

```go
// BlameEntry records which commit last modified a file.
type BlameEntry struct {
    CommitHash    Hash      `json:"commitHash"`
    CommitMessage string    `json:"commitMessage"`
    AuthorName    string    `json:"authorName"`
    When          time.Time `json:"when"`
}
```

#### New Backend Methods

In `internal/gitcore/blame.go` (new file):

```go
// GetFileBlame returns per-file last-modified info for immediate children of dirPath
// at the given commit.
func (r *Repository) GetFileBlame(commitHash Hash, dirPath string) (map[string]*BlameEntry, error)

// resolveTreeAtPath navigates from a root tree hash down to the tree at dirPath.
// Returns nil if the path doesn't exist in the tree.
func (r *Repository) resolveTreeAtPath(rootTree Hash, dirPath string) (*Tree, error)
```

#### Cache

The server maintains a `sync.Map` or similar cache:

```go
// blameCache maps "commitHash:dirPath" -> map[string]*BlameEntry
blameCache sync.Map
```

This cache lives on the `Server` struct in `server.go`. It is cleared when the repository
is reloaded (since new commits may have been added, but old commit results remain valid --
actually, old commit blame results are permanently valid since they reference immutable objects,
so the cache should NOT be cleared on reload; it is append-only).


### 4. Data Flow

#### Opening a commit's file tree

```
User clicks tree icon on commit node in graph
    |
    v
graphController calls onCommitTreeClick(commit)
    |
    v
app.js switches sidebar to "File Explorer" tab
app.js calls fileExplorer.openCommit(commit)
    |
    v
fileExplorer fetches GET /api/tree/{rootTreeHash}
fileExplorer renders root entries (collapsed)
    |
    v
fileExplorer fetches GET /api/tree/blame/{commitHash}?path=
    |
    v
Server walks commit history, computes blame for root entries
Server caches result, returns JSON
    |
    v
fileExplorer annotates each root entry with last-modified info
```

#### Expanding a directory

```
User clicks directory entry or its chevron
    |
    v
fileExplorer adds path to expandedDirs
fileExplorer fetches GET /api/tree/{dirTreeHash}  (if not cached)
    |
    v
fileExplorer inserts child entries into visible list
fileExplorer re-renders
    |
    v
fileExplorer fetches GET /api/tree/blame/{commitHash}?path={dirPath}  (async)
    |
    v
When blame data arrives, annotate child entries and re-render annotations only
```

#### Clicking a file

```
User clicks file entry
    |
    v
fileExplorer sets selectedFile
fileExplorer fetches GET /api/blob/{blobHash}  (existing endpoint)
    |
    v
fileContentViewer renders file content (line numbers, etc.)
```


### 5. File-by-File Change List

#### Files to DELETE
- `/Users/ryanbaker/projects/gitvista/web/fileSidebar.js` -- Right sidebar container, no longer needed.

#### Files to CREATE
- `/Users/ryanbaker/projects/gitvista/web/sidebarTabs.js` -- Tab bar component for the left sidebar.
- `/Users/ryanbaker/projects/gitvista/web/fileExplorer.js` -- Full tree explorer with expand/collapse.
- `/Users/ryanbaker/projects/gitvista/web/fileContentViewer.js` -- Extracted file content rendering (from fileBrowser.js).
- `/Users/ryanbaker/projects/gitvista/internal/gitcore/blame.go` -- Per-file last-modified computation.
- `/Users/ryanbaker/projects/gitvista/internal/gitcore/blame_test.go` -- Tests for blame logic.

#### Files to MODIFY

**`/Users/ryanbaker/projects/gitvista/web/app.js`**
- Remove imports: `fileSidebar`, `fileBrowser`.
- Add imports: `sidebarTabs`, `fileExplorer`, `fileContentViewer`.
- Create tab system in sidebar content area.
- Mount `indexView` under "Working Tree" tab.
- Mount `fileExplorer` under "File Explorer" tab.
- Wire `onCommitTreeClick` to switch to File Explorer tab and call `fileExplorer.openCommit()`.
- Remove right sidebar DOM attachment.

**`/Users/ryanbaker/projects/gitvista/web/sidebar.js`**
- No structural changes needed. The `content` div is already generic. Tabs are mounted into it by
  `app.js`. However, consider widening the default width from 280px to 320px to accommodate the
  file explorer's additional columns (age, hash).

**`/Users/ryanbaker/projects/gitvista/web/fileBrowser.js`**
- Keep temporarily for reference during migration, then delete once `fileExplorer.js` and
  `fileContentViewer.js` are complete.

**`/Users/ryanbaker/projects/gitvista/web/styles.css`**
- Remove all `.file-sidebar*` styles (right sidebar).
- Remove `.file-breadcrumbs*` styles (breadcrumb navigation).
- Add `.sidebar-tabs` styles (tab bar).
- Add `.file-explorer*` styles (tree rows, indentation, chevrons, annotations).
- Modify `.file-tree-entry` styles for the new flat list layout with blame columns.
- Keep `.file-content*` styles (used by `fileContentViewer.js`).

**`/Users/ryanbaker/projects/gitvista/web/index.html`**
- No changes needed. The DOM is built dynamically.

**`/Users/ryanbaker/projects/gitvista/internal/server/handlers.go`**
- Add `handleTreeBlame` handler for `GET /api/tree/blame/{commitHash}`.
- Parse `commitHash` from path, `path` from query parameter.
- Call `repo.GetFileBlame(commitHash, path)`.
- Check server-side cache before computing.

**`/Users/ryanbaker/projects/gitvista/internal/server/server.go`**
- Add blame cache field: `blameCache sync.Map`.
- Register new route: `http.HandleFunc("/api/tree/blame/", s.handleTreeBlame)`.

**`/Users/ryanbaker/projects/gitvista/internal/gitcore/repository.go`**
- Add `resolveTreeAtPath` method (navigates tree hierarchy by path components).
- This is a general-purpose utility that `GetFileBlame` in `blame.go` will use, but it
  logically belongs on `Repository` since it uses `GetTree`.

**`/Users/ryanbaker/projects/gitvista/internal/gitcore/types.go`**
- Add `BlameEntry` struct (or put it in `blame.go` to keep types co-located with their logic).


### 6. CSS Design for Tree Explorer

Key visual elements:

```css
/* Tab bar */
.sidebar-tabs {
    display: flex;
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
}
.sidebar-tab {
    flex: 1;
    padding: 8px 12px;
    font-size: 12px;
    font-weight: 500;
    border: none;
    background: transparent;
    color: var(--text-color);
    cursor: pointer;
    border-bottom: 2px solid transparent;
    opacity: 0.6;
}
.sidebar-tab.is-active {
    opacity: 1;
    border-bottom-color: var(--node-color);
}

/* Tree explorer entry row */
.explorer-entry {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    padding-left: calc(8px + var(--depth, 0) * 16px);
    font-size: 12px;
    cursor: pointer;
    border-radius: 4px;
}
.explorer-entry:hover {
    background: rgba(148, 163, 184, 0.10);
}
.explorer-entry.is-selected {
    background: rgba(148, 163, 184, 0.18);
}

/* Chevron for directories */
.explorer-chevron {
    display: inline-flex;
    width: 16px;
    height: 16px;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    transition: transform 0.12s ease;
    transform: rotate(0deg);
}
.explorer-chevron.is-expanded {
    transform: rotate(90deg);
}
/* Files get invisible spacer instead of chevron */
.explorer-chevron-spacer {
    width: 16px;
    flex-shrink: 0;
}

/* Name and annotation columns */
.explorer-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.explorer-blame {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
    font-size: 11px;
    color: rgba(99, 110, 123, 0.7);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.explorer-blame-age {
    width: 48px;
    text-align: right;
}
.explorer-blame-hash {
    width: 52px;
}
```


### 7. Risk Assessment

**Risk: Blame computation is slow for repos with deep history.**
Mitigation: Impose a walk depth limit (default 1000 commits). If a file's last-modifying commit
is not found within that limit, display "unknown" or omit the annotation. The cache prevents
repeated slow computations.

**Risk: Large directories produce many entries, causing DOM performance issues.**
Mitigation: The flat list approach is efficient for hundreds of entries. If directories have
thousands of files (e.g., `node_modules`), consider virtual scrolling as a follow-up enhancement.
The initial implementation renders all visible entries directly, which handles typical project
directories well.

**Risk: Multiple rapid expand/collapse actions cause race conditions with async fetches.**
Mitigation: Each fetch carries the commit hash and path. When the response arrives, verify that
the file explorer is still showing the same commit and the directory is still expanded before
applying the data. Use a generation counter that increments on `openCommit` to discard stale
responses.

**Risk: Sidebar width is too narrow for blame annotations.**
Mitigation: Increase default sidebar width to 320px. The blame columns (age + hash) add roughly
100px. The file name column uses `flex: 1` with `text-overflow: ellipsis`, so it adapts to
available space. On very narrow widths, hide the blame columns via CSS `@container` or a
width threshold check.

**Assumption: Commit history is linear enough for efficient blame.**
If the repo uses heavy merge workflows, the parent-walking algorithm must handle merge commits
(multiple parents). The algorithm should follow ALL parents, not just first-parent, to correctly
identify when a file was last changed. This increases complexity proportional to the number of
merge commits but remains bounded by the depth limit.


---


## Implementation Plan

### Work Items (in dependency order)

Items marked with `[parallel]` can be developed simultaneously.

#### Phase 1: Backend (no frontend changes yet)

**WI-1: Add `resolveTreeAtPath` to Repository** `[parallel]`
- File: `/Users/ryanbaker/projects/gitvista/internal/gitcore/repository.go`
- Add method that takes a root tree hash and path string, walks subtrees, returns `*Tree`.
- Unit test with a real git repo fixture.
- Estimated effort: Small.

**WI-2: Implement `GetFileBlame`** (depends on WI-1)
- Files: `/Users/ryanbaker/projects/gitvista/internal/gitcore/blame.go`, `blame_test.go`
- Define `BlameEntry` struct.
- Implement commit history walk with parent comparison.
- Add depth limit parameter (default 1000).
- Unit test against test repository.
- Estimated effort: Medium.

**WI-3: Add `/api/tree/blame/` endpoint** (depends on WI-2)
- Files: `/Users/ryanbaker/projects/gitvista/internal/server/handlers.go`, `server.go`
- Add handler, parse commit hash from URL path, parse `path` query param.
- Add `blameCache sync.Map` to Server.
- Wire route in `Start()`.
- Estimated effort: Small.

#### Phase 2: Frontend infrastructure `[parallel with Phase 1 after API contract is agreed]`

**WI-4: Create sidebar tab system** `[parallel]`
- File: `/Users/ryanbaker/projects/gitvista/web/sidebarTabs.js`
- Create `createSidebarTabs(tabs)` that returns `{ el, showTab, getActiveTab }`.
- Add CSS styles for `.sidebar-tabs`, `.sidebar-tab`, `.sidebar-tab.is-active`.
- Estimated effort: Small.

**WI-5: Extract file content viewer** `[parallel]`
- File: `/Users/ryanbaker/projects/gitvista/web/fileContentViewer.js`
- Extract the blob-fetching and rendering logic from `fileBrowser.js` lines 167-283.
- `createFileContentViewer()` -> `{ el, open(blobHash, fileName), close(), onBack(callback) }`.
- Reuse existing `.file-content*` CSS styles.
- Estimated effort: Small.

#### Phase 3: File Explorer component (depends on WI-4, WI-5)

**WI-6: Build `fileExplorer.js`**
- File: `/Users/ryanbaker/projects/gitvista/web/fileExplorer.js`
- State management: `expandedDirs`, `treeCache`, `lastModifiedCache`, `selectedFile`.
- `openCommit(commit)` entry point.
- Tree rendering with expand/collapse.
- Lazy blame fetching.
- File click -> content viewer.
- Add all `.explorer-*` CSS styles.
- Estimated effort: Large (core of the feature).

#### Phase 4: Integration (depends on WI-3, WI-6)

**WI-7: Rewire `app.js` and remove right sidebar**
- File: `/Users/ryanbaker/projects/gitvista/web/app.js`
- Replace right sidebar with tab-based left sidebar.
- Wire `onCommitTreeClick` to file explorer.
- Estimated effort: Small.

**WI-8: CSS cleanup**
- File: `/Users/ryanbaker/projects/gitvista/web/styles.css`
- Remove `.file-sidebar*` styles.
- Remove `.file-breadcrumbs*` styles.
- Verify no orphaned styles remain.
- Estimated effort: Small.

**WI-9: Delete dead code**
- Delete `/Users/ryanbaker/projects/gitvista/web/fileSidebar.js`.
- Delete `/Users/ryanbaker/projects/gitvista/web/fileBrowser.js` (after migration confirmed).
- Estimated effort: Trivial.


### Parallelization Summary

```
        WI-1 ──────> WI-2 ──────> WI-3 ──┐
                                          ├──> WI-7 ──> WI-8 ──> WI-9
        WI-4 ──┐                          │
               ├──> WI-6 ────────────────-┘
        WI-5 ──┘
```

WI-1 and WI-4/WI-5 can proceed in parallel.
WI-2 and WI-6 can overlap if the API contract (response shape) is agreed early.
WI-7 through WI-9 are sequential cleanup once everything is integrated.


---


## API Contract Reference

### Existing Endpoints (unchanged)

```
GET /api/repository        -> { name, gitDir }
GET /api/tree/{treeHash}   -> { hash, entries: [{ hash, name, mode, type }] }
GET /api/blob/{blobHash}   -> { hash, size, binary, truncated, content }
WS  /api/ws                -> UpdateMessage { delta, status }
```

### New Endpoint

```
GET /api/tree/blame/{commitHash}?path={dirPath}

Response: {
    "entries": {
        "<filename>": {
            "commitHash": "<40-char hex>",
            "commitMessage": "<first line of message>",
            "authorName": "<string>",
            "when": "<RFC3339 timestamp>"
        },
        ...
    }
}

Error responses:
- 400: Invalid commit hash format
- 404: Commit not found, or path not found in tree
- 500: Internal error during blame computation
```
