package gitcore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wireHeadCommit(repo *Repository, treeHash Hash) {
	headHash := Hash(strings.Repeat("a", 40))
	commit := &Commit{ID: headHash, Tree: treeHash}
	repo.head = headHash
	repo.commits = []*Commit{commit}
	repo.commitMap[headHash] = commit
}

func writeDiskFile(t *testing.T, repo *Repository, relPath string, content []byte) {
	t.Helper()
	path := filepath.Join(repo.workDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", relPath, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", relPath, err)
	}
}

func removeDiskFile(t *testing.T, repo *Repository, relPath string) {
	t.Helper()
	if err := os.Remove(filepath.Join(repo.workDir, filepath.FromSlash(relPath))); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Remove(%s): %v", relPath, err)
	}
}

func TestResolveBlobAtPath(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("hello"))
	child := createTree(t, repo, []TreeEntry{{ID: blob, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob}})
	root := createTree(t, repo, []TreeEntry{{ID: child, Name: "nested", Mode: "040000", Type: ObjectTypeTree}})

	got, err := resolveBlobAtPath(repo, root, "nested/file.txt")
	if err != nil {
		t.Fatalf("resolveBlobAtPath() error = %v", err)
	}
	if got != blob {
		t.Fatalf("resolveBlobAtPath() = %s, want %s", got, blob)
	}

	_, err = resolveBlobAtPath(repo, root, "nested/missing.txt")
	if !errors.Is(err, errBlobNotFound) {
		t.Fatalf("resolveBlobAtPath(missing) error = %v, want errBlobNotFound", err)
	}

	_, err = resolveBlobAtPath(repo, root, "../nested/file.txt")
	if !errors.Is(err, errInvalidWorktreePath) {
		t.Fatalf("resolveBlobAtPath(traversal) error = %v, want errInvalidWorktreePath", err)
	}
}

func TestComputeWorkingTreeFileDiff_ModifiedNewDeletedAndTruncated(t *testing.T) {
	repo := setupTestRepo(t)
	oldBlob := createBlob(t, repo, []byte("line 1\nline 2\n"))
	root := createTree(t, repo, []TreeEntry{{ID: oldBlob, Name: "main.go", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeDiskFile(t, repo, "main.go", []byte("line 1\nmodified line 2\n"))
	diff, err := ComputeWorkingTreeFileDiff(repo, "main.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(modified) error = %v", err)
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected modified file hunks")
	}

	writeDiskFile(t, repo, "new.go", []byte("new file\n"))
	diff, err = ComputeWorkingTreeFileDiff(repo, "new.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(new) error = %v", err)
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected new file hunks")
	}

	removeDiskFile(t, repo, "main.go")
	diff, err = ComputeWorkingTreeFileDiff(repo, "main.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(deleted) error = %v", err)
	}
	foundDeletion := false
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == LineTypeDeletion {
				foundDeletion = true
			}
		}
	}
	if !foundDeletion {
		t.Fatal("expected deletion lines")
	}

	bigBlob := createBlob(t, repo, bytes.Repeat([]byte("x"), maxBlobSize+1))
	bigTree := createTree(t, repo, []TreeEntry{{ID: bigBlob, Name: "big.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, bigTree)
	diff, err = ComputeWorkingTreeFileDiff(repo, "big.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(truncated) error = %v", err)
	}
	if !diff.Truncated {
		t.Fatal("expected truncated diff")
	}
}

func TestComputeWorkingTreeFileDiff_BinaryAndEmptyHead(t *testing.T) {
	repo := setupTestRepo(t)
	writeDiskFile(t, repo, "hello.go", []byte("package main\n"))

	diff, err := ComputeWorkingTreeFileDiff(repo, "hello.go", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(empty head) error = %v", err)
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected additions when HEAD is empty")
	}

	binBlob := createBlob(t, repo, []byte{0x00, 0x01, 0x02})
	tree := createTree(t, repo, []TreeEntry{{ID: binBlob, Name: "image.png", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, tree)
	writeDiskFile(t, repo, "image.png", []byte{0x00, 0x03, 0x04})

	diff, err = ComputeWorkingTreeFileDiff(repo, "image.png", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(binary) error = %v", err)
	}
	if !diff.IsBinary {
		t.Fatal("expected binary diff")
	}
}

func TestComputeWorkingTreeFileDiff_NormalizesRelativePath(t *testing.T) {
	repo := setupTestRepo(t)
	oldBlob := createBlob(t, repo, []byte("line 1\n"))
	child := createTree(t, repo, []TreeEntry{{ID: oldBlob, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob}})
	root := createTree(t, repo, []TreeEntry{{ID: child, Name: "nested", Mode: "040000", Type: ObjectTypeTree}})
	wireHeadCommit(repo, root)

	writeDiskFile(t, repo, "nested/file.txt", []byte("line 1\nline 2\n"))

	diff, err := ComputeWorkingTreeFileDiff(repo, "./nested//file.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(normalized) error = %v", err)
	}
	if diff.Path != "nested/file.txt" {
		t.Fatalf("ComputeWorkingTreeFileDiff(normalized) path = %q, want %q", diff.Path, "nested/file.txt")
	}
	if len(diff.Hunks) == 0 {
		t.Fatal("expected diff hunks for normalized path")
	}
}

func TestComputeWorkingTreeFileDiff_RejectsEscapingPaths(t *testing.T) {
	repo := setupTestRepo(t)

	outsidePath := filepath.Join(filepath.Dir(repo.workDir), "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside): %v", err)
	}

	tests := []string{
		"../outside.txt",
		"nested/../../outside.txt",
		outsidePath,
	}

	for _, filePath := range tests {
		t.Run(filePath, func(t *testing.T) {
			diff, err := ComputeWorkingTreeFileDiff(repo, filePath, DefaultContextLines)
			if !errors.Is(err, errInvalidWorktreePath) {
				t.Fatalf("ComputeWorkingTreeFileDiff(%q) error = %v, want errInvalidWorktreePath", filePath, err)
			}
			if diff != nil {
				t.Fatalf("ComputeWorkingTreeFileDiff(%q) diff = %#v, want nil", filePath, diff)
			}
		})
	}
}

func TestComputeWorkingTreeFileDiff_SubmoduleDoesNotReadGitlinkAsBlob(t *testing.T) {
	repo := setupTestRepo(t)
	submoduleCommit := Hash(strings.Repeat("b", 40))
	root := createTree(t, repo, []TreeEntry{{ID: submoduleCommit, Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})
	wireHeadCommit(repo, root)

	if err := os.Mkdir(filepath.Join(repo.workDir, "mod"), 0o755); err != nil {
		t.Fatalf("Mkdir(mod): %v", err)
	}

	diff, err := ComputeWorkingTreeFileDiff(repo, "mod", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeFileDiff(submodule) error = %v", err)
	}
	if diff == nil {
		t.Fatal("ComputeWorkingTreeFileDiff(submodule) = nil")
	}
}
