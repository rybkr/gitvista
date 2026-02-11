# Project Tasks - Phase: Tree Browsing, Diff Viewing & Performance

## Task: Blob Content & Diff REST API
Branch: feature/blob-content-api
Dependencies: []

Add backend endpoints for reading blob content and computing unified diffs between two blobs.

**gitcore changes (`internal/gitcore/objects.go`):**
- Add `ReadBlob(gitDir string, packIndices []PackIndex, hash Hash) ([]byte, error)` that reads a blob object and returns its raw content (reuse the existing `readObject` infrastructure but return the content bytes instead of parsing)
- Add a `BlobTooLargeError` for blobs exceeding a configurable size limit (default 1MB) to avoid sending huge binaries

**New file `internal/gitcore/diff.go`:**
- Add `DiffBlobs(contentA, contentB []byte) string` that computes a unified diff between two blob contents (use a pure-Go diff algorithm — `github.com/sergi/go-diff/diffmatchpatch` or implement Myers diff)
- Return unified diff format string with context lines

**New file `internal/server/blob_handlers.go`:**
- `GET /api/blob/{hash}` — returns `{"content": "<base64-or-utf8>", "size": N, "binary": bool}`. Detect binary content (null bytes in first 8KB). Return base64 for binary, UTF-8 for text.
- `GET /api/diff?a={hashA}&b={hashB}` — returns `{"diff": "<unified-diff-string>", "binary": bool}`. If either blob is binary, return `{"binary": true}` with no diff.
- Register routes in `server.go` `SetupRoutes`

**Acceptance criteria:**
- `curl localhost:8080/api/blob/{hash}` returns blob content for any valid blob hash in the repo
- `curl localhost:8080/api/diff?a={hash}&b={hash}` returns a unified diff between two text blobs
- Binary blobs are detected and flagged without attempting diff
- Blobs over the size limit return a 413 with an error message
- Unit tests for `DiffBlobs` and `ReadBlob` in gitcore

---

## Task: Repository Metadata API & Sidebar Info Panel
Branch: feature/repo-metadata
Dependencies: []

Add a backend endpoint that exposes rich repository metadata, and build a frontend sidebar section to display it.

**New file `internal/gitcore/metadata.go`:**
- Add `type RepositoryMetadata struct` with fields: `Name string`, `Description string`, `DefaultBranch string`, `Remotes []Remote` (name + URL), `BranchCount int`, `TagCount int`, `CommitCount int` (from the loaded commits, not exhaustive)
- Add `type Remote struct { Name, URL string }`
- Add `LoadMetadata(gitDir string, refs, commits)` that:
  - Reads `.git/description` (if exists and not the default template text)
  - Parses `.git/config` for `[remote "..."]` sections to extract remote names and URLs (simple line-based parsing — no full INI parser needed)
  - Resolves HEAD to determine default branch
  - Counts branches, tags, commits from already-loaded data

**Server changes (`internal/server/handlers.go`):**
- Enhance the existing `handleRepository` or add a new `handleMetadata` handler to return the full metadata struct at `GET /api/metadata`
- Register the new route in `server.go`

**Frontend sidebar enhancement (`web/sidebar.js`):**
- Add a "Repository Info" section at the top of the sidebar, above the working tree index
- Display: repo name (as heading), description (if present), HEAD / default branch, remote URLs (as a compact list), branch/tag/commit counts as small badges
- Fetch metadata from `/api/metadata` on startup in `backend.js` and pass to sidebar via a new callback
- Style with existing CSS patterns; add new rules in `web/styles.css` for the info section
- Section should be collapsible like the existing working tree sections

**Acceptance criteria:**
- Sidebar shows repo name, description, current branch, remotes, and counts on page load
- Metadata endpoint returns correct data for repos with and without `.git/description`
- Info section is collapsible and follows existing sidebar styling conventions
- Works when `.git/description` is missing or contains the default Git template text ("Unnamed repository; edit this file...")

---

## Task: Recursive Tree Diff API
Branch: feature/tree-diff-api
Dependencies: []

Add a backend API that recursively compares two commit trees and returns a structured changeset showing which files were added, deleted, modified, or unchanged.

