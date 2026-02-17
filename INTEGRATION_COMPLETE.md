# Diff View Integration - Complete

## Summary

The diff view feature has been **fully integrated** into the GitVista frontend. Users can now view commit diffs directly from the file explorer.

## Integration Points

### 1. Component Instantiation (`web/fileExplorer.js`)

- **Lines 100-107**: Both `diffContentViewer` and `diffView` are created within fileExplorer
- Components are wired together: `diffView` receives `diffContentViewer` as a parameter
- `diffContentViewer` has a back button callback that returns to the file list

### 2. View Mode State (`web/fileExplorer.js`)

- **Line 126**: `viewMode` state property added ("tree" | "diff")
- **Line 950**: Default to "tree" mode when opening a commit
- View mode controls which UI is rendered

### 3. Toggle Button UI (`web/fileExplorer.js`)

**Tree Mode (lines 519-528)**:
- Shows "Show Diff" button with diff icon
- Button appears in file explorer header
- Clicking switches to diff mode and re-renders

**Diff Mode (lines 464-475)**:
- Shows "Show Tree" button with tree icon
- Button appears in diff view header
- Clicking switches back to tree mode and closes diffView

### 4. Rendering Logic (`web/fileExplorer.js`)

- **Lines 502-506**: `render()` checks viewMode and calls `renderDiffMode()` for diff view
- **Lines 449-483**: `renderDiffMode()` creates header with toggle button and calls `diffView.open()`
- **Line 482**: `diffView.el` is appended to the file explorer container

### 5. Data Flow (`web/fileExplorer.js`)

1. User clicks commit → `openCommit()` called → viewMode set to "tree" (line 950)
2. User clicks "Show Diff" → viewMode set to "diff" (line 525) → `render()` called
3. `render()` detects diff mode → calls `renderDiffMode()` (lines 502-506)
4. `renderDiffMode()` calls `diffView.open(commitHash, commitMessage)` (line 479)
5. `diffView` fetches file list from `/api/commit/diff/{hash}`
6. User clicks file → `diffView` fetches file diff and passes to `diffContentViewer`
7. `diffContentViewer` renders the line-level diff with dual gutters

## User Flow

1. **Open commit**: Click commit's tree icon in the graph
2. **File explorer opens**: Shows file tree by default
3. **Toggle to diff view**: Click "Show Diff" button in header
4. **View changed files**: See list of added/modified/deleted files with stats
5. **View file diff**: Click any file to see line-level diff with +/- highlighting
6. **Back to file list**: Click back button in diff content viewer
7. **Toggle to tree view**: Click "Show Tree" button to return to file browser

## CSS Styling

All necessary styles are present in `web/styles.css`:

- `.diff-view`, `.diff-view-header`, `.diff-view-commit-info` (diff view container and header)
- `.diff-stats-bar`, `.diff-stat`, `.diff-stat--added/modified/deleted` (file change stats)
- `.diff-file-list`, `.diff-file-item`, `.diff-file-path` (file list)
- `.diff-status`, `.diff-status--added/modified/deleted` (status badges)
- `.diff-content-viewer`, `.diff-file-header` (line-level diff viewer)
- `.diff-hunk`, `.diff-hunk-header`, `.diff-line` (hunk rendering)
- `.diff-line-add`, `.diff-line-delete`, `.diff-line-context` (line styling)
- `.file-explorer-toggle` (toggle button styling)

## Backend API Endpoints

The frontend expects these endpoints (already implemented in backend):

- `GET /api/commit/diff/{commitHash}` - Returns list of changed files
- `GET /api/commit/diff/{commitHash}/file?path={path}` - Returns line-level diff for a file

Response formats match the component expectations.

## Testing Instructions

1. Start the server: `go run ./cmd/vista -repo /path/to/repo`
2. Open http://localhost:8080 in browser
3. Click any commit's tree icon in the graph
4. File explorer opens showing the file tree
5. Click "Show Diff" button in the header
6. Changed files list appears with status badges (A/M/D)
7. Click any file to see the line-level diff
8. Use back button to return to file list
9. Click "Show Tree" to return to file browser

## Files Modified

- `web/fileExplorer.js` - Added diff view integration, toggle button, and view mode state
- (No changes to `web/app.js` needed - fileExplorer is self-contained)
- (All CSS already present in `web/styles.css`)

## Notes

- The integration follows the existing architectural pattern where components are self-contained
- `fileExplorer` manages both tree and diff views internally
- No changes to `app.js` were needed because `fileExplorer` handles everything
- The toggle button only appears when viewing a commit (not working tree)
- Diff view automatically closes when opening a new commit (resets to tree mode)
