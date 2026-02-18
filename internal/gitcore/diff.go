package gitcore

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	maxDiffEntries      = 500        // Maximum number of file changes to process
	maxBlobSize         = 512 * 1024 // Maximum blob size for diff (512KB)
	defaultContextLines = 3          // Default number of context lines in unified diff

	// DefaultContextLines is the exported default for callers outside this package.
	DefaultContextLines = defaultContextLines
)

// TreeDiff recursively compares two trees and returns a flat list of changed files.
// oldTreeHash can be empty (zero hash) for root commits (no parent tree).
// prefix is used for recursion to build full paths (initially empty string).
// Returns an error if the number of entries exceeds maxDiffEntries.
func TreeDiff(repo *Repository, oldTreeHash, newTreeHash Hash, prefix string) ([]DiffEntry, error) {
	entries := make([]DiffEntry, 0)

	// Handle nil old tree (root commit case)
	var oldTree *Tree
	if oldTreeHash != "" {
		var err error
		oldTree, err = repo.GetTree(oldTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get old tree %s: %w", oldTreeHash, err)
		}
	}

	// Get new tree
	newTree, err := repo.GetTree(newTreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get new tree %s: %w", newTreeHash, err)
	}

	// Build maps of name -> TreeEntry for efficient lookup
	oldEntries := make(map[string]TreeEntry)
	if oldTree != nil {
		for _, entry := range oldTree.Entries {
			oldEntries[entry.Name] = entry
		}
	}

	newEntries := make(map[string]TreeEntry)
	for _, entry := range newTree.Entries {
		newEntries[entry.Name] = entry
	}

	// Track all unique names
	allNames := make(map[string]bool)
	for name := range oldEntries {
		allNames[name] = true
	}
	for name := range newEntries {
		allNames[name] = true
	}

	// Process each entry
	for name := range allNames {
		oldEntry, existsInOld := oldEntries[name]
		newEntry, existsInNew := newEntries[name]

		path := name
		if prefix != "" {
			path = prefix + "/" + name
		}

		// Check entry count limit
		if len(entries) >= maxDiffEntries {
			return nil, fmt.Errorf("diff too large: exceeded maximum of %d entries", maxDiffEntries)
		}

		switch {
		case !existsInOld && existsInNew:
			// Added entry
			if isTreeEntry(newEntry) {
				// Recursively add all files in the new tree
				subEntries, err := treeDiffRecursive(repo, "", newEntry.ID, path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, subEntries...)
			} else {
				// Added file
				isBinary := isSubmodule(newEntry)
				entries = append(entries, DiffEntry{
					Path:     path,
					Status:   DiffStatusAdded,
					NewHash:  newEntry.ID,
					IsBinary: isBinary,
					NewMode:  newEntry.Mode,
				})
			}

		case existsInOld && !existsInNew:
			// Deleted entry
			if isTreeEntry(oldEntry) {
				// Recursively delete all files in the old tree
				subEntries, err := treeDiffRecursive(repo, oldEntry.ID, "", path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, subEntries...)
			} else {
				// Deleted file
				isBinary := isSubmodule(oldEntry)
				entries = append(entries, DiffEntry{
					Path:     path,
					Status:   DiffStatusDeleted,
					OldHash:  oldEntry.ID,
					IsBinary: isBinary,
					OldMode:  oldEntry.Mode,
				})
			}

		case existsInOld && existsInNew:
			// Entry exists in both - check if modified
			if oldEntry.ID != newEntry.ID {
				// Different hash - either modified file or changed tree
				if isTreeEntry(oldEntry) && isTreeEntry(newEntry) {
					// Both are trees - recurse
					subEntries, err := treeDiffRecursive(repo, oldEntry.ID, newEntry.ID, path)
					if err != nil {
						return nil, err
					}
					entries = append(entries, subEntries...)
				} else if isTreeEntry(oldEntry) || isTreeEntry(newEntry) {
					// Type changed (file <-> directory)
					// Delete old, add new
					if isTreeEntry(oldEntry) {
						subEntries, err := treeDiffRecursive(repo, oldEntry.ID, "", path)
						if err != nil {
							return nil, err
						}
						entries = append(entries, subEntries...)
					} else {
						isBinary := isSubmodule(oldEntry)
						entries = append(entries, DiffEntry{
							Path:     path,
							Status:   DiffStatusDeleted,
							OldHash:  oldEntry.ID,
							IsBinary: isBinary,
							OldMode:  oldEntry.Mode,
						})
					}
					if isTreeEntry(newEntry) {
						subEntries, err := treeDiffRecursive(repo, "", newEntry.ID, path)
						if err != nil {
							return nil, err
						}
						entries = append(entries, subEntries...)
					} else {
						isBinary := isSubmodule(newEntry)
						entries = append(entries, DiffEntry{
							Path:     path,
							Status:   DiffStatusAdded,
							NewHash:  newEntry.ID,
							IsBinary: isBinary,
							NewMode:  newEntry.Mode,
						})
					}
				} else {
					// Both are files - modified
					isBinary := isSubmodule(oldEntry) || isSubmodule(newEntry)
					entries = append(entries, DiffEntry{
						Path:     path,
						Status:   DiffStatusModified,
						OldHash:  oldEntry.ID,
						NewHash:  newEntry.ID,
						IsBinary: isBinary,
						OldMode:  oldEntry.Mode,
						NewMode:  newEntry.Mode,
					})
				}
			}
			// If hashes are the same, no change - skip
		}
	}

	return entries, nil
}

