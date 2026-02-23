package gitcore

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	maxDiffEntries = 500        // Maximum number of file changes to process
	maxBlobSize    = 512 * 1024 // Maximum blob size for diff (512KB)

	// DefaultContextLines is the number of unchanged lines to include around each
	// change in unified diff output.
	DefaultContextLines = 3
)

// TreeDiff recursively compares two trees and returns a flat list of changed files.
// oldTreeHash can be empty for root commits. prefix builds full paths during recursion.
// Returns an error if the number of entries exceeds maxDiffEntries.
// Rename detection is applied once after the full recursive traversal so that
// cross-directory renames (the common case) are correctly identified.
func TreeDiff(repo *Repository, oldTreeHash, newTreeHash Hash, prefix string) ([]DiffEntry, error) {
	entries, err := treeDiffRecursive(repo, oldTreeHash, newTreeHash, prefix)
	if err != nil {
		return nil, err
	}
	return detectRenames(entries), nil
}

// treeDiffRecursive is the internal implementation of TreeDiff. It recurses into
// sub-trees and collects raw added/deleted/modified entries without rename detection.
// Rename detection is deferred to TreeDiff so it operates on the complete flat list.
func treeDiffRecursive(repo *Repository, oldTreeHash, newTreeHash Hash, prefix string) ([]DiffEntry, error) {
	entries := make([]DiffEntry, 0)

	var oldTree *Tree
	if oldTreeHash != "" {
		var err error
		oldTree, err = repo.GetTree(oldTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get old tree %s: %w", oldTreeHash, err)
		}
	}

	var newTree *Tree
	if newTreeHash != "" {
		var err error
		newTree, err = repo.GetTree(newTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get new tree %s: %w", newTreeHash, err)
		}
	}

	oldEntries := make(map[string]TreeEntry)
	if oldTree != nil {
		for _, entry := range oldTree.Entries {
			oldEntries[entry.Name] = entry
		}
	}

	newEntries := make(map[string]TreeEntry)
	if newTree != nil {
		for _, entry := range newTree.Entries {
			newEntries[entry.Name] = entry
		}
	}

	allNames := make(map[string]bool)
	for name := range oldEntries {
		allNames[name] = true
	}
	for name := range newEntries {
		allNames[name] = true
	}

	for name := range allNames {
		oldEntry, existsInOld := oldEntries[name]
		newEntry, existsInNew := newEntries[name]

		path := name
		if prefix != "" {
			path = prefix + "/" + name
		}

		if len(entries) >= maxDiffEntries {
			return nil, fmt.Errorf("diff too large: exceeded maximum of %d entries", maxDiffEntries)
		}

		switch {
		case !existsInOld && existsInNew:
			if isTreeEntry(newEntry) {
				subEntries, err := treeDiffRecursive(repo, "", newEntry.ID, path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, subEntries...)
			} else {
				entries = append(entries, DiffEntry{
					Path:     path,
					Status:   DiffStatusAdded,
					NewHash:  newEntry.ID,
					IsBinary: isSubmodule(newEntry),
					NewMode:  newEntry.Mode,
				})
			}

		case existsInOld && !existsInNew:
			if isTreeEntry(oldEntry) {
				subEntries, err := treeDiffRecursive(repo, oldEntry.ID, "", path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, subEntries...)
			} else {
				entries = append(entries, DiffEntry{
					Path:     path,
					Status:   DiffStatusDeleted,
					OldHash:  oldEntry.ID,
					IsBinary: isSubmodule(oldEntry),
					OldMode:  oldEntry.Mode,
				})
			}

		case existsInOld && existsInNew:
			if oldEntry.ID != newEntry.ID {
				if isTreeEntry(oldEntry) && isTreeEntry(newEntry) {
					subEntries, err := treeDiffRecursive(repo, oldEntry.ID, newEntry.ID, path)
					if err != nil {
						return nil, err
					}
					entries = append(entries, subEntries...)
				} else if isTreeEntry(oldEntry) || isTreeEntry(newEntry) {
					// Type changed (file <-> directory): emit delete + add
					if isTreeEntry(oldEntry) {
						subEntries, err := treeDiffRecursive(repo, oldEntry.ID, "", path)
						if err != nil {
							return nil, err
						}
						entries = append(entries, subEntries...)
					} else {
						entries = append(entries, DiffEntry{
							Path:     path,
							Status:   DiffStatusDeleted,
							OldHash:  oldEntry.ID,
							IsBinary: isSubmodule(oldEntry),
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
						entries = append(entries, DiffEntry{
							Path:     path,
							Status:   DiffStatusAdded,
							NewHash:  newEntry.ID,
							IsBinary: isSubmodule(newEntry),
							NewMode:  newEntry.Mode,
						})
					}
				} else {
					entries = append(entries, DiffEntry{
						Path:     path,
						Status:   DiffStatusModified,
						OldHash:  oldEntry.ID,
						NewHash:  newEntry.ID,
						IsBinary: isSubmodule(oldEntry) || isSubmodule(newEntry),
						OldMode:  oldEntry.Mode,
						NewMode:  newEntry.Mode,
					})
				}
			}
		}
	}

	return entries, nil
}

// detectRenames post-processes diff entries to identify file renames.
// A rename is detected when a deleted file and an added file share the same
// blob hash (exact content match). Content-identical renames are common after
// refactors (e.g., moving a file to a new package without editing it).
// Files with different content are left as separate delete+add entries.
//
// Multiple deleted files sharing the same blob hash (e.g., duplicated config
// files) are each tracked independently so they can be paired correctly with
// any matching added files.
func detectRenames(entries []DiffEntry) []DiffEntry {
	type deletedInfo struct {
		index int
		path  string
		mode  string
	}

	// Use a slice per hash so that multiple deleted files with identical
	// content are all tracked and can be paired without non-determinism.
	deletedByHash := make(map[Hash][]deletedInfo)
	for i, entry := range entries {
		if entry.Status == DiffStatusDeleted && entry.OldHash != "" {
			deletedByHash[entry.OldHash] = append(deletedByHash[entry.OldHash], deletedInfo{
				index: i,
				path:  entry.Path,
				mode:  entry.OldMode,
			})
		}
	}

	if len(deletedByHash) == 0 {
		return entries
	}

	// Track consumed positions in each candidate slice to handle many-to-many
	// cases without revisiting already-paired deletes.
	consumed := make(map[Hash]int)
	matched := make(map[int]bool)

	for i := range entries {
		if entries[i].Status != DiffStatusAdded || entries[i].NewHash == "" {
			continue
		}
		candidates := deletedByHash[entries[i].NewHash]
		idx := consumed[entries[i].NewHash]
		if idx >= len(candidates) {
			continue
		}
		info := candidates[idx]
		consumed[entries[i].NewHash] = idx + 1

		// Promote this added entry to a rename.
		entries[i].Status = DiffStatusRenamed
		entries[i].OldPath = info.path
		entries[i].OldHash = entries[i].NewHash
		entries[i].OldMode = info.mode
		matched[info.index] = true
	}

	// Remove the matched deleted entries, preserving all other entries in order.
	if len(matched) == 0 {
		return entries
	}
	result := make([]DiffEntry, 0, len(entries)-len(matched))
	for i, entry := range entries {
		if !matched[i] {
			result = append(result, entry)
		}
	}
	return result
}

func isTreeEntry(entry TreeEntry) bool {
	return entry.Type == objectTypeTree || entry.Mode == "040000" || entry.Mode == "40000"
}

func isSubmodule(entry TreeEntry) bool {
	return entry.Mode == "160000"
}

// ComputeFileDiff computes a line-level unified diff between two blobs.
// Empty hash for oldBlobHash means added file; empty newBlobHash means deleted.
// Files exceeding maxBlobSize are returned with Truncated=true.
func ComputeFileDiff(repo *Repository, oldBlobHash, newBlobHash Hash, path string, contextLines int) (*FileDiff, error) {
	result := &FileDiff{
		Path:    path,
		OldHash: oldBlobHash,
		NewHash: newBlobHash,
		Hunks:   make([]DiffHunk, 0),
	}

	var oldContent []byte
	if oldBlobHash != "" {
		var err error
		oldContent, err = repo.GetBlob(oldBlobHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read old blob %s: %w", oldBlobHash, err)
		}
	}

	var newContent []byte
	if newBlobHash != "" {
		var err error
		newContent, err = repo.GetBlob(newBlobHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read new blob %s: %w", newBlobHash, err)
		}
	}

	if len(oldContent) > maxBlobSize || len(newContent) > maxBlobSize {
		result.Truncated = true
		return result, nil
	}

	if isBinaryContent(oldContent) || isBinaryContent(newContent) {
		result.IsBinary = true
		return result, nil
	}

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)
	result.Hunks = myersDiff(oldLines, newLines, contextLines)

	return result, nil
}

// isBinaryContent uses Git's heuristic: checks first 8KB for null bytes.
func isBinaryContent(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	return bytes.IndexByte(data[:limit], 0) != -1
}

// splitLines splits on newlines, removing a trailing empty element if content ends with \n.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return []string{}
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// myersDiff implements Myers' O(ND) diff algorithm, returning hunks with context lines.
func myersDiff(oldLines, newLines []string, context int) []DiffHunk {
	if len(oldLines) == 0 && len(newLines) == 0 {
		return []DiffHunk{}
	}

	edits := computeEdits(oldLines, newLines)
	if len(edits) == 0 {
		return []DiffHunk{}
	}

	return buildHunks(oldLines, newLines, edits, context)
}

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
				// Save V state for this final d before backtracking
				vCopy := make([]int, len(v))
				copy(vCopy, v)
				trace = append(trace, vCopy)
				return backtrack(oldLines, newLines, trace, d, max)
			}
		}

		// Save V array AFTER processing this edit distance so that
		// trace[d] reflects the state at the end of d, not before it.
		vCopy := make([]int, len(v))
		copy(vCopy, v)
		trace = append(trace, vCopy)
	}

	// Should not reach here for valid input
	return []edit{}
}

