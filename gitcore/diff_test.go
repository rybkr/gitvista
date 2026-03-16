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
	"time"
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

func TestMyersDiffHelpers(t *testing.T) {
	if got := myersDiff([]string{"same"}, []string{"same"}, DefaultContextLines); len(got) != 0 {
		t.Fatalf("myersDiff(no changes) len = %d, want 0", len(got))
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
	if IsBinaryContent([]byte("plain text")) {
		t.Fatal("IsBinaryContent() = true, want false")
	}
	if DiffStatusRenamed.String() != StatusRenamed {
		t.Fatalf("DiffStatusRenamed.String() = %q, want %q", DiffStatusRenamed.String(), StatusRenamed)
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

func TestMergeBaseCommitHeapOrdering(t *testing.T) {
	now := time.Now()
	a := &Commit{Committer: Signature{When: now}}
	b := &Commit{Committer: Signature{When: now.Add(time.Hour)}}
	if !(mergeBaseCommitHeap{b, a}).Less(0, 1) {
		t.Fatal("expected newer commit to sort first")
	}
}