// treeDiffRecursive is a helper for TreeDiff that handles recursive tree traversal.
func treeDiffRecursive(repo *Repository, oldTreeHash, newTreeHash Hash, prefix string) ([]DiffEntry, error) {
	return TreeDiff(repo, oldTreeHash, newTreeHash, prefix)
}

// isTreeEntry checks if a TreeEntry represents a directory.
func isTreeEntry(entry TreeEntry) bool {
	return entry.Type == "tree" || entry.Mode == "040000" || entry.Mode == "40000"
}

// isSubmodule checks if a TreeEntry represents a submodule (mode 160000).
func isSubmodule(entry TreeEntry) bool {
	return entry.Mode == "160000"
}

// ComputeFileDiff computes the line-level unified diff between two blobs.
// oldBlobHash can be empty for added files, newBlobHash can be empty for deleted files.
// contextLines controls how many unchanged lines to include around each change;
// pass defaultContextLines (3) for standard unified diff output.
// Returns a FileDiff struct with hunks containing line-level changes.
// Files larger than maxBlobSize are marked as truncated.
func ComputeFileDiff(repo *Repository, oldBlobHash, newBlobHash Hash, path string, contextLines int) (*FileDiff, error) {
	result := &FileDiff{
		Path:    path,
		OldHash: oldBlobHash,
		NewHash: newBlobHash,
		Hunks:   make([]DiffHunk, 0),
	}

	// Read old blob (empty if added file)
	var oldContent []byte
	if oldBlobHash != "" {
		var err error
		oldContent, err = repo.GetBlob(oldBlobHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read old blob %s: %w", oldBlobHash, err)
		}
	}

	// Read new blob (empty if deleted file)
	var newContent []byte
	if newBlobHash != "" {
		var err error
		newContent, err = repo.GetBlob(newBlobHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read new blob %s: %w", newBlobHash, err)
		}
	}

	// Check size limits
	if len(oldContent) > maxBlobSize || len(newContent) > maxBlobSize {
		result.Truncated = true
		return result, nil
	}

	// Check if binary
	if isBinaryContent(oldContent) || isBinaryContent(newContent) {
		result.IsBinary = true
		return result, nil
	}

	// Split into lines
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// Compute diff using Myers algorithm with caller-specified context depth
	hunks := myersDiff(oldLines, newLines, contextLines)
	result.Hunks = hunks

	return result, nil
}

// isBinaryContent detects if content appears to be binary.
// Uses the same heuristic as Git: checks first 8KB for null bytes.
func isBinaryContent(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	return bytes.IndexByte(data[:limit], 0) != -1
}

// splitLines splits content into lines, preserving empty lines.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return []string{}
	}

	lines := strings.Split(string(content), "\n")

	// Remove trailing empty line if content doesn't end with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// myersDiff implements the Myers diff algorithm to compute line-level diffs.
// Returns a list of hunks with context lines.
// See: "An O(ND) Difference Algorithm and Its Variations" by Eugene W. Myers.
func myersDiff(oldLines, newLines []string, context int) []DiffHunk {
	// Handle edge cases
	if len(oldLines) == 0 && len(newLines) == 0 {
		return []DiffHunk{}
	}

	// Compute the shortest edit script using Myers algorithm
	edits := computeEdits(oldLines, newLines)

	// If no changes, return empty hunks
	if len(edits) == 0 {
		return []DiffHunk{}
	}

	// Convert edits to hunks with context
	return buildHunks(oldLines, newLines, edits, context)
}

// editType represents the type of edit operation.
type editType int

const (
	editKeep editType = iota
	editDelete
	editInsert
)

