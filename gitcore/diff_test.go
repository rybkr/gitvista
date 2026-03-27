package gitcore

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1" // #nosec G505 -- test helper
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestRepo(t *testing.T) *Repository {
	t.Helper()
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	objectsDir := filepath.Join(gitDir, "objects")
	if err := os.MkdirAll(objectsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(objects): %v", err)
	}
	return &Repository{
		gitDir:        gitDir,
		workDir:       workDir,
		refs:          make(map[string]Hash),
		commits:       make([]*Commit, 0),
		commitMap:     make(map[Hash]*Commit),
		tags:          make([]*Tag, 0),
		stashes:       make([]*StashEntry, 0),
		packLocations: make(map[Hash]PackLocation),
		packReaders:   make(map[string]*PackReader),
	}
}

func createBlob(t *testing.T, repo *Repository, content []byte) Hash {
	t.Helper()
	header := []byte(fmt.Sprintf("blob %d\x00", len(content)))
	full := append(header, content...)
	sum := sha1.Sum(full) // #nosec G401 -- test helper
	hash := Hash(fmt.Sprintf("%x", sum[:]))

	dir := filepath.Join(repo.gitDir, "objects", string(hash[:2]))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(object dir): %v", err)
	}

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(full); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	path := filepath.Join(dir, string(hash[2:]))
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile(object): %v", err)
	}

	return hash
}

func createTree(t *testing.T, repo *Repository, entries []TreeEntry) Hash {
	t.Helper()
	var body bytes.Buffer
	for _, entry := range entries {
		fmt.Fprintf(&body, "%s %s\x00", entry.Mode, entry.Name)
		raw := hashFromHex(string(entry.ID))
		body.Write(raw[:])
	}

	header := []byte(fmt.Sprintf("tree %d\x00", body.Len()))
	full := append(header, body.Bytes()...)
	sum := sha1.Sum(full) // #nosec G401 -- test helper
	hash := Hash(fmt.Sprintf("%x", sum[:]))

	dir := filepath.Join(repo.gitDir, "objects", string(hash[:2]))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(tree dir): %v", err)
	}

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(full); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	path := filepath.Join(dir, string(hash[2:]))
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile(tree): %v", err)
	}

	return hash
}

func sha1Sum(data []byte) []byte {
	sum := sha1.Sum(data) // #nosec G401 -- test helper
	return sum[:]
}

