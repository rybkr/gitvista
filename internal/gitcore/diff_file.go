package gitcore

import (
	"bytes"
	"fmt"
	"strings"
)

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

	if IsBinaryContent(oldContent) || IsBinaryContent(newContent) {
		result.IsBinary = true
		return result, nil
	}

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)
	result.Hunks = myersDiff(oldLines, newLines, contextLines)

	return result, nil
}

// IsBinaryContent uses Git's heuristic: checks first 8KB for null bytes.
func IsBinaryContent(data []byte) bool {
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

	if n == 0 && m == 0 {
		return []edit{}
	}

	v := make([]int, 2*max+1)
	trace := make([][]int, 0)

	for d := 0; d <= max; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			kIdx := k + max

			if k == -d || (k != d && v[kIdx-1] < v[kIdx+1]) {
				x = v[kIdx+1]
			} else {
				x = v[kIdx-1] + 1
			}

			y := x - k

			for x < n && y < m && oldLines[x] == newLines[y] {
				x++
				y++
			}

			v[kIdx] = x

			if x >= n && y >= m {
				vCopy := make([]int, len(v))
				copy(vCopy, v)
				trace = append(trace, vCopy)
				return backtrack(oldLines, newLines, trace, d, max)
			}
		}

		vCopy := make([]int, len(v))
		copy(vCopy, v)
		trace = append(trace, vCopy)
	}

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

	var currentHunk *DiffHunk
	lastChangeIdx := -1

	for i, edit := range edits {
		isChange := edit.Type != editKeep

		if isChange {
			if currentHunk == nil {
				currentHunk = &DiffHunk{
					Lines: make([]DiffLine, 0),
				}

				contextStart := i - context
				if contextStart < 0 {
					contextStart = 0
				}
				for j := contextStart; j < i; j++ {
					if edits[j].Type == editKeep {
						currentHunk.Lines = append(currentHunk.Lines, DiffLine{
							Type:    LineTypeContext,
							Content: oldLines[edits[j].OldLine],
							OldLine: edits[j].OldLine + 1,
							NewLine: edits[j].NewLine + 1,
						})
					}
				}

				if len(currentHunk.Lines) > 0 {
					currentHunk.OldStart = currentHunk.Lines[0].OldLine
					currentHunk.NewStart = currentHunk.Lines[0].NewLine
				} else {
					switch edit.Type {
					case editDelete:
						currentHunk.OldStart = edit.OldLine + 1
						if len(newLines) > 0 {
							currentHunk.NewStart = 1
						} else {
							currentHunk.NewStart = 0
						}
					case editInsert:
						currentHunk.NewStart = edit.NewLine + 1
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
			switch edit.Type {
			case editKeep:
				if lastChangeIdx >= 0 && i-lastChangeIdx > context*2 {
					excess := (i - lastChangeIdx - 1) - context
					if excess > 0 {
						currentHunk.Lines = currentHunk.Lines[:len(currentHunk.Lines)-excess]
					}

					finalizeHunk(currentHunk)
					hunks = append(hunks, *currentHunk)
					currentHunk = nil
					lastChangeIdx = -1
				} else {
					currentHunk.Lines = append(currentHunk.Lines, DiffLine{
						Type:    LineTypeContext,
						Content: oldLines[edit.OldLine],
						OldLine: edit.OldLine + 1,
						NewLine: edit.NewLine + 1,
					})
				}

			case editDelete:
				if edit.OldLine < len(oldLines) {
					currentHunk.Lines = append(currentHunk.Lines, DiffLine{
						Type:    LineTypeDeletion,
						Content: oldLines[edit.OldLine],
						OldLine: edit.OldLine + 1,
						NewLine: 0,
					})
				}

			case editInsert:
				if edit.NewLine < len(newLines) {
					currentHunk.Lines = append(currentHunk.Lines, DiffLine{
						Type:    LineTypeAddition,
						Content: newLines[edit.NewLine],
						OldLine: 0,
						NewLine: edit.NewLine + 1,
					})
				}
			}
		}
	}

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