// edit represents a single edit operation.
type edit struct {
	Type    editType
	OldLine int // 0-based index in old lines
	NewLine int // 0-based index in new lines
}

// computeEdits uses Myers diff algorithm to compute the shortest edit script.
func computeEdits(oldLines, newLines []string) []edit {
	n := len(oldLines)
	m := len(newLines)
	max := n + m

	// Handle trivial cases
	if n == 0 && m == 0 {
		return []edit{}
	}

	// V array stores the furthest reaching path for each k-line
	// k represents diagonal (k = x - y)
	// We offset by max to avoid negative indices
	v := make([]int, 2*max+1)

	// Trace stores the V array for each d value for backtracking
	trace := make([][]int, 0)

	// Forward search
	for d := 0; d <= max; d++ {
		// Save current V array before modifying
		vCopy := make([]int, len(v))
		copy(vCopy, v)
		trace = append(trace, vCopy)

		for k := -d; k <= d; k += 2 {
			var x int
			kIdx := k + max

			// Determine if we should move down or right
			if k == -d || (k != d && v[kIdx-1] < v[kIdx+1]) {
				// Move down (insert from new)
				x = v[kIdx+1]
			} else {
				// Move right (delete from old)
				x = v[kIdx-1] + 1
			}

			y := x - k

			// Follow diagonal (matching lines)
			for x < n && y < m && oldLines[x] == newLines[y] {
				x++
				y++
			}

			v[kIdx] = x

			// Check if we reached the end
			if x >= n && y >= m {
				// Backtrack to build edit script
				return backtrack(oldLines, newLines, trace, d, max)
			}
		}
	}

	// Should not reach here for valid input
	return []edit{}
}