// backtrack reconstructs the edit script from the trace.
// Builds in reverse order then flips once to avoid O(n^2) prepend allocations.
func backtrack(oldLines, newLines []string, trace [][]int, d int, max int) []edit {
	edits := make([]edit, 0)
	x := len(oldLines)
	y := len(newLines)

	for depth := d; depth > 0; depth-- {
		vPrev := trace[depth-1]
		k := x - y
		kIdx := k + max

		var prevK int
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

		for x > prevX && y > prevY && x > 0 && y > 0 && oldLines[x-1] == newLines[y-1] {
			x--
			y--
			edits = append(edits, edit{Type: editKeep, OldLine: x, NewLine: y})
		}

		if prevY < 0 {
			prevY = 0
		}

		if x > prevX {
			x--
			edits = append(edits, edit{Type: editDelete, OldLine: x})
		} else if y > prevY {
			y--
			edits = append(edits, edit{Type: editInsert, NewLine: y})
		}
	}

	for x > 0 && y > 0 {
		x--
		y--
		edits = append(edits, edit{Type: editKeep, OldLine: x, NewLine: y})
	}
	for x > 0 {
		x--
		edits = append(edits, edit{Type: editDelete, OldLine: x})
	}
	for y > 0 {
		y--
		edits = append(edits, edit{Type: editInsert, NewLine: y})
	}

	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
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
					// Context lines between lastChangeIdx and i were already
					// added by the else branch on previous iterations.  Trim
					// to keep only `context` trailing lines after the last change.
					excess := (i - lastChangeIdx - 1) - context
					if excess > 0 {
						currentHunk.Lines = currentHunk.Lines[:len(currentHunk.Lines)-excess]
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

	// Close final hunk — trim trailing context to at most `context` lines.
	if currentHunk != nil {
		trailingContext := 0
		for k := len(currentHunk.Lines) - 1; k >= 0; k-- {
			if currentHunk.Lines[k].Type == LineTypeContext {
				trailingContext++
			} else {
				break
			}
		}
		if trailingContext > context {
			currentHunk.Lines = currentHunk.Lines[:len(currentHunk.Lines)-(trailingContext-context)]
		}

		finalizeHunk(currentHunk)
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

// finalizeHunk computes the OldLines and NewLines counts for a hunk.
func finalizeHunk(hunk *DiffHunk) {
	for _, line := range hunk.Lines {
		if line.Type == LineTypeContext || line.Type == LineTypeDeletion {
			hunk.OldLines++
		}
		if line.Type == LineTypeContext || line.Type == LineTypeAddition {
			hunk.NewLines++
		}
	}
}
