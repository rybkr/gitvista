package gitcore

import (
	"fmt"
	"sort"
)

// editBlock represents a contiguous range of edits mapped to base line ranges.
type editBlock struct {
	baseStart int      // 0-based inclusive start in base
	baseEnd   int      // 0-based exclusive end in base (lines consumed)
	newLines  []string // replacement lines
}

// editsToBlocks converts a sequence of edits from computeEdits into contiguous
// blocks of changes. Each block records which base lines are consumed and what
// replaces them.
func editsToBlocks(edits []edit, oldLines, newLines []string) []editBlock {
	blocks := make([]editBlock, 0)
	i := 0
	for i < len(edits) {
		if edits[i].Type == editKeep {
			i++
			continue
		}

		// Start of a new block — collect all consecutive non-keep edits.
		block := editBlock{
			baseStart: -1,
			baseEnd:   -1,
			newLines:  make([]string, 0),
		}

		for i < len(edits) && edits[i].Type != editKeep {
			switch edits[i].Type {
			case editDelete:
				if block.baseStart == -1 {
					block.baseStart = edits[i].OldLine
				}
				block.baseEnd = edits[i].OldLine + 1
			case editInsert:
				if edits[i].NewLine < len(newLines) {
					block.newLines = append(block.newLines, newLines[edits[i].NewLine])
				}
			}
			i++
		}

		// If block only has inserts (no deletes), anchor it at the position
		// where the insert occurs in the base.
		if block.baseStart == -1 {
			// Find the base position by looking at surrounding context.
			if i < len(edits) {
				block.baseStart = edits[i].OldLine
			} else {
				block.baseStart = len(oldLines)
			}
			block.baseEnd = block.baseStart
		}

		blocks = append(blocks, block)
	}
	return blocks
}

// ComputeThreeWayDiff computes a three-way merge diff between base, ours, and
// theirs versions of a file. It returns regions classified as context, ours-only,
// theirs-only, or conflict.
func ComputeThreeWayDiff(repo *Repository, baseHash, oursHash, theirsHash Hash, path string) (*ThreeWayFileDiff, error) {
	result := &ThreeWayFileDiff{
		Path:    path,
		Regions: make([]MergeRegion, 0),
	}

	// Pre-classify conflict type based on hash patterns for structural conflicts.
	// For the normal case (all hashes present), defer to after the merge walk.
	switch {
	case baseHash == "" && oursHash != "" && theirsHash != "":
		result.ConflictType = ConflictBothAdded
	case oursHash == "" && theirsHash != "":
		result.ConflictType = ConflictDeleteModify
	case oursHash != "" && theirsHash == "":
		result.ConflictType = ConflictDeleteModify
	}

	// Load blob contents — empty hash means empty content (added/deleted file).
	var baseContent, oursContent, theirsContent []byte
	var err error

	if baseHash != "" {
		baseContent, err = repo.GetBlob(baseHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read base blob %s: %w", baseHash, err)
		}
	}
	if oursHash != "" {
		oursContent, err = repo.GetBlob(oursHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read ours blob %s: %w", oursHash, err)
		}
	}
	if theirsHash != "" {
		theirsContent, err = repo.GetBlob(theirsHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read theirs blob %s: %w", theirsHash, err)
		}
	}

	// Check for binary content.
	if IsBinaryContent(baseContent) || IsBinaryContent(oursContent) || IsBinaryContent(theirsContent) {
		result.IsBinary = true
		return result, nil
	}

	// Check for size limits.
	if len(baseContent) > maxBlobSize || len(oursContent) > maxBlobSize || len(theirsContent) > maxBlobSize {
		result.Truncated = true
		return result, nil
	}

	baseLines := splitLines(baseContent)
	oursLines := splitLines(oursContent)
	theirsLines := splitLines(theirsContent)

	// Compute edits from base to each side.
	editsOurs := computeEdits(baseLines, oursLines)
	editsTheirs := computeEdits(baseLines, theirsLines)

	// Convert to edit blocks.
	blocksOurs := editsToBlocks(editsOurs, baseLines, oursLines)
	blocksTheirs := editsToBlocks(editsTheirs, baseLines, theirsLines)

	// Merge walk.
	result.Regions = mergeWalk(baseLines, blocksOurs, blocksTheirs)

	// Compute stats.
	result.Stats = computeThreeWayStats(result.Regions)

	// Set ConflictType based on actual diff results when not pre-classified.
	if result.ConflictType == "" {
		if result.Stats.ConflictRegions > 0 {
			result.ConflictType = ConflictConflicting
		} else {
			result.ConflictType = ConflictNone
		}
	}

	return result, nil
}

