# Diff Content Viewer Component

A standalone component for rendering line-level unified diffs in GitVista. Displays hunks with dual line number gutters, color-coded additions/deletions/context, and handles special cases like binary files and truncated diffs.

## API

```javascript
import { createDiffContentViewer } from './diffContentViewer.js';

const viewer = createDiffContentViewer();

// Returns:
// {
//   el,                    // DOM element to mount
//   show(fileDiff),        // Display a FileDiff object
//   clear(),               // Clear and hide the viewer
//   onBack(callback),      // Register back button callback
//   onFileSelect(callback) // Optional: file navigation callback
// }
```

## Usage

```javascript
import { createDiffContentViewer } from './diffContentViewer.js';

// Create the viewer
const viewer = createDiffContentViewer();

// Mount to DOM
document.getElementById('container').appendChild(viewer.el);

// Handle back button
viewer.onBack(() => {
    console.log('User clicked back');
    viewer.clear();
});

// Show a diff
viewer.show({
    path: "web/app.js",
    status: "modified",
    binary: false,
    hunks: [
        {
            oldStart: 10,
            oldCount: 7,
            newStart: 10,
            newCount: 9,
            lines: [
                { type: "context", content: "import { foo } ...", oldNum: 10, newNum: 10 },
                { type: "delete", content: "import { old } ...", oldNum: 11 },
                { type: "add", content: "import { new } ...", newNum: 11 }
            ]
        }
    ],
    truncated: false
});
```

## FileDiff Object Format

```typescript
{
    path: string,           // File path (e.g., "web/app.js")
    status: string,         // "added" | "modified" | "deleted" | "renamed" | "copied"
    binary: boolean,        // True for binary files
    hunks: Array<{
        oldStart: number,   // Starting line in old file
        oldCount: number,   // Line count in old file
        newStart: number,   // Starting line in new file
        newCount: number,   // Line count in new file
        lines: Array<{
            type: string,   // "add" | "delete" | "context"
            content: string,// Line content (without leading +/- markers)
            oldNum?: number,// Line number in old file (if applicable)
            newNum?: number // Line number in new file (if applicable)
        }>
    }>,
    truncated: boolean      // True if diff was truncated due to size
}
```

## Line Types

Each line in a hunk has a `type` field:

- **`"context"`**: Unchanged line (shown in both old and new)
  - Has both `oldNum` and `newNum`
  - Neutral background color

- **`"add"`**: Added line (only in new file)
  - Has `newNum` only
  - Green background

- **`"delete"`**: Deleted line (only in old file)
  - Has `oldNum` only
  - Red background

## Special Cases

### Binary Files
```javascript
viewer.show({
    path: "assets/logo.png",
    status: "modified",
    binary: true,
    hunks: [],
    truncated: false
});
// Shows: "Binary file changed"
```

### Added Files
```javascript
viewer.show({
    path: "web/newFile.js",
    status: "added",
    binary: false,
    hunks: [/* hunks with only "add" lines */],
    truncated: false
});
// Shows "New file" notice + hunks
```

### Deleted Files
```javascript
viewer.show({
    path: "web/oldFile.js",
    status: "deleted",
    binary: false,
    hunks: [/* hunks with only "delete" lines */],
    truncated: false
});
// Shows "Deleted file" notice + hunks
```

### Truncated Diffs
```javascript
viewer.show({
    path: "data/large.json",
    status: "modified",
    binary: false,
    hunks: [/* partial hunks */],
    truncated: true
});
// Shows warning: "Diff truncated (too large to display completely)"
```

## Styling

The component uses CSS classes from `web/styles.css`:

### Main Structure
- `.diff-content-viewer` - Root container
- `.diff-content-back` - Back button
- `.diff-file-header` - File status and path header
- `.diff-content-body` - Scrollable content area

### Status Badges
- `.diff-status-badge` - Base badge style
- `.diff-status-badge--added` - Green for added files
- `.diff-status-badge--modified` - Yellow for modified files
- `.diff-status-badge--deleted` - Red for deleted files
- `.diff-status-badge--renamed` - Blue for renamed files
- `.diff-status-badge--copied` - Blue for copied files

### Hunks
- `.diff-hunks` - Container for all hunks
- `.diff-hunk` - Single hunk container
- `.diff-hunk-header` - Hunk header line (e.g., "@@ -10,7 +10,9 @@")

### Lines
- `.diff-line` - Base line style
- `.diff-line-add` - Added line (green background)
- `.diff-line-delete` - Deleted line (red background)
- `.diff-line-context` - Context line (neutral background)

### Line Numbers
- `.line-num-old` - Old file line number (left gutter)
- `.line-num-new` - New file line number (right gutter)
- `.line-content` - Line text content

### Notices
- `.diff-file-notice` - Generic file notice
- `.diff-file-notice--added` - "New file" notice
- `.diff-file-notice--deleted` - "Deleted file" notice
- `.diff-binary-notice` - Binary file message
- `.diff-truncated-notice` - Truncation warning

## Testing

Open `web/diffContentViewer.test.html` in a browser to test the component with various scenarios:

- Modified file with multiple hunks
- Added file (all new lines)
- Deleted file (all removed lines)
- Binary file
- Truncated diff
- Large diff (200+ lines)

## Integration Example

```javascript
// In your main app
import { createDiffContentViewer } from './diffContentViewer.js';

const diffViewer = createDiffContentViewer();
sidebar.appendChild(diffViewer.el);

// Fetch and display diff when commit is selected
async function showCommitDiff(commitHash) {
    const response = await fetch(`/api/commit/${commitHash}/diff`);
    const diff = await response.json();

    // Show first file by default
    if (diff.files && diff.files.length > 0) {
        diffViewer.show(diff.files[0]);
    }
}

// Handle back navigation
diffViewer.onBack(() => {
    diffViewer.clear();
    // Show file list or previous view
});
```

## Features

- **Dual Line Number Gutters**: Shows both old and new line numbers
- **Color-Coded Lines**: Green for additions, red for deletions, neutral for context
- **Hunk Headers**: Clear section markers with line ranges
- **Monospace Font**: Preserves code formatting and alignment
- **Whitespace Preservation**: Uses `white-space: pre` to maintain indentation
- **Responsive**: Works in constrained sidebar widths
- **Dark Mode Support**: Adapts to system color scheme
- **Accessibility**: Uses semantic HTML structure

## Performance Considerations

- Uses vanilla DOM manipulation (no virtual DOM overhead)
- Efficient line-by-line rendering
- Scrollable content area for large diffs
- No unnecessary re-renders (call `show()` only when needed)

## Browser Compatibility

- Modern browsers supporting ES modules
- CSS Grid and Flexbox required
- CSS custom properties (CSS variables) required
- Tested in Chrome, Firefox, Safari (latest versions)
