# DiffView Component

A UI component that displays the list of changed files in a Git commit and coordinates with a diff content viewer to show line-level changes.

## Files

- `web/diffView.js` - Main component implementation
- `web/diffView-demo.html` - Standalone demo with mock data
- `web/styles.css` - Styling (`.diff-view-*` classes added at end of file)

## API

```javascript
import { createDiffView } from './diffView.js';

const diffView = createDiffView(backend, diffContentViewer);

// Open diff for a commit
await diffView.open(commitHash, commitMessage);

// Close the view
diffView.close();

// Check if open
const isOpen = diffView.isOpen();

// Access DOM element
document.body.appendChild(diffView.el);
```

## Parameters

### `createDiffView(backend, diffContentViewer)`

- **`backend`** - Backend API wrapper (currently unused, reserved for future use)
- **`diffContentViewer`** - Diff content viewer component with the following interface:
  - `el` - DOM element
  - `show(fileDiff)` - Display a FileDiff object
  - `showLoading()` - (optional) Show loading state
  - `showError(message)` - (optional) Show error message
  - `close()` - (optional) Close/hide the viewer

## Expected API Endpoints

The component expects these backend endpoints to be available:

### `GET /api/commit/diff/{commitHash}`

Returns the list of changed files in a commit.

**Response format:**
```json
{
  "commitHash": "abc123...",
  "parentHash": "def456...",
  "entries": [
    {
      "path": "internal/gitcore/diff.go",
      "status": "added",
      "newHash": "...",
      "binary": false
    },
    {
      "path": "web/app.js",
      "status": "modified",
      "oldHash": "...",
      "newHash": "...",
      "binary": false
    },
    {
      "path": "old-file.txt",
      "status": "deleted",
      "oldHash": "...",
      "binary": false
    }
  ],
  "stats": {
    "added": 1,
    "modified": 1,
    "deleted": 1,
    "total": 3
  }
}
```

**Status values:**
- `"added"` - File was added
- `"modified"` - File was modified
- `"deleted"` - File was deleted
- `"renamed"` - File was renamed (future enhancement)

### `GET /api/commit/diff/{commitHash}/file?path={path}`

Returns the line-level diff for a specific file.

**Response format:**
```json
{
  "path": "web/app.js",
  "status": "modified",
  "binary": false,
  "oldHash": "...",
  "newHash": "...",
  "hunks": [
    {
      "oldStart": 10,
      "oldLines": 5,
      "newStart": 10,
      "newLines": 7,
      "lines": [
        { "type": "context", "content": "  const state = {" },
        { "type": "delete", "content": "    count: 0" },
        { "type": "add", "content": "    count: 0," },
        { "type": "add", "content": "    loading: false" },
        { "type": "context", "content": "  };" }
      ]
    }
  ]
}
```

## State Management

The component manages the following internal state:

```javascript
{
  commitHash: null,       // Currently displayed commit
  parentHash: null,       // Parent commit hash (null for root commits)
  entries: [],            // DiffEntry[] from API
  stats: null,            // { added, modified, deleted, total }
  loading: false,         // True while fetching
  selectedFile: null,     // Currently selected file path
  filterText: "",         // Filter input (optional, not implemented in v1)
  generation: 0,          // Stale response protection counter
  commitMessage: null,    // Commit message for header display
}
```

## Features

✅ **Lazy loading** - Only fetches file-level diffs when user clicks a file
✅ **Stale response protection** - Generation counter prevents race conditions
✅ **Error handling** - User-friendly error messages for failed fetches
✅ **Empty state** - Handles commits with no changes gracefully
✅ **Root commits** - Supports commits with no parent
✅ **Binary file detection** - Shows "binary" badge for binary files
✅ **Status badges** - Color-coded A/M/D/R badges
✅ **File selection** - Highlights selected file in list
✅ **Loading states** - Animated spinner during fetch

## UI Structure

```
┌──────────────────────────────────────────────┐
│ Commit: abc1234 - "Add diff feature"         │  ← Header
│ +3 added  ~1 modified  -1 deleted            │  ← Stats bar
├──────────────────────────────────────────────┤
│ Changed Files                                │  ← Section heading
├──────────────────────────────────────────────┤
│ A  internal/diff.go                          │  ← File list
│ M  web/app.js                    ◄───────────│     (selected)
│ D  old-file.txt                              │
├──────────────────────────────────────────────┤
│ [Diff Content Viewer]                        │  ← Content area
│ Shows line-level diff when file selected     │
└──────────────────────────────────────────────┘
```

## CSS Classes

All styles are prefixed with `.diff-view-*`, `.diff-stat-*`, `.diff-file-*`, or `.diff-status-*`.

Key classes:
- `.diff-view` - Main container
- `.diff-view-header` - Commit info header
- `.diff-stats-bar` - File change statistics
- `.diff-file-list` - Scrollable file list
- `.diff-file-item` - Individual file row (clickable)
- `.diff-file-item.is-selected` - Selected file (highlighted)
- `.diff-status` - Status badge (A/M/D/R)
- `.diff-status--added` - Green badge for added files
- `.diff-status--modified` - Yellow/orange badge for modified files
- `.diff-status--deleted` - Red badge for deleted files
- `.diff-view-loading` - Loading state container
- `.diff-view-empty` - Empty state (no changes)
- `.diff-view-error` - Error state

## Testing

1. **Demo page**: Open `web/diffView-demo.html` in a browser to see the component in isolation with mock data.
2. **Integration**: Wire it into `app.js` alongside the diff content viewer component.
3. **Backend**: Ensure the API endpoints are implemented and return the expected JSON format.

## Future Enhancements

- [ ] File filtering (search box to filter changed files by name)
- [ ] Keyboard navigation (arrow keys to navigate file list)
- [ ] File rename detection (show old → new path for renamed files)
- [ ] Expandable/collapsible sections by file type
- [ ] Diff stats per file (lines added/deleted)
- [ ] Jump to file in file explorer
- [ ] Copy file path to clipboard

## Integration Example

```javascript
import { createDiffView } from './diffView.js';
import { createDiffContentViewer } from './diffContentViewer.js';

// Create diff content viewer
const diffContentViewer = createDiffContentViewer();

// Create diff view with content viewer
const diffView = createDiffView(backend, diffContentViewer);

// Add to sidebar or main view
document.getElementById('sidebar-content').appendChild(diffView.el);

// Open when user clicks a commit's diff button
commitNode.addEventListener('click', () => {
  diffView.open(commit.hash, commit.message);
});
```
