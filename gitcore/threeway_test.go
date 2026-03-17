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
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if len(result.Regions) != 1 || result.Regions[0].Type != MergeRegionContext {
		t.Fatalf("regions = %#v, want single context region", result.Regions)
	}
}

func TestComputeThreeWayDiff_OursOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\nline3\n"))
	ours := createBlob(t, repo, []byte("line1\nmodified\nline3\n"))

	result, err := ComputeThreeWayDiff(repo, base, ours, base, "ours.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	hasOursRegion := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionOurs {
			hasOursRegion = true
		}
		if r.Type == MergeRegionConflict {
			t.Fatal("unexpected conflict region")
		}
	}
	if !hasOursRegion || result.Stats.ConflictRegions != 0 {
		t.Fatalf("regions/stats = %#v %+v, want ours region and no conflicts", result.Regions, result.Stats)
	}
}

func TestComputeThreeWayDiff_TheirsOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\nline3\n"))
	theirs := createBlob(t, repo, []byte("line1\ntheirs-modified\nline3\n"))

	result, err := ComputeThreeWayDiff(repo, base, base, theirs, "theirs.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	hasTheirsRegion := false
	for _, r := range result.Regions {
		if r.Type == MergeRegionTheirs {
			hasTheirsRegion = true
		}
		if r.Type == MergeRegionConflict {
			t.Fatal("unexpected conflict region")
		}
	}
	if !hasTheirsRegion || result.Stats.ConflictRegions != 0 {
		t.Fatalf("regions/stats = %#v %+v, want theirs region and no conflicts", result.Regions, result.Stats)
	}
}

func TestComputeThreeWayDiff_NonOverlappingChanges(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"))
	ours := createBlob(t, repo, []byte("line1\nours-changed\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"))
	theirs := createBlob(t, repo, []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\ntheirs-changed\nline10\n"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "nonoverlap.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.Stats.ConflictRegions != 0 {
		t.Fatalf("ConflictRegions = %d, want 0", result.Stats.ConflictRegions)
	}
}

func TestComputeThreeWayDiff_OverlappingChanges(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\nline3\nline4\nline5\n"))
	ours := createBlob(t, repo, []byte("line1\nours-A\nours-B\nours-C\nline5\n"))
	theirs := createBlob(t, repo, []byte("line1\ntheirs-A\ntheirs-B\ntheirs-C\nline5\n"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "overlap.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.Stats.ConflictRegions != 1 {
		t.Fatalf("ConflictRegions = %d, want 1", result.Stats.ConflictRegions)
	}
}

func TestComputeThreeWayDiff_IdenticalChanges(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\nline3\n"))
	changed := createBlob(t, repo, []byte("line1\nsame-fix\nline3\n"))

	result, err := ComputeThreeWayDiff(repo, base, changed, changed, "identical.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.Stats.ConflictRegions != 0 {
		t.Fatalf("ConflictRegions = %d, want 0", result.Stats.ConflictRegions)
	}
}

func TestComputeThreeWayDiff_BothAddedEmptyBase(t *testing.T) {
	repo := setupTestRepo(t)
	ours := createBlob(t, repo, []byte("ours content\n"))
	theirs := createBlob(t, repo, []byte("theirs content\n"))

	result, err := ComputeThreeWayDiff(repo, "", ours, theirs, "bothadded.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.ConflictType != ConflictBothAdded || result.Stats.ConflictRegions != 1 {
		t.Fatalf("conflict=%s stats=%+v, want both_added with 1 conflict region", result.ConflictType, result.Stats)
	}
}

func TestComputeThreeWayDiff_DeleteModify(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\n"))
	theirs := createBlob(t, repo, []byte("line1\nmodified\n"))

	result, err := ComputeThreeWayDiff(repo, base, "", theirs, "deletemod.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.ConflictType != ConflictDeleteModify {
		t.Fatalf("ConflictType = %s, want %s", result.ConflictType, ConflictDeleteModify)
	}
}

func TestComputeThreeWayDiff_BinaryFile(t *testing.T) {
	repo := setupTestRepo(t)
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A, 0x1A}
	base := createBlob(t, repo, binaryContent)
	ours := createBlob(t, repo, append(binaryContent, 0xFF))
	theirs := createBlob(t, repo, append(binaryContent, 0xFE))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "image.png")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if !result.IsBinary || len(result.Regions) != 0 {
		t.Fatalf("binary result = %+v, want IsBinary and no regions", result)
	}
}

func TestComputeThreeWayDiff_LargeFile(t *testing.T) {
	repo := setupTestRepo(t)
	large := strings.Repeat("x", 513*1024)
	base := createBlob(t, repo, []byte(large))
	ours := createBlob(t, repo, []byte(large+"a"))
	theirs := createBlob(t, repo, []byte(large+"b"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "large.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if !result.Truncated || len(result.Regions) != 0 {
		t.Fatalf("large result = %+v, want truncated with no regions", result)
	}
}

func TestComputeThreeWayDiff_MultiBlockOverlap(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("a\nb\nc\nd\ne\nf\n"))
	ours := createBlob(t, repo, []byte("a\nX\nY\nf\n"))
	theirs := createBlob(t, repo, []byte("a\nB-theirs\nc\nD-theirs\ne\nf\n"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "multi.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.Stats.ConflictRegions != 1 {
		t.Fatalf("ConflictRegions = %d, want 1", result.Stats.ConflictRegions)
	}

	coveredLines := make(map[int]bool)
	for _, r := range result.Regions {
		for i := 0; i < len(r.BaseLines); i++ {
			line := r.BaseStart - 1 + i
			if coveredLines[line] {
				t.Fatalf("base line %d appears in multiple regions", line)
			}
			coveredLines[line] = true
		}
	}
}

func TestComputeThreeWayDiff_ConflictTypeDeferred(t *testing.T) {
	repo := setupTestRepo(t)
	base := createBlob(t, repo, []byte("line1\nline2\nline3\nline4\n"))
	ours := createBlob(t, repo, []byte("OURS\nline2\nline3\nline4\n"))
	theirs := createBlob(t, repo, []byte("line1\nline2\nline3\nTHEIRS\n"))

	result, err := ComputeThreeWayDiff(repo, base, ours, theirs, "clean.txt")
	if err != nil {
		t.Fatalf("ComputeThreeWayDiff() error = %v", err)
	}
	if result.ConflictType != ConflictNone {
		t.Fatalf("ConflictType = %s, want %s", result.ConflictType, ConflictNone)
	}
}

func TestEditsToBlocks_PureInsert(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "x", "b", "c"}

	blocks := editsToBlocks(computeEdits(oldLines, newLines), oldLines, newLines)
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if len(blocks[0].newLines) != 1 || blocks[0].newLines[0] != "x" {
		t.Fatalf("newLines = %#v, want [x]", blocks[0].newLines)
	}
}

func TestEditsToBlocks_PureDelete(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "c"}

	blocks := editsToBlocks(computeEdits(oldLines, newLines), oldLines, newLines)
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].baseStart != 1 || blocks[0].baseEnd != 2 {
		t.Fatalf("range = [%d,%d), want [1,2)", blocks[0].baseStart, blocks[0].baseEnd)
	}
	if len(blocks[0].newLines) != 0 {
		t.Fatalf("newLines len = %d, want 0", len(blocks[0].newLines))
	}
}