// backtrack reconstructs the edit script from the trace.
func backtrack(oldLines, newLines []string, trace [][]int, d int, max int) []edit {
	edits := make([]edit, 0)
	x := len(oldLines)
	y := len(newLines)

	for depth := d; depth > 0; depth-- {
		vPrev := trace[depth-1]
		k := x - y
		kIdx := k + max

		var prevK int
		// Determine which direction we came from with bounds checking
		kPrevLeft := kIdx - 1
		kPrevRight := kIdx + 1
		canGoLeft := k != -depth && kPrevLeft >= 0 && kPrevLeft < len(vPrev)
		canGoRight := k != depth && kPrevRight >= 0 && kPrevRight < len(vPrev)

		if !canGoLeft || (canGoRight && vPrev[kPrevLeft] < vPrev[kPrevRight]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevKIdx := prevK + max
		prevX := vPrev[prevKIdx]
		prevY := prevX - prevK

		// From (x,y), we need to get back to (prevX, prevY)
		// First, follow any diagonal matches backwards
		// We can only follow the diagonal if the lines actually match
		for x > prevX && y > prevY && x > 0 && y > 0 && oldLines[x-1] == newLines[y-1] {
			x--
			y--
			edits = append([]edit{{Type: editKeep, OldLine: x, NewLine: y}}, edits...)
		}

		// Now we're at a position where we need to undo the edit operation
		// The edit happened when going from (prevX, prevY) to current position
		if prevY < 0 {
			// This shouldn't happen in valid scenarios
			prevY = 0
		}

		// Determine which edit operation got us here
		if x > prevX {
			// We have more x, so we must have moved right (delete)
			x--
			edits = append([]edit{{Type: editDelete, OldLine: x}}, edits...)
		} else if y > prevY {
			// We have more y, so we must have moved down (insert)
			y--
			edits = append([]edit{{Type: editInsert, NewLine: y}}, edits...)
		}
		// If x == prevX && y == prevY, we only followed diagonals (no edit)
	}

	// Add remaining edits from the beginning of the files
	// These must also be prepended since we've been building backwards
	for x > 0 && y > 0 {
		x--
		y--
		edits = append([]edit{{Type: editKeep, OldLine: x, NewLine: y}}, edits...)
	}

	// Handle any remaining deletions at the start
	for x > 0 {
		x--
		edits = append([]edit{{Type: editDelete, OldLine: x}}, edits...)
	}

	// Handle any remaining insertions at the start
	for y > 0 {
		y--
		edits = append([]edit{{Type: editInsert, NewLine: y}}, edits...)
	}

	return edits
}

// buildHunks converts edits into hunks with context lines.
func buildHunks(oldLines, newLines []string, edits []edit, context int) []DiffHunk {
	hunks := make([]DiffHunk, 0)

	if len(edits) == 0 {
		return hunks
	}

	// Find ranges of changes with context
	var currentHunk *DiffHunk
	lastChangeIdx := -1

	for i, edit := range edits {
		isChange := edit.Type != editKeep

		if isChange {
			// Start a new hunk or extend current one
			if currentHunk == nil {
				// Start new hunk - include context before
				currentHunk = &DiffHunk{
					Lines: make([]DiffLine, 0),
				}

				// Add context lines before first change
				contextStart := i - context
				if contextStart < 0 {
					contextStart = 0
				}
				for j := contextStart; j < i; j++ {
					if edits[j].Type == editKeep {
						currentHunk.Lines = append(currentHunk.Lines, DiffLine{
							Type:    "context",
							Content: oldLines[edits[j].OldLine],
							OldLine: edits[j].OldLine + 1,
							NewLine: edits[j].NewLine + 1,
						})
					}
				}

				// Set hunk start positions based on first line or first change
				if len(currentHunk.Lines) > 0 {
					currentHunk.OldStart = currentHunk.Lines[0].OldLine
					currentHunk.NewStart = currentHunk.Lines[0].NewLine
				} else {
					// No context - determine start based on edit type
					switch edit.Type {
					case editDelete:
						currentHunk.OldStart = edit.OldLine + 1
						// For NewStart, try to find the position in new file
						// If there's no new content, start at 0
						if len(newLines) > 0 {
							currentHunk.NewStart = 1
						} else {
							currentHunk.NewStart = 0
						}
					case editInsert:
						currentHunk.NewStart = edit.NewLine + 1
						// For OldStart, try to find the position in old file
						if len(oldLines) > 0 {
							currentHunk.OldStart = 1
						} else {
							currentHunk.OldStart = 0
						}
					}
				}
			}

			lastChangeIdx = i
		}

		if currentHunk != nil {
			// Add line to current hunk
			switch edit.Type {
			case editKeep:
				// Check if we should close the hunk (too far from last change)
				if lastChangeIdx >= 0 && i-lastChangeIdx > context*2 {
					// Add trailing context and close hunk
					for j := lastChangeIdx + 1; j <= lastChangeIdx+context && j < len(edits); j++ {
						if edits[j].Type == editKeep {
							currentHunk.Lines = append(currentHunk.Lines, DiffLine{
								Type:    "context",
								Content: oldLines[edits[j].OldLine],
								OldLine: edits[j].OldLine + 1,
								NewLine: edits[j].NewLine + 1,
							})
						}
					}

					// Finalize hunk
					finalizeHunk(currentHunk)
					hunks = append(hunks, *currentHunk)
					currentHunk = nil
					lastChangeIdx = -1
				} else {
					// Add context line
					currentHunk.Lines = append(currentHunk.Lines, DiffLine{
						Type:    "context",
						Content: oldLines[edit.OldLine],
						OldLine: edit.OldLine + 1,
						NewLine: edit.NewLine + 1,
					})
				}

			case editDelete:
				if edit.OldLine < len(oldLines) {
					currentHunk.Lines = append(currentHunk.Lines, DiffLine{
						Type:    "deletion",
						Content: oldLines[edit.OldLine],
						OldLine: edit.OldLine + 1,
						NewLine: 0,
					})
				}

			case editInsert:
				if edit.NewLine < len(newLines) {
					currentHunk.Lines = append(currentHunk.Lines, DiffLine{
						Type:    "addition",
						Content: newLines[edit.NewLine],
						OldLine: 0,
						NewLine: edit.NewLine + 1,
					})
				}
			}
		}
	}

	// Close final hunk with trailing context
	if currentHunk != nil {
		// Add trailing context
		contextEnd := lastChangeIdx + context + 1
		if contextEnd > len(edits) {
			contextEnd = len(edits)
		}
		for j := lastChangeIdx + 1; j < contextEnd; j++ {
			if edits[j].Type == editKeep {
				currentHunk.Lines = append(currentHunk.Lines, DiffLine{
					Type:    "context",
					Content: oldLines[edits[j].OldLine],
					OldLine: edits[j].OldLine + 1,
					NewLine: edits[j].NewLine + 1,
				})
			}
		}

		finalizeHunk(currentHunk)
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

// finalizeHunk computes the OldLines and NewLines counts for a hunk.
func finalizeHunk(hunk *DiffHunk) {
	for _, line := range hunk.Lines {
		if line.Type == "context" || line.Type == "deletion" {
			hunk.OldLines++
		}
		if line.Type == "context" || line.Type == "addition" {
			hunk.NewLines++
		}
	}
}
