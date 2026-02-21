package gitcore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers specific to worktree diff tests
// ---------------------------------------------------------------------------

// wireHeadCommit creates a synthetic Commit in-memory (no loose object written)
// and sets repo.head + repo.commits so that repo.Head() and repo.Commits()
// return consistent data. The tree hash must already exist in the object store.
func wireHeadCommit(repo *Repository, treeHash Hash) {
	commit := &Commit{
		ID:      Hash("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		Tree:    treeHash,
		Parents: []Hash{},
		Message: "test commit",
	}
	repo.head = commit.ID
	repo.commits = append(repo.commits, commit)
}

// writeDiskFile writes content to a file under the repo's working directory,
// creating any parent directories as needed.
func writeDiskFile(t *testing.T, repo *Repository, relPath string, content []byte) {
	t.Helper()
	fullPath := filepath.Join(repo.workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("writeDiskFile: mkdir %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		t.Fatalf("writeDiskFile: %v", err)
	}
}

// removeDiskFile deletes a file from the repo's working directory.
func removeDiskFile(t *testing.T, repo *Repository, relPath string) {
	t.Helper()
	if err := os.Remove(filepath.Join(repo.workDir, relPath)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("removeDiskFile: %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolveBlobAtPath tests
// ---------------------------------------------------------------------------

func TestResolveBlobAtPath_RootFile(t *testing.T) {
	repo := setupTestRepo(t)

	blobHash := createBlob(t, repo, []byte("hello"))
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	got, err := resolveBlobAtPath(repo, treeHash, "file.txt")
	if err != nil {
		t.Fatalf("resolveBlobAtPath failed: %v", err)
	}
	if got != blobHash {
		t.Errorf("got hash %s, want %s", got, blobHash)
	}
}

func TestResolveBlobAtPath_NestedFile(t *testing.T) {
	repo := setupTestRepo(t)

	blobHash := createBlob(t, repo, []byte("nested content"))
	innerTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "inner.go", Mode: "100644", Type: "blob"},
	})
	rootTree := createTree(t, repo, []TreeEntry{
		{ID: innerTree, Name: "pkg", Mode: "040000", Type: "tree"},
	})

	got, err := resolveBlobAtPath(repo, rootTree, "pkg/inner.go")
	if err != nil {
		t.Fatalf("resolveBlobAtPath failed: %v", err)
	}
	if got != blobHash {
		t.Errorf("got hash %s, want %s", got, blobHash)
	}
}

func TestResolveBlobAtPath_DeeplyNested(t *testing.T) {
	repo := setupTestRepo(t)

	blobHash := createBlob(t, repo, []byte("deep"))
	deepTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "deep.go", Mode: "100644", Type: "blob"},
	})
	midTree := createTree(t, repo, []TreeEntry{
		{ID: deepTree, Name: "gitcore", Mode: "040000", Type: "tree"},
	})
	rootTree := createTree(t, repo, []TreeEntry{
		{ID: midTree, Name: "internal", Mode: "040000", Type: "tree"},
	})

	got, err := resolveBlobAtPath(repo, rootTree, "internal/gitcore/deep.go")
	if err != nil {
		t.Fatalf("resolveBlobAtPath failed: %v", err)
	}
	if got != blobHash {
		t.Errorf("got hash %s, want %s", got, blobHash)
	}
}

func TestResolveBlobAtPath_FileNotInTree(t *testing.T) {
	repo := setupTestRepo(t)

	treeHash := createTree(t, repo, []TreeEntry{})

	_, err := resolveBlobAtPath(repo, treeHash, "nonexistent.txt")
	if err == nil {
		t.Fatal("expected errBlobNotFound, got nil")
	}
}

func TestResolveBlobAtPath_DirectoryComponentMissing(t *testing.T) {
	repo := setupTestRepo(t)

	blobHash := createBlob(t, repo, []byte("data"))
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	// "missing/file.txt" — the "missing" directory does not exist.
	_, err := resolveBlobAtPath(repo, treeHash, "missing/file.txt")
	if err == nil {
		t.Fatal("expected errBlobNotFound, got nil")
	}
}

func TestResolveBlobAtPath_PathPointsToTree(t *testing.T) {
	repo := setupTestRepo(t)

	blobHash := createBlob(t, repo, []byte("data"))
	innerTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "file.go", Mode: "100644", Type: "blob"},
	})
	rootTree := createTree(t, repo, []TreeEntry{
		{ID: innerTree, Name: "pkg", Mode: "040000", Type: "tree"},
	})

	// "pkg" refers to a tree, not a blob.
	_, err := resolveBlobAtPath(repo, rootTree, "pkg")
	if err == nil {
		t.Fatal("expected error when path resolves to a tree, got nil")
	}
}

