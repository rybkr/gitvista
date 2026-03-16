package gitcore

import (
	"encoding/binary"
	"crypto/sha1" // #nosec G505 -- test helper
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func computeExpectedBlobHash(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	sum := sha1.Sum(append([]byte(header), content...)) // #nosec G401 -- test helper
	return fmt.Sprintf("%x", sum)
}

func TestHashBlobContentAndFlattenTree(t *testing.T) {
	content := []byte("hello world\n")
	if got := string(hashBlobContent(content)); got != computeExpectedBlobHash(content) {
		t.Fatalf("hashBlobContent() = %q, want %q", got, computeExpectedBlobHash(content))
	}

	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("nested"))
	inner := createTree(t, repo, []TreeEntry{{ID: blob, Name: "deep.txt", Mode: "100644", Type: ObjectTypeBlob}})
	root := createTree(t, repo, []TreeEntry{{ID: inner, Name: "dir", Mode: "040000", Type: ObjectTypeTree}})

	got, err := flattenTree(repo, root, "")
	if err != nil {
		t.Fatalf("flattenTree() error = %v", err)
	}
	if got["dir/deep.txt"] != blob {
		t.Fatalf("flattenTree()[dir/deep.txt] = %s, want %s", got["dir/deep.txt"], blob)
	}
}

type indexEntrySpec struct {
	path     string
	blobHash Hash
	fileSize uint32
}

func writeIndexWithEntries(t *testing.T, gitDir string, entries []indexEntrySpec) {
	t.Helper()
	data := buildIndexHeader(uint32(len(entries)))
	for _, entry := range entries {
		hash := hashFromHex(string(entry.blobHash))
		start := len(data)
		data = append(data, buildIndexEntryWithStats(entry.path, hash, 0o100644, 0, 1, 0, 1, 0)...)
		binary.BigEndian.PutUint32(data[start+36:start+40], entry.fileSize)
	}
	writeIndexFile(t, gitDir, data)
}

func statusByPath(t *testing.T, status *WorkingTreeStatus) map[string]FileStatus {
	t.Helper()
	m := make(map[string]FileStatus, len(status.Files))
	for _, file := range status.Files {
		m[file.Path] = file
	}
	return m
}

func TestComputeWorkingTreeStatus_BasicScenarios(t *testing.T) {
	repo := setupTestRepo(t)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus(empty) error = %v", err)
	}
	if len(status.Files) != 0 {
		t.Fatalf("len(status.Files) = %d, want 0", len(status.Files))
	}

	headBlob := createBlob(t, repo, []byte("old\n"))
	root := createTree(t, repo, []TreeEntry{{ID: headBlob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: headBlob, fileSize: uint32(len("old\n"))},
		{path: "staged.txt", blobHash: createBlob(t, repo, []byte("staged\n")), fileSize: uint32(len("staged\n"))},
	})
	writeDiskFile(t, repo, "tracked.txt", []byte("modified\n"))
	writeDiskFile(t, repo, "untracked.txt", []byte("new\n"))

	status, err = ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["staged.txt"].IndexStatus != StatusAdded {
		t.Fatalf("staged.txt IndexStatus = %q, want added", got["staged.txt"].IndexStatus)
	}
	if got["tracked.txt"].WorkStatus != StatusModified {
		t.Fatalf("tracked.txt WorkStatus = %q, want modified", got["tracked.txt"].WorkStatus)
	}
	if !got["untracked.txt"].IsUntracked {
		t.Fatal("untracked.txt should be untracked")
	}
}

func TestComputeWorkingTreeStatus_StagedDeletionAndNoChanges(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("same\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "gone.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, nil)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["gone.txt"].IndexStatus != StatusDeleted {
		t.Fatalf("gone.txt IndexStatus = %q, want deleted", got["gone.txt"].IndexStatus)
	}

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{{path: "gone.txt", blobHash: blob, fileSize: uint32(len("same\n"))}})
	writeDiskFile(t, repo, "gone.txt", []byte("same\n"))

	status, err = ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus(no changes) error = %v", err)
	}
	if len(status.Files) != 0 {
		paths := make([]string, 0, len(status.Files))
		for _, file := range status.Files {
			paths = append(paths, file.Path)
		}
		sort.Strings(paths)
		t.Fatalf("expected no changes, got %v", paths)
	}
}

func TestComputeWorkingTreeStatus_GitignoreExcludedUntracked(t *testing.T) {
	repo := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo.workDir, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}
	writeDiskFile(t, repo, "ignored.log", []byte("ignore\n"))
	writeDiskFile(t, repo, "visible.txt", []byte("keep\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if _, ok := got["ignored.log"]; ok {
		t.Fatal("ignored.log should not appear in status")
	}
	if !got["visible.txt"].IsUntracked {
		t.Fatal("visible.txt should be untracked")
	}
}