**New file `internal/gitcore/treediff.go`:**
- Add types:
  ```go
  type DiffStatus string // "added", "deleted", "modified", "unchanged"
  type TreeDiffEntry struct {
      Path       string     `json:"path"`
      Status     DiffStatus `json:"status"`
      OldHash    Hash       `json:"oldHash,omitempty"`
      NewHash    Hash       `json:"newHash,omitempty"`
      Mode       string     `json:"mode"`
      IsTree     bool       `json:"isTree"`
  }
  ```
- Add `DiffTrees(gitDir string, packIndices []PackIndex, treeHashA, treeHashB Hash) ([]TreeDiffEntry, error)` that:
  - Loads both tree objects (reusing existing `ReadTree`)
  - Compares entries by name: entries only in A are "deleted", only in B are "added", in both with different hashes are "modified", same hashes are "unchanged"
  - Recurses into sub-trees for "modified" tree entries to produce full path diffs
  - Returns a flat list of `TreeDiffEntry` with full relative paths (e.g. `"src/main.go"`)

**New file `internal/server/diff_handlers.go`:**
- `GET /api/treediff?base={commitHash}&head={commitHash}` — resolves each commit to its tree hash, calls `DiffTrees`, returns JSON array of `TreeDiffEntry`
- Validate that both hashes exist and are commits
- Register route in `server.go`

**Acceptance criteria:**
- Comparing a commit with its parent shows the correct set of changed files
- Unchanged files are included with status "unchanged" (important for showing what stayed the same)
- Nested directory changes produce full relative paths (e.g. `internal/server/handlers.go`)
- Sub-tree additions/deletions are recursively expanded into individual file entries
- Returns 400 for invalid hashes, 404 for unknown commits
- Unit tests with synthetic tree objects covering: add, delete, modify, unchanged, nested directory changes

---

## Task: Blob Content Viewer Panel
Branch: feature/blob-viewer
Dependencies: [feature/blob-content-api]

Build a frontend panel for viewing blob (file) content with line numbers, triggered from blob nodes on the graph.

**New file `web/blobViewer.js`:**
- Export `createBlobViewer()` factory that returns `{ show(hash, filename), hide(), isVisible() }`
- Renders as a slide-in panel from the right side of the screen (overlays the graph, does not replace it)
- Panel contains: header bar (filename, abbreviated hash, close button), scrollable content area with line-numbered monospace text
- For text content: display with line numbers in a `<pre>` block
- For binary content: show a "Binary file (N bytes)" placeholder message
- Close on Escape key or close button click
- Panel width: ~50% of viewport, resizable via drag handle (mirror the sidebar resize pattern)

**Wiring (`web/app.js`):**
- Create blob viewer instance during bootstrap
- Pass blob viewer to tooltip manager or wire a callback so blob tooltip can trigger it

**Tooltip update (`web/tooltips/blobTooltip.js`):**
- Add a "View Content" button to the blob tooltip
- On click, trigger the blob viewer with the blob's hash and entry name

**Styles (`web/styles.css`):**
- Panel styling: fixed position, right-aligned, z-index above graph canvas but below tooltips
- Smooth slide-in CSS transition
- Line numbers column (muted color, right-aligned, non-selectable, visually separated from content)
- Monospace font for content area

**Acceptance criteria:**
- Clicking "View Content" on a blob tooltip opens the viewer panel with the file's content
- Line numbers are displayed alongside content
- Binary files show a placeholder message instead of garbled content
- Panel can be closed with Escape or the close button
- Panel is scrollable for long files
- Panel does not interfere with graph panning/zooming when open

---

## Task: Commit Comparison & File Change View
Branch: feature/commit-diff-view
Dependencies: [feature/tree-diff-api, feature/blob-content-api]

Build a frontend UI for comparing two commits, showing which files changed vs stayed the same, with inline diff viewing.