func TestTreeDiff_AddedModifiedAndDeleted(t *testing.T) {
	repo := setupTestRepo(t)
	oldBlob := createBlob(t, repo, []byte("old"))
	newBlob := createBlob(t, repo, []byte("new"))
	addedBlob := createBlob(t, repo, []byte("added"))

	oldTree := createTree(t, repo, []TreeEntry{
		{ID: oldBlob, Name: "mod.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: oldBlob, Name: "gone.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	newTree := createTree(t, repo, []TreeEntry{
		{ID: newBlob, Name: "mod.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: addedBlob, Name: "new.txt", Mode: "100644", Type: ObjectTypeBlob},
	})

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	statuses := map[string]DiffStatus{}
	for _, entry := range entries {
		statuses[entry.Path] = entry.Status
	}
	if statuses["mod.txt"] != DiffStatusModified {
		t.Fatalf("mod.txt status = %v, want modified", statuses["mod.txt"])
	}
	if statuses["gone.txt"] != DiffStatusDeleted {
		t.Fatalf("gone.txt status = %v, want deleted", statuses["gone.txt"])
	}
	if statuses["new.txt"] != DiffStatusAdded {
		t.Fatalf("new.txt status = %v, want added", statuses["new.txt"])
	}
}

func TestTreeDiff_NestedAndRenameDetection(t *testing.T) {
	repo := setupTestRepo(t)
	content := []byte("package util\n\nfunc Helper() {}\n")
	blob := createBlob(t, repo, content)

	oldSrc := createTree(t, repo, []TreeEntry{{ID: blob, Name: "helper.go", Mode: "100644", Type: ObjectTypeBlob}})
	oldRoot := createTree(t, repo, []TreeEntry{{ID: oldSrc, Name: "src", Mode: "040000", Type: ObjectTypeTree}})

	newLib := createTree(t, repo, []TreeEntry{{ID: blob, Name: "helper.go", Mode: "100644", Type: ObjectTypeBlob}})
	newRoot := createTree(t, repo, []TreeEntry{{ID: newLib, Name: "lib", Mode: "040000", Type: ObjectTypeTree}})

	entries, err := TreeDiff(repo, oldRoot, newRoot, "")
	if err != nil {
		t.Fatalf("TreeDiff() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Status != DiffStatusRenamed {
		t.Fatalf("entry.Status = %v, want renamed", entry.Status)
	}
	if entry.OldPath != "src/helper.go" || entry.Path != "lib/helper.go" {
		t.Fatalf("rename = %q -> %q, want src/helper.go -> lib/helper.go", entry.OldPath, entry.Path)
	}
}

func TestTreeDiff_RecursiveAndTreeFileReplacement(t *testing.T) {
	repo := setupTestRepo(t)

	oldNestedBlob := createBlob(t, repo, []byte("old nested"))
	newNestedBlob := createBlob(t, repo, []byte("new nested"))
	oldDirToFileBlob := createBlob(t, repo, []byte("old dir file"))
	newDirBlob := createBlob(t, repo, []byte("new dir file"))
	oldFileBlob := createBlob(t, repo, []byte("old file"))
	newFileBlob := createBlob(t, repo, []byte("new plain file"))

	oldDirToDirTree := createTree(t, repo, []TreeEntry{
		{ID: oldNestedBlob, Name: "nested.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	newDirToDirTree := createTree(t, repo, []TreeEntry{
		{ID: newNestedBlob, Name: "nested.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	oldDirToFileTree := createTree(t, repo, []TreeEntry{
		{ID: oldDirToFileBlob, Name: "old.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	newFileToDirTree := createTree(t, repo, []TreeEntry{
		{ID: newDirBlob, Name: "new.txt", Mode: "100644", Type: ObjectTypeBlob},
	})

	oldRoot := createTree(t, repo, []TreeEntry{
		{ID: oldDirToDirTree, Name: "dir_to_dir", Mode: "040000", Type: ObjectTypeTree},
		{ID: oldDirToFileTree, Name: "dir_to_file", Mode: "040000", Type: ObjectTypeTree},
		{ID: oldFileBlob, Name: "file_to_dir", Mode: "100644", Type: ObjectTypeBlob},
	})
	newRoot := createTree(t, repo, []TreeEntry{
		{ID: newDirToDirTree, Name: "dir_to_dir", Mode: "040000", Type: ObjectTypeTree},
		{ID: newFileBlob, Name: "dir_to_file", Mode: "100644", Type: ObjectTypeBlob},
		{ID: newFileToDirTree, Name: "file_to_dir", Mode: "040000", Type: ObjectTypeTree},
	})

	entries, err := TreeDiff(repo, oldRoot, newRoot, "")
	if err != nil {
		t.Fatalf("TreeDiff() error = %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("len(entries) = %d, want 5", len(entries))
	}

	type diffKey struct {
		path   string
		status DiffStatus
	}
	got := make(map[diffKey]DiffEntry, len(entries))
	for _, entry := range entries {
		got[diffKey{path: entry.Path, status: entry.Status}] = entry
	}

	if entry, ok := got[diffKey{path: "dir_to_dir/nested.txt", status: DiffStatusModified}]; !ok {
		t.Fatal("missing modified entry for dir_to_dir/nested.txt")
	} else if entry.OldHash != oldNestedBlob || entry.NewHash != newNestedBlob {
		t.Fatalf("dir_to_dir/nested.txt hashes = %q -> %q, want %q -> %q", entry.OldHash, entry.NewHash, oldNestedBlob, newNestedBlob)
	}

	if entry, ok := got[diffKey{path: "dir_to_file/old.txt", status: DiffStatusDeleted}]; !ok {
		t.Fatal("missing deleted entry for dir_to_file/old.txt")
	} else if entry.OldHash != oldDirToFileBlob {
		t.Fatalf("dir_to_file/old.txt OldHash = %q, want %q", entry.OldHash, oldDirToFileBlob)
	}

	if entry, ok := got[diffKey{path: "dir_to_file", status: DiffStatusAdded}]; !ok {
		t.Fatal("missing added entry for dir_to_file")
	} else if entry.NewHash != newFileBlob {
		t.Fatalf("dir_to_file NewHash = %q, want %q", entry.NewHash, newFileBlob)
	}

	if entry, ok := got[diffKey{path: "file_to_dir", status: DiffStatusDeleted}]; !ok {
		t.Fatal("missing deleted entry for file_to_dir")
	} else if entry.OldHash != oldFileBlob {
		t.Fatalf("file_to_dir OldHash = %q, want %q", entry.OldHash, oldFileBlob)
	}

	if entry, ok := got[diffKey{path: "file_to_dir/new.txt", status: DiffStatusAdded}]; !ok {
		t.Fatal("missing added entry for file_to_dir/new.txt")
	} else if entry.NewHash != newDirBlob {
		t.Fatalf("file_to_dir/new.txt NewHash = %q, want %q", entry.NewHash, newDirBlob)
	}
}

func TestTreeDiff_Submodule(t *testing.T) {
	repo := setupTestRepo(t)
	oldTree := createTree(t, repo, []TreeEntry{{ID: Hash(strings.Repeat("a", 40)), Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})
	newTree := createTree(t, repo, []TreeEntry{{ID: Hash(strings.Repeat("b", 40)), Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if !entries[0].IsBinary {
		t.Fatal("submodule diff should be binary")
	}
}

func TestTreeDiff_InvalidHash(t *testing.T) {
	repo := setupTestRepo(t)

	entries, err := TreeDiff(repo, Hash(strings.Repeat("a", 40)), Hash(strings.Repeat("b", 40)), "")
	if err == nil || entries != nil {
		t.Fatalf("TreeDiff() expected error for invalid hash")
	}

	oldTree := createTree(t, repo, []TreeEntry{{ID: Hash(strings.Repeat("a", 40)), Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})

	entries, err = TreeDiff(repo, oldTree, Hash(strings.Repeat("b", 40)), "")
	if err == nil || entries != nil {
		t.Fatalf("TreeDiff() expected error for invalid hash")
	}
}

func TestTreeDiff_RecursiveErrorPaths(t *testing.T) {
	repo := setupTestRepo(t)

	validBlob := createBlob(t, repo, []byte("blob"))
	validLeafTree := createTree(t, repo, []TreeEntry{
		{ID: validBlob, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	emptyTree := createTree(t, repo, nil)
	invalidTreeID := Hash(strings.Repeat("a", 40))

	t.Run("added tree recursion failure", func(t *testing.T) {
		newRoot := createTree(t, repo, []TreeEntry{
			{ID: invalidTreeID, Name: "dir", Mode: "040000", Type: ObjectTypeTree},
		})

		entries, err := TreeDiff(repo, emptyTree, newRoot, "")
		if err == nil || entries != nil {
			t.Fatalf("TreeDiff() = (%v, %v), want recursive error", entries, err)
		}
	})

	t.Run("deleted tree recursion failure", func(t *testing.T) {
		oldRoot := createTree(t, repo, []TreeEntry{
			{ID: invalidTreeID, Name: "dir", Mode: "040000", Type: ObjectTypeTree},
		})

		entries, err := TreeDiff(repo, oldRoot, emptyTree, "")
		if err == nil || entries != nil {
			t.Fatalf("TreeDiff() = (%v, %v), want recursive error", entries, err)
		}
	})

	t.Run("tree to tree recursion failure", func(t *testing.T) {
		oldRoot := createTree(t, repo, []TreeEntry{
			{ID: invalidTreeID, Name: "dir", Mode: "040000", Type: ObjectTypeTree},
		})
		newRoot := createTree(t, repo, []TreeEntry{
			{ID: validLeafTree, Name: "dir", Mode: "040000", Type: ObjectTypeTree},
		})

		entries, err := TreeDiff(repo, oldRoot, newRoot, "")
		if err == nil || entries != nil {
			t.Fatalf("TreeDiff() = (%v, %v), want recursive error", entries, err)
		}
	})

	t.Run("tree to file old-side recursion failure", func(t *testing.T) {
		oldRoot := createTree(t, repo, []TreeEntry{
			{ID: invalidTreeID, Name: "node", Mode: "040000", Type: ObjectTypeTree},
		})
		newRoot := createTree(t, repo, []TreeEntry{
			{ID: validBlob, Name: "node", Mode: "100644", Type: ObjectTypeBlob},
		})

		entries, err := TreeDiff(repo, oldRoot, newRoot, "")
		if err == nil || entries != nil {
			t.Fatalf("TreeDiff() = (%v, %v), want recursive error", entries, err)
		}
	})

	t.Run("file to tree new-side recursion failure", func(t *testing.T) {
		oldRoot := createTree(t, repo, []TreeEntry{
			{ID: validBlob, Name: "node", Mode: "100644", Type: ObjectTypeBlob},
		})
		newRoot := createTree(t, repo, []TreeEntry{
			{ID: invalidTreeID, Name: "node", Mode: "040000", Type: ObjectTypeTree},
		})

		entries, err := TreeDiff(repo, oldRoot, newRoot, "")
		if err == nil || entries != nil {
			t.Fatalf("TreeDiff() = (%v, %v), want recursive error", entries, err)
		}
	})
}

func TestTreeDiff_TooLarge(t *testing.T) {
	repo := setupTestRepo(t)

	blob := createBlob(t, repo, []byte("same"))
	oldEntries := make([]TreeEntry, 0, maxDiffEntries+1)
	for i := 0; i < maxDiffEntries+1; i++ {
		oldEntries = append(oldEntries, TreeEntry{
			ID:   blob,
			Name: fmt.Sprintf("file-%04d.txt", i),
			Mode: "100644",
			Type: ObjectTypeBlob,
		})
	}

	oldTree := createTree(t, repo, oldEntries)
	newTree := createTree(t, repo, nil)

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err == nil || entries != nil {
		t.Fatalf("TreeDiff() = (%v, %v), want diff too large error", entries, err)
	}
}

func TestComputeFileDiff_TextBinaryAndLarge(t *testing.T) {
	repo := setupTestRepo(t)

	oldHash := createBlob(t, repo, []byte("line 1\nline 2\n"))
	newHash := createBlob(t, repo, []byte("line 1\nupdated line 2\n"))

	diff, err := ComputeFileDiff(repo, oldHash, newHash, "file.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff() error = %v", err)
	}
	if diff.IsBinary || diff.Truncated {
		t.Fatalf("diff flags = binary:%v truncated:%v, want false/false", diff.IsBinary, diff.Truncated)
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected textual hunks")
	}

	binaryHash := createBlob(t, repo, []byte{0x00, 0x01, 0x02})
	binaryDiff, err := ComputeFileDiff(repo, "", binaryHash, "image.bin", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff(binary) error = %v", err)
	}
	if !binaryDiff.IsBinary {
		t.Fatal("expected binary diff")
	}

	largeHash := createBlob(t, repo, bytes.Repeat([]byte("x"), maxBlobSize+1))
	largeDiff, err := ComputeFileDiff(repo, "", largeHash, "large.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff(large) error = %v", err)
	}
	if !largeDiff.Truncated {
		t.Fatal("expected truncated diff")
	}
}

func TestComputeFileDiff_InvalidBlobHashes(t *testing.T) {
	repo := setupTestRepo(t)
	validBlob := createBlob(t, repo, []byte("ok"))
	invalidHash := Hash(strings.Repeat("a", 40))

	diff, err := ComputeFileDiff(repo, invalidHash, validBlob, "file.txt", DefaultContextLines)
	if err == nil || diff != nil {
		t.Fatalf("ComputeFileDiff() with invalid old hash = (%v, %v), want (nil, error)", diff, err)
	}

	diff, err = ComputeFileDiff(repo, validBlob, invalidHash, "file.txt", DefaultContextLines)
	if err == nil || diff != nil {
		t.Fatalf("ComputeFileDiff() with invalid new hash = (%v, %v), want (nil, error)", diff, err)
	}
}

func TestMyersDiffHelpers(t *testing.T) {
	if got := myersDiff([]string{"same"}, []string{"same"}, DefaultContextLines); len(got) != 0 {
		t.Fatalf("myersDiff(no changes) len = %d, want 0", len(got))
	}
	if got := myersDiff(nil, nil, DefaultContextLines); len(got) != 0 {
		t.Fatalf("myersDiff(empty) len = %d, want 0", len(got))
	}
	if got := computeEdits(nil, nil); len(got) != 0 {
		t.Fatalf("computeEdits(empty) len = %d, want 0", len(got))
	}
	if got := buildHunks([]string{"a"}, []string{"b"}, nil, 1); len(got) != 0 {
		t.Fatalf("buildHunks(empty edits) len = %d, want 0", len(got))
	}

	hunks := myersDiff([]string{"a", "b"}, []string{"a", "c"}, 1)
	if len(hunks) != 1 {
		t.Fatalf("myersDiff(change) len = %d, want 1", len(hunks))
	}
	if got := splitLines([]byte("a\nb\n")); len(got) != 2 || got[1] != "b" {
		t.Fatalf("splitLines() = %#v, want [a b]", got)
	}
	if !IsBinaryContent([]byte{0x00, 'a'}) {
		t.Fatal("IsBinaryContent() = false, want true")
	}
	if IsBinaryContent(append(bytes.Repeat([]byte{'a'}, 8192), 0)) {
		t.Fatal("IsBinaryContent() = true, want false for null byte beyond 8KB")
	}
	if IsBinaryContent([]byte("plain text")) {
		t.Fatal("IsBinaryContent() = true, want false")
	}

	if DiffStatusRenamed.String() != "renamed" {
		t.Fatalf("DiffStatusRenamed.String() = %q, want %q", DiffStatusRenamed.String(), "renamed")
	}
	if DiffStatus(-1).String() != "unknown" {
		t.Fatalf("DiffStatusRenamed.String() = %q, want %q", DiffStatus(-1).String(), "unknown")
	}

	if LineTypeAddition.String() != "addition" {
		t.Fatalf("LineTypeRenamed.String() = %q, want %q", LineTypeAddition.String(), "addition")
	}
	if LineType(-1).String() != "unknown" {
		t.Fatalf("LineTypeRenamed.String() = %q, want %q", LineType(-1).String(), "unknown")
	}
}

func TestBacktrackTrailingEdits(t *testing.T) {
	deletes := backtrack([]string{"a", "b"}, nil, nil, 0, 2)
	if len(deletes) != 2 {
		t.Fatalf("len(backtrack deletes) = %d, want 2", len(deletes))
	}
	if deletes[0].Type != editDelete || deletes[1].Type != editDelete {
		t.Fatalf("backtrack deletes = %#v, want only deletes", deletes)
	}

	inserts := backtrack(nil, []string{"a", "b"}, nil, 0, 2)
	if len(inserts) != 2 {
		t.Fatalf("len(backtrack inserts) = %d, want 2", len(inserts))
	}
	if inserts[0].Type != editInsert || inserts[1].Type != editInsert {
		t.Fatalf("backtrack inserts = %#v, want only inserts", inserts)
	}
}

func TestBacktrackPrevYBelowZero(t *testing.T) {
	trace := [][]int{
		{0, 0, 0, -1, 0, 0, 0, 0, 0},
	}

	edits := backtrack([]string{"a", "b"}, []string{"x", "b"}, trace, 1, 4)
	if len(edits) == 0 {
		t.Fatal("backtrack() returned no edits")
	}
}

func TestComputeEdits_PreservesSharedSuffix(t *testing.T) {
	edits := computeEdits([]string{"old", "shared"}, []string{"new", "shared"})
	if len(edits) == 0 {
		t.Fatal("computeEdits() returned no edits")
	}

	foundKeep := false
	for _, edit := range edits {
		if edit.Type == editKeep && edit.OldLine == 1 && edit.NewLine == 1 {
			foundKeep = true
			break
		}
	}
	if !foundKeep {
		t.Fatalf("computeEdits() = %#v, want keep edit for shared suffix", edits)
	}
}

func TestBuildHunksSplitsOnDistantContext(t *testing.T) {
	oldLines := []string{"first", "middle", "tail"}
	newLines := []string{"middle", "tail"}
	edits := []edit{
		{Type: editDelete, OldLine: 0},
		{Type: editKeep, OldLine: 1, NewLine: 0},
		{Type: editKeep, OldLine: 2, NewLine: 1},
	}

	hunks := buildHunks(oldLines, newLines, edits, 1)
	if len(hunks) != 1 {
		t.Fatalf("len(buildHunks()) = %d, want 1", len(hunks))
	}
	if len(hunks[0].Lines) != 2 {
		t.Fatalf("len(hunks[0].Lines) = %d, want 2", len(hunks[0].Lines))
	}
	if hunks[0].Lines[0].Type != LineTypeDeletion || hunks[0].Lines[1].Type != LineTypeContext {
		t.Fatalf("hunk lines = %#v, want deletion followed by context", hunks[0].Lines)
	}
}

func TestDetectRenamesEdgeCases(t *testing.T) {
	hashA := Hash(strings.Repeat("a", 40))
	hashB := Hash(strings.Repeat("b", 40))
	entries := []DiffEntry{
		{Path: "old_a.go", Status: DiffStatusDeleted, OldHash: hashA, OldMode: "100644"},
		{Path: "old_b.go", Status: DiffStatusDeleted, OldHash: hashB, OldMode: "100755"},
		{Path: "new_a.go", Status: DiffStatusAdded, NewHash: hashA},
		{Path: "new_b.go", Status: DiffStatusAdded, NewHash: hashB},
	}

	got := detectRenames(entries)
	if len(got) != 2 {
		t.Fatalf("len(detectRenames()) = %d, want 2", len(got))
	}
	for _, entry := range got {
		if entry.Status != DiffStatusRenamed {
			t.Fatalf("entry.Status = %v, want renamed", entry.Status)
		}
		if entry.Path == "new_b.go" && entry.OldMode != "100755" {
			t.Fatalf("entry.OldMode = %q, want 100755", entry.OldMode)
		}
	}
}