func TestResolveBlobAtPath_EmptyPath(t *testing.T) {
	repo := setupTestRepo(t)
	treeHash := createTree(t, repo, []TreeEntry{})

	_, err := resolveBlobAtPath(repo, treeHash, "")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

// ---------------------------------------------------------------------------
// ComputeWorkingTreeFileDiff tests
// ---------------------------------------------------------------------------

// TestComputeWorkingTreeFileDiff_ModifiedFile verifies that when both HEAD and
// disk versions exist, the diff is computed between them.
func TestComputeWorkingTreeFileDiff_ModifiedFile(t *testing.T) {
	repo := setupTestRepo(t)

	headContent := []byte("line 1\nline 2\nline 3\n")
	blobHash := createBlob(t, repo, headContent)
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "main.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, treeHash)

	diskContent := []byte("line 1\nmodified line 2\nline 3\n")
	writeDiskFile(t, repo, "main.go", diskContent)

	diff, err := ComputeWorkingTreeFileDiff(repo, "main.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if diff.Path != "main.go" {
		t.Errorf("path = %q, want %q", diff.Path, "main.go")
	}
	if diff.IsBinary {
		t.Error("expected IsBinary=false")
	}
	if diff.Truncated {
		t.Error("expected Truncated=false")
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Verify the diff contains a deletion of "line 2" and an addition of "modified line 2".
	var hasDeletion, hasAddition bool
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == LineTypeDeletion && strings.Contains(line.Content, "line 2") {
				hasDeletion = true
			}
			if line.Type == LineTypeAddition && strings.Contains(line.Content, "modified line 2") {
				hasAddition = true
			}
		}
	}
	if !hasDeletion {
		t.Error("expected deletion of original 'line 2'")
	}
	if !hasAddition {
		t.Error("expected addition of 'modified line 2'")
	}
}

// TestComputeWorkingTreeFileDiff_NewFile verifies that when the file is not in
// HEAD (untracked), all on-disk lines appear as additions.
func TestComputeWorkingTreeFileDiff_NewFile(t *testing.T) {
	repo := setupTestRepo(t)

	// HEAD tree exists but does not contain the file.
	treeHash := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, treeHash)

	diskContent := []byte("new line 1\nnew line 2\n")
	writeDiskFile(t, repo, "newfile.go", diskContent)

	diff, err := ComputeWorkingTreeFileDiff(repo, "newfile.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if len(diff.Hunks) == 0 {
		t.Fatal("expected hunks for new file")
	}

	additions := 0
	deletions := 0
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case LineTypeAddition:
				additions++
			case LineTypeDeletion:
				deletions++
			}
		}
	}

	if additions != 2 {
		t.Errorf("expected 2 additions for new file, got %d", additions)
	}
	if deletions != 0 {
		t.Errorf("expected 0 deletions for new file, got %d", deletions)
	}
	// OldHash must be empty — there is no HEAD version.
	if diff.OldHash != "" {
		t.Errorf("expected empty OldHash for new file, got %s", diff.OldHash)
	}
}

// TestComputeWorkingTreeFileDiff_DeletedFile verifies that when the file is in
// HEAD but absent on disk, all HEAD lines appear as deletions.
func TestComputeWorkingTreeFileDiff_DeletedFile(t *testing.T) {
	repo := setupTestRepo(t)

	headContent := []byte("deleted line 1\ndeleted line 2\ndeleted line 3\n")
	blobHash := createBlob(t, repo, headContent)
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "gone.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, treeHash)

	// Deliberately do NOT write the file to disk (it has been deleted).

	diff, err := ComputeWorkingTreeFileDiff(repo, "gone.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if len(diff.Hunks) == 0 {
		t.Fatal("expected hunks for deleted file")
	}

	deletions := 0
	additions := 0
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case LineTypeDeletion:
				deletions++
			case LineTypeAddition:
				additions++
			}
		}
	}

	if deletions != 3 {
		t.Errorf("expected 3 deletions for deleted file, got %d", deletions)
	}
	if additions != 0 {
		t.Errorf("expected 0 additions for deleted file, got %d", additions)
	}
	if diff.OldHash != blobHash {
		t.Errorf("OldHash = %s, want %s", diff.OldHash, blobHash)
	}
}

// TestComputeWorkingTreeFileDiff_IdenticalContent verifies that when HEAD and
// on-disk content are identical, no hunks are produced.
func TestComputeWorkingTreeFileDiff_IdenticalContent(t *testing.T) {
	repo := setupTestRepo(t)

	content := []byte("same content\non both sides\n")
	blobHash := createBlob(t, repo, content)
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "same.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, treeHash)

	// On-disk content is byte-for-byte identical to the HEAD blob.
	writeDiskFile(t, repo, "same.go", content)

	diff, err := ComputeWorkingTreeFileDiff(repo, "same.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if len(diff.Hunks) != 0 {
		t.Errorf("expected 0 hunks for identical content, got %d", len(diff.Hunks))
	}
}