**New file `web/diffView.js`:**
- Export `createDiffView()` factory returning `{ compare(baseHash, headHash), hide(), isVisible() }`
- Renders as a panel (left side, or replaces sidebar content in a tab-like switch)
- Fetches from `/api/treediff?base={base}&head={head}`
- Displays a tree-structured file list grouped by directory, with status indicators:
  - Green `+` prefix for added files
  - Red `-` prefix for deleted files
  - Yellow `~` prefix for modified files
  - Gray text for unchanged files (collapsed by default under a "N unchanged files" toggle)
- Header shows: base commit (abbreviated hash + short message) → head commit, summary counts (e.g. "3 added, 1 deleted, 5 modified")
- Clicking a modified file fetches `/api/diff?a={oldHash}&b={newHash}` and shows a unified diff inline with red/green line highlighting
- Clicking an added file fetches `/api/blob/{hash}` and shows content highlighted in green
- Clicking a deleted file fetches `/api/blob/{hash}` and shows content highlighted in red

**Commit tooltip integration (`web/tooltips/commitTooltip.js`):**
- Add a "Compare with parent" button to the commit tooltip
- On click, calls `diffView.compare(parentHash, commitHash)` using the commit's first parent
- If commit has no parents (root commit), compare against an empty tree

**Wiring (`web/app.js`):**
- Create diff view instance during bootstrap
- Wire "compare" callback from commit tooltip to diff view

**Styles (`web/styles.css`):**
- File tree list with indentation for directory nesting
- Status colors: green (#22863a), red (#cb2431), yellow (#b08800), gray (#6a737d)
- Unified diff display: green background for added lines, red for deleted, neutral for context
- Diff line number columns (old + new)

**Acceptance criteria:**
- "Compare with parent" on a commit tooltip shows which files changed in that commit
- File list correctly categorizes files as added/deleted/modified/unchanged
- Clicking a modified file shows a unified diff with color highlighting
- Unchanged files are visible but collapsed by default
- Panel can be dismissed to return to normal graph view
- Works for root commits (no parent)

---

## Task: Large-Repo Graph Performance Optimization
Branch: feature/graph-viewport-perf
Dependencies: []

Optimize the D3 force simulation and canvas rendering to handle repositories with 1000+ commits without UI lag.

**Viewport-aware simulation (`web/graph/graphController.js`):**
- Implement a "simulation viewport" that only actively simulates nodes within or near the visible area (with a generous margin, e.g. 2x viewport in graph-space coordinates)
- Nodes outside the simulation viewport get their velocities zeroed and positions pinned (`node.fx = node.x; node.fy = node.y`)
- On pan/zoom changes, update which nodes are active vs pinned
- Cap maximum simultaneously simulated nodes at 500; if more are within viewport, prioritize recent commits and their direct neighbors (branches, immediate parent/child commits)

**Progressive chunk loading (`web/graph/layout/layoutManager.js`):**
- When receiving initial state chunks of 200 commits, add them to the simulation progressively with brief stabilization between batches
- Don't restart the simulation from high alpha on each chunk; gently reheat with `simulation.alpha(0.1).restart()` instead of the current full restart
- Batch the node/link array rebuild to avoid an O(n) rebuild per chunk arrival

**Level-of-detail rendering (`web/graph/rendering/graphRenderer.js`):**
- Increase viewport culling margin from the static 100px to a dynamic value based on zoom level (wider margin when zoomed out to prevent pop-in)
- When zoomed out beyond threshold (zoom < 0.4): render commits as plain filled circles (no gradients or glows), hide blob and tree entry name labels, reduce branch label font size
- Skip rendering tree/blob expand indicators when zoomed out beyond threshold

**New constants (`web/graph/constants.js`):**
- `SIMULATION_VIEWPORT_MARGIN = 800` (pixels in graph space)
- `MAX_SIMULATED_NODES = 500`
- `LOD_ZOOM_THRESHOLD = 0.4`
- `CHUNK_SETTLE_ALPHA = 0.1`

**Acceptance criteria:**
- Opening a repo with 1000+ commits does not freeze the browser during initial load
- Panning and zooming remain smooth (>30fps) with 2000+ nodes in graph state
- Zooming out on a large graph shows simplified node rendering without visual artifacts
- The simulation correctly activates/deactivates nodes as the user pans around
- No visible layout jumps or position resets when nodes transition between active and pinned states
