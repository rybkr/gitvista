package gitcore

import (
	"strings"
	"testing"
)

func TestComputeThreeWayDiff_AllIdentical(t *testing.T) {
	repo := setupTestRepo(t)

	content := []byte("line1\nline2\nline3\n")
	blob := createBlob(t, repo, content)

	result, err := ComputeThreeWayDiff(repo, blob, blob, blob, "same.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if len(result.Regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result.Regions))
	}
	if result.Regions[0].Type != MergeRegionContext {
		t.Errorf("expected context region, got %s", result.Regions[0].Type)
	}
	if len(result.Regions[0].BaseLines) != 3 {
		t.Errorf("expected 3 base lines, got %d", len(result.Regions[0].BaseLines))
	}
}

func TestComputeThreeWayDiff_OursOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)

	base := createBlob(t, repo, []byte("line1\nline2\nline3\n"))
	ours := createBlob(t, repo, []byte("line1\nmodified\nline3\n"))
	theirs := base

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "ours.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	hasOursRegion := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionOurs {
			hasOursRegion = true
			if len(r.OursLines) == 0 {
				t.Error("ours region should have replacement lines")
			}
		}
		if r.Type == MergeRegionConflict {
			t.Error("should not have conflict when only ours changed")
		}
	}
	if !hasOursRegion {
		t.Error("expected at least one ours region")
	}
	if result.Stats.ConflictRegions != 0 {
		t.Errorf("expected 0 conflict regions, got %d", result.Stats.ConflictRegions)
	}
}

func TestComputeThreeWayDiff_TheirsOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)

	base := createBlob(t, repo, []byte("line1\nline2\nline3\n"))
	ours := base
	theirs := createBlob(t, repo, []byte("line1\ntheirs-modified\nline3\n"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "theirs.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	hasTheirsRegion := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionTheirs {
			hasTheirsRegion = true
			if len(r.TheirsLines) == 0 {
				t.Error("theirs region should have replacement lines")
			}
		}
		if r.Type == MergeRegionConflict {
			t.Error("should not have conflict when only theirs changed")
		}
	}
	if !hasTheirsRegion {
		t.Error("expected at least one theirs region")
	}
	if result.Stats.ConflictRegions != 0 {
		t.Errorf("expected 0 conflict regions, got %d", result.Stats.ConflictRegions)
	}
}

func TestComputeThreeWayDiff_NonOverlappingChanges(t *testing.T) {
	repo := setupTestRepo(t)

	baseContent := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	oursContent := "line1\nours-changed\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	theirsContent := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\ntheirs-changed\nline10\n"

	base := createBlob(t, repo, []byte(baseContent))
	ours := createBlob(t, repo, []byte(oursContent))
	theirs := createBlob(t, repo, []byte(theirsContent))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "nonoverlap.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if result.Stats.ConflictRegions != 0 {
		t.Errorf("expected 0 conflicts, got %d", result.Stats.ConflictRegions)
	}

	hasOurs := false
	hasTheirs := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionOurs {
			hasOurs = true
		}
		if r.Type == MergeRegionTheirs {
			hasTheirs = true
		}
	}
	if !hasOurs || !hasTheirs {
		t.Errorf("expected both ours and theirs regions, got ours=%v theirs=%v", hasOurs, hasTheirs)
	}
}

func TestComputeThreeWayDiff_OverlappingChanges(t *testing.T) {
	repo := setupTestRepo(t)

	baseContent := "line1\nline2\nline3\nline4\nline5\n"
	oursContent := "line1\nours-A\nours-B\nours-C\nline5\n"
	theirsContent := "line1\ntheirs-A\ntheirs-B\ntheirs-C\nline5\n"

	base := createBlob(t, repo, []byte(baseContent))
	ours := createBlob(t, repo, []byte(oursContent))
	theirs := createBlob(t, repo, []byte(theirsContent))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "overlap.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if result.Stats.ConflictRegions != 1 {
		t.Errorf("expected 1 conflict region, got %d", result.Stats.ConflictRegions)
	}

	hasConflict := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionConflict {
			hasConflict = true
			if len(r.OursLines) == 0 || len(r.TheirsLines) == 0 {
				t.Error("conflict region should have both ours and theirs lines")
			}
		}
	}
	if !hasConflict {
		t.Error("expected a conflict region")
	}
}

func TestComputeThreeWayDiff_IdenticalChanges(t *testing.T) {
	repo := setupTestRepo(t)

	baseContent := "line1\nline2\nline3\n"
	changedContent := "line1\nsame-fix\nline3\n"

	base := createBlob(t, repo, []byte(baseContent))
	changed := createBlob(t, repo, []byte(changedContent))

	result, err := ComputeThreeWayDiff(repo, base, changed, changed, "identical.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if result.Stats.ConflictRegions != 0 {
		t.Errorf("identical changes should have 0 conflicts, got %d", result.Stats.ConflictRegions)
	}

	for _, r := range result.Regions {
		if r.Type == MergeRegionConflict {
			t.Error("identical changes should not produce a conflict region")
		}
	}
}

func TestComputeThreeWayDiff_BothAddedEmptyBase(t *testing.T) {
	repo := setupTestRepo(t)

	ours := createBlob(t, repo, []byte("ours content\n"))
	theirs := createBlob(t, repo, []byte("theirs content\n"))

	result, err := ComputeThreeWayDiff(repo, "", ours, theirs, "bothadded.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if result.ConflictType != ConflictBothAdded {
		t.Errorf("expected ConflictBothAdded, got %s", result.ConflictType)
	}

	if result.Stats.ConflictRegions != 1 {
		t.Errorf("expected 1 conflict region, got %d", result.Stats.ConflictRegions)
	}
}