// TestComputeWorkingTreeFileDiff_BinaryFile verifies that binary content is
// detected and returned with IsBinary=true and no hunks.
func TestComputeWorkingTreeFileDiff_BinaryFile(t *testing.T) {
	repo := setupTestRepo(t)

	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF}
	blobHash := createBlob(t, repo, binaryContent)
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "image.png", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, treeHash)

	// On-disk version is slightly different binary content.
	writeDiskFile(t, repo, "image.png", []byte{0x00, 0x01, 0x03, 0xFE})

	diff, err := ComputeWorkingTreeFileDiff(repo, "image.png", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if !diff.IsBinary {
		t.Error("expected IsBinary=true for binary content")
	}
	if len(diff.Hunks) != 0 {
		t.Errorf("expected 0 hunks for binary file, got %d", len(diff.Hunks))
	}
}

// TestComputeWorkingTreeFileDiff_TruncatedFile verifies that files exceeding
// maxBlobSize return Truncated=true and no hunks.
func TestComputeWorkingTreeFileDiff_TruncatedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// HEAD version is small text.
	smallContent := []byte("small head content\n")
	blobHash := createBlob(t, repo, smallContent)
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "big.txt", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, treeHash)

	// On-disk version is large (>512KB).
	largeContent := bytes.Repeat([]byte("x"), maxBlobSize+1)
	writeDiskFile(t, repo, "big.txt", largeContent)

	diff, err := ComputeWorkingTreeFileDiff(repo, "big.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if !diff.Truncated {
		t.Error("expected Truncated=true for oversized file")
	}
	if len(diff.Hunks) != 0 {
		t.Errorf("expected 0 hunks for truncated file, got %d", len(diff.Hunks))
	}
}

// TestComputeWorkingTreeFileDiff_EmptyHead verifies that when the repository
// has no HEAD commit (empty repo), all on-disk lines are treated as additions.
func TestComputeWorkingTreeFileDiff_EmptyHead(t *testing.T) {
	repo := setupTestRepo(t)
	// repo.head is "" by default — no commits loaded.

	writeDiskFile(t, repo, "hello.go", []byte("package main\n"))

	diff, err := ComputeWorkingTreeFileDiff(repo, "hello.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	// With no HEAD, disk content is fully new — all lines are additions.
	additions := 0
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == LineTypeAddition {
				additions++
			}
		}
	}
	if additions != 1 {
		t.Errorf("expected 1 addition for empty-HEAD repo, got %d", additions)
	}
}

// TestComputeWorkingTreeFileDiff_BothAbsent verifies the degenerate case where
// neither side exists: the result should have no hunks and no errors.
func TestComputeWorkingTreeFileDiff_BothAbsent(t *testing.T) {
	repo := setupTestRepo(t)

	// HEAD tree exists but does not contain the file.
	treeHash := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, treeHash)

	// No disk file written — both sides absent.
	removeDiskFile(t, repo, "ghost.go") // idempotent: file doesn't exist

	diff, err := ComputeWorkingTreeFileDiff(repo, "ghost.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if len(diff.Hunks) != 0 {
		t.Errorf("expected 0 hunks when both sides absent, got %d", len(diff.Hunks))
	}
}

// TestComputeWorkingTreeFileDiff_NestedFile verifies that files in subdirectories
// are resolved correctly through the tree walk.
func TestComputeWorkingTreeFileDiff_NestedFile(t *testing.T) {
	repo := setupTestRepo(t)

	headContent := []byte("package gitcore\n")
	blobHash := createBlob(t, repo, headContent)
	innerTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "diff.go", Mode: "100644", Type: "blob"},
	})
	rootTree := createTree(t, repo, []TreeEntry{
		{ID: innerTree, Name: "gitcore", Mode: "040000", Type: "tree"},
	})
	wireHeadCommit(repo, rootTree)

	diskContent := []byte("package gitcore\n\n// changed\n")
	writeDiskFile(t, repo, "gitcore/diff.go", diskContent)

	diff, err := ComputeWorkingTreeFileDiff(repo, "gitcore/diff.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff failed: %v", err)
	}

	if diff.Path != "gitcore/diff.go" {
		t.Errorf("path = %q, want %q", diff.Path, "gitcore/diff.go")
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected hunks for modified nested file")
	}

	hasAddition := false
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == LineTypeAddition {
				hasAddition = true
			}
		}
	}
	if !hasAddition {
		t.Error("expected at least one addition line for nested file diff")
	}
}