// mergeWalk performs a diff3-style walk over the base lines, interleaving edit
// blocks from both sides to produce classified merge regions.
func mergeWalk(baseLines []string, blocksOurs, blocksTheirs []editBlock) []MergeRegion {
	regions := make([]MergeRegion, 0)

	// Sort blocks by baseStart.
	sort.Slice(blocksOurs, func(i, j int) bool {
		return blocksOurs[i].baseStart < blocksOurs[j].baseStart
	})
	sort.Slice(blocksTheirs, func(i, j int) bool {
		return blocksTheirs[i].baseStart < blocksTheirs[j].baseStart
	})

	idxOurs := 0
	idxTheirs := 0
	basePos := 0

	for idxOurs < len(blocksOurs) || idxTheirs < len(blocksTheirs) {
		var nextOurs, nextTheirs *editBlock
		if idxOurs < len(blocksOurs) {
			nextOurs = &blocksOurs[idxOurs]
		}
		if idxTheirs < len(blocksTheirs) {
			nextTheirs = &blocksTheirs[idxTheirs]
		}

		// Determine the next block to process.
		if nextOurs != nil && nextTheirs != nil {
			// Check for overlap.
			if blocksOverlap(*nextOurs, *nextTheirs) {
				// Emit context before the overlap.
				overlapStart := nextOurs.baseStart
				if nextTheirs.baseStart < overlapStart {
					overlapStart = nextTheirs.baseStart
				}
				if basePos < overlapStart {
					regions = appendContext(regions, baseLines, basePos, overlapStart)
					basePos = overlapStart
				}

				// Determine the combined range, consuming ALL overlapping
				// blocks from both sides (not just one from each).
				overlapEnd := nextOurs.baseEnd
				if nextTheirs.baseEnd > overlapEnd {
					overlapEnd = nextTheirs.baseEnd
				}

				var combinedOurs []string
				combinedOurs = append(combinedOurs, blocksOurs[idxOurs].newLines...)
				oursStart := blocksOurs[idxOurs].baseStart
				oursEnd := blocksOurs[idxOurs].baseEnd
				idxOurs++
				for idxOurs < len(blocksOurs) && blockInRange(blocksOurs[idxOurs], overlapEnd) {
					combinedOurs = append(combinedOurs, blocksOurs[idxOurs].newLines...)
					if blocksOurs[idxOurs].baseEnd > overlapEnd {
						overlapEnd = blocksOurs[idxOurs].baseEnd
					}
					oursEnd = blocksOurs[idxOurs].baseEnd
					idxOurs++
				}

				var combinedTheirs []string
				combinedTheirs = append(combinedTheirs, blocksTheirs[idxTheirs].newLines...)
				theirsStart := blocksTheirs[idxTheirs].baseStart
				theirsEnd := blocksTheirs[idxTheirs].baseEnd
				idxTheirs++
				for idxTheirs < len(blocksTheirs) && blockInRange(blocksTheirs[idxTheirs], overlapEnd) {
					combinedTheirs = append(combinedTheirs, blocksTheirs[idxTheirs].newLines...)
					if blocksTheirs[idxTheirs].baseEnd > overlapEnd {
						overlapEnd = blocksTheirs[idxTheirs].baseEnd
					}
					theirsEnd = blocksTheirs[idxTheirs].baseEnd
					idxTheirs++
				}

				// Check if both sides made identical changes.
				if slicesEqual(combinedOurs, combinedTheirs) &&
					oursStart == theirsStart && oursEnd == theirsEnd {
					// Identical change — emit as ours (clean).
					regions = append(regions, MergeRegion{
						Type:      MergeRegionOurs,
						BaseStart: basePos + 1,
						BaseLines: copySlice(baseLines, basePos, overlapEnd),
						OursLines: combinedOurs,
					})
				} else {
					// True conflict.
					regions = append(regions, MergeRegion{
						Type:        MergeRegionConflict,
						BaseStart:   basePos + 1,
						BaseLines:   copySlice(baseLines, basePos, overlapEnd),
						OursLines:   combinedOurs,
						TheirsLines: combinedTheirs,
					})
				}

				basePos = overlapEnd
				continue
			}

			// No overlap — process whichever comes first.
			if nextOurs.baseStart <= nextTheirs.baseStart {
				if basePos < nextOurs.baseStart {
					regions = appendContext(regions, baseLines, basePos, nextOurs.baseStart)
					basePos = nextOurs.baseStart
				}
				regions = append(regions, MergeRegion{
					Type:      MergeRegionOurs,
					BaseStart: basePos + 1,
					BaseLines: copySlice(baseLines, basePos, nextOurs.baseEnd),
					OursLines: nextOurs.newLines,
				})
				basePos = nextOurs.baseEnd
				idxOurs++
			} else {
				if basePos < nextTheirs.baseStart {
					regions = appendContext(regions, baseLines, basePos, nextTheirs.baseStart)
					basePos = nextTheirs.baseStart
				}
				regions = append(regions, MergeRegion{
					Type:        MergeRegionTheirs,
					BaseStart:   basePos + 1,
					BaseLines:   copySlice(baseLines, basePos, nextTheirs.baseEnd),
					TheirsLines: nextTheirs.newLines,
				})
				basePos = nextTheirs.baseEnd
				idxTheirs++
			}
		} else if nextOurs != nil {
			if basePos < nextOurs.baseStart {
				regions = appendContext(regions, baseLines, basePos, nextOurs.baseStart)
				basePos = nextOurs.baseStart
			}
			regions = append(regions, MergeRegion{
				Type:      MergeRegionOurs,
				BaseStart: basePos + 1,
				BaseLines: copySlice(baseLines, basePos, nextOurs.baseEnd),
				OursLines: nextOurs.newLines,
			})
			basePos = nextOurs.baseEnd
			idxOurs++
		} else {
			if basePos < nextTheirs.baseStart {
				regions = appendContext(regions, baseLines, basePos, nextTheirs.baseStart)
				basePos = nextTheirs.baseStart
			}
			regions = append(regions, MergeRegion{
				Type:        MergeRegionTheirs,
				BaseStart:   basePos + 1,
				BaseLines:   copySlice(baseLines, basePos, nextTheirs.baseEnd),
				TheirsLines: nextTheirs.newLines,
			})
			basePos = nextTheirs.baseEnd
			idxTheirs++
		}
	}

	// Trailing context.
	if basePos < len(baseLines) {
		regions = appendContext(regions, baseLines, basePos, len(baseLines))
	}

	return regions
}