func TestComputeThreeWayDiff_DeleteModify(t *testing.T) {
	repo := setupTestRepo(t)

	base := createBlob(t, repo, []byte("line1\nline2\n"))
	theirs := createBlob(t, repo, []byte("line1\nmodified\n"))

	// Ours deleted the file (empty hash).
	result, err := ComputeThreeWayDiff(repo, base, "", theirs, "deletemod.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if result.ConflictType != ConflictDeleteModify {
		t.Errorf("expected ConflictDeleteModify, got %s", result.ConflictType)
	}

	// Ours side is empty, theirs has content.
	hasOursOrConflict := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionOurs || r.Type == MergeRegionConflict {
			hasOursOrConflict = true
		}
	}
	if !hasOursOrConflict {
		t.Error("expected ours or conflict regions for delete/modify case")
	}
}

func TestComputeThreeWayDiff_BinaryFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Binary content (contains null byte).
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A, 0x1A}
	base := createBlob(t, repo, binaryContent)
	ours := createBlob(t, repo, append(binaryContent, 0xFF))
	theirs := createBlob(t, repo, append(binaryContent, 0xFE))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "image.png")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if !result.IsBinary {
		t.Error("expected IsBinary = true for binary content")
	}
	if len(result.Regions) != 0 {
		t.Errorf("expected 0 regions for binary file, got %d", len(result.Regions))
	}
}

func TestComputeThreeWayDiff_LargeFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create content just over the maxBlobSize limit.
	large := strings.Repeat("x", 513*1024) // 513KB
	base := createBlob(t, repo, []byte(large))
	ours := createBlob(t, repo, []byte(large+"a"))
	theirs := createBlob(t, repo, []byte(large+"b"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "large.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if !result.Truncated {
		t.Error("expected Truncated = true for large file")
	}
	if len(result.Regions) != 0 {
		t.Errorf("expected 0 regions for truncated file, got %d", len(result.Regions))
	}
}

// TestComputeThreeWayDiff_MultiBlockOverlap tests that a large ours block
// overlapping with multiple theirs blocks produces a single conflict region
// with all content from both sides, without double-counting base lines.
func TestComputeThreeWayDiff_MultiBlockOverlap(t *testing.T) {
	repo := setupTestRepo(t)

	baseContent := "a\nb\nc\nd\ne\nf\n"
	// Ours replaces lines b-e (large block).
	oursContent := "a\nX\nY\nf\n"
	// Theirs makes two small changes within that range.
	theirsContent := "a\nB-theirs\nc\nD-theirs\ne\nf\n"

	base := createBlob(t, repo, []byte(baseContent))
	ours := createBlob(t, repo, []byte(oursContent))
	theirs := createBlob(t, repo, []byte(theirsContent))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "multi.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	// Should have exactly 1 conflict region (not multiple with double-counted lines).
	if result.Stats.ConflictRegions != 1 {
		t.Errorf("expected 1 conflict region, got %d", result.Stats.ConflictRegions)
	}

	// Verify no base line appears in multiple regions.
	coveredLines := make(map[int]bool)
	for _, r := range result.Regions {
		for i := 0; i < len(r.BaseLines); i++ {
			line := r.BaseStart - 1 + i // convert 1-based to 0-based
			if coveredLines[line] {
				t.Errorf("base line %d appears in multiple regions", line)
			}
			coveredLines[line] = true
		}
	}
}

// TestComputeThreeWayDiff_ConflictTypeDeferred tests that ConflictType
// is set based on actual diff results, not pre-classified for normal cases.
func TestComputeThreeWayDiff_ConflictTypeDeferred(t *testing.T) {
	repo := setupTestRepo(t)

	// Non-overlapping changes should NOT be classified as conflicting.
	baseContent := "line1\nline2\nline3\nline4\n"
	oursContent := "OURS\nline2\nline3\nline4\n"
	theirsContent := "line1\nline2\nline3\nTHEIRS\n"

	base := createBlob(t, repo, []byte(baseContent))
	ours := createBlob(t, repo, []byte(oursContent))
	theirs := createBlob(t, repo, []byte(theirsContent))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "clean.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff failed: %v", err)
	}

	if result.ConflictType != ConflictNone {
		t.Errorf("expected ConflictNone for non-overlapping changes, got %s", result.ConflictType)
	}
}

func TestEditsToBlocks_PureInsert(t *testing.T) {
	// Test that pure inserts (no deletes) produce valid blocks.
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "x", "b", "c"}

	edits := computeEdits(oldLines, newLines)
	blocks := editsToBlocks(edits, oldLines, newLines)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if len(blocks[0].newLines) != 1 || blocks[0].newLines[0] != "x" {
		t.Errorf("expected insert of 'x', got %v", blocks[0].newLines)
	}
}

func TestEditsToBlocks_PureDelete(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "c"}

	edits := computeEdits(oldLines, newLines)
	blocks := editsToBlocks(edits, oldLines, newLines)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].baseStart != 1 || blocks[0].baseEnd != 2 {
		t.Errorf("expected base range [1,2), got [%d,%d)", blocks[0].baseStart, blocks[0].baseEnd)
	}
	if len(blocks[0].newLines) != 0 {
		t.Errorf("expected 0 new lines for delete, got %d", len(blocks[0].newLines))
	}
}