// blocksOverlap reports whether two edit blocks overlap or touch in base line space.
func blocksOverlap(a, b editBlock) bool {
	// Overlap if one starts before the other ends.
	// Also treat adjacent pure-inserts at the same position as overlapping.
	return a.baseStart < b.baseEnd && b.baseStart < a.baseEnd ||
		(a.baseStart == a.baseEnd && a.baseStart >= b.baseStart && a.baseStart <= b.baseEnd) ||
		(b.baseStart == b.baseEnd && b.baseStart >= a.baseStart && b.baseStart <= a.baseEnd)
}

// blockInRange reports whether an edit block falls within or touches the given
// overlap end position. Used to consume all blocks that belong to an overlap.
func blockInRange(b editBlock, overlapEnd int) bool {
	return b.baseStart < overlapEnd ||
		(b.baseStart == b.baseEnd && b.baseStart <= overlapEnd)
}

// appendContext appends a context region for base lines [from, to).
func appendContext(regions []MergeRegion, baseLines []string, from, to int) []MergeRegion {
	if from >= to {
		return regions
	}
	return append(regions, MergeRegion{
		Type:      MergeRegionContext,
		BaseStart: from + 1,
		BaseLines: copySlice(baseLines, from, to),
	})
}

// copySlice returns a copy of baseLines[from:to].
func copySlice(lines []string, from, to int) []string {
	if from >= to || from >= len(lines) {
		return []string{}
	}
	if to > len(lines) {
		to = len(lines)
	}
	result := make([]string, to-from)
	copy(result, lines[from:to])
	return result
}

// slicesEqual reports whether two string slices are equal.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// computeThreeWayStats computes statistics from merge regions.
func computeThreeWayStats(regions []MergeRegion) ThreeWayDiffStats {
	var stats ThreeWayDiffStats
	for _, r := range regions {
		switch r.Type {
		case MergeRegionOurs:
			stats.OursDeleted += len(r.BaseLines)
			stats.OursAdded += len(r.OursLines)
		case MergeRegionTheirs:
			stats.TheirsDeleted += len(r.BaseLines)
			stats.TheirsAdded += len(r.TheirsLines)
		case MergeRegionConflict:
			stats.ConflictRegions++
			stats.OursDeleted += len(r.BaseLines)
			stats.OursAdded += len(r.OursLines)
			stats.TheirsDeleted += len(r.BaseLines)
			stats.TheirsAdded += len(r.TheirsLines)
		}
	}
	return stats
}
