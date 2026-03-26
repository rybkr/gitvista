package gitcore

import (
	"crypto/sha1" // #nosec G505 -- test helper
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
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
	if got["dir/deep.txt"].Hash != blob {
		t.Fatalf("flattenTree()[dir/deep.txt].Hash = %s, want %s", got["dir/deep.txt"].Hash, blob)
	}
	if got["dir/deep.txt"].Mode != "100644" {
		t.Fatalf("flattenTree()[dir/deep.txt].Mode = %q, want %q", got["dir/deep.txt"].Mode, "100644")
	}
}

type indexEntrySpec struct {
	path     string
	blobHash Hash
	fileSize uint32
	mode     uint32
}

func writeIndexWithEntries(t *testing.T, gitDir string, entries []indexEntrySpec) {
	t.Helper()
	data := buildIndexHeader(uint32(len(entries)))
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o100644
		}
		hash := hashFromHex(string(entry.blobHash))
		start := len(data)
		data = append(data, buildIndexEntryWithStats(entry.path, hash, mode, 0, 1, 0, 1, 0)...)
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

func TestComputeWorkingTreeStatus_TrackedRegularFileReplacedBySymlink(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("tracked\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("tracked\n"))},
	})

	if err := os.Symlink(filepath.Join("..", "missing-target.txt"), filepath.Join(repo.workDir, "tracked.txt")); err != nil {
		t.Fatalf("Symlink(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["tracked.txt"].WorkStatus != StatusTypeChanged {
		t.Fatalf("tracked.txt WorkStatus = %q, want typechanged", got["tracked.txt"].WorkStatus)
	}
}

func TestComputeWorkingTreeStatus_TrackedSymlinkReplacedByRegularFile(t *testing.T) {
	repo := setupTestRepo(t)
	linkHash := Hash(computeExpectedBlobHash([]byte("target.txt")))

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "link.txt", blobHash: linkHash, fileSize: uint32(len("target.txt")), mode: 0o120000},
	})
	writeDiskFile(t, repo, "link.txt", []byte("plain file\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["link.txt"].WorkStatus != StatusTypeChanged {
		t.Fatalf("link.txt WorkStatus = %q, want typechanged", got["link.txt"].WorkStatus)
	}
}

func TestComputeWorkingTreeStatus_TrackedFileReplacedByDirectory(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("tracked\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("tracked\n"))},
	})

	if err := os.Mkdir(filepath.Join(repo.workDir, "tracked.txt"), 0o755); err != nil {
		t.Fatalf("Mkdir(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["tracked.txt"].WorkStatus != StatusTypeChanged {
		t.Fatalf("tracked.txt WorkStatus = %q, want typechanged", got["tracked.txt"].WorkStatus)
	}
}

func TestComputeWorkingTreeStatus_PropagatesWalkErrors(t *testing.T) {
	repo := setupTestRepo(t)

	originalWalk := walkWorktree
	t.Cleanup(func() {
		walkWorktree = originalWalk
	})

	walkWorktree = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "blocked"), nil, fs.ErrPermission)
	}

	if _, err := ComputeWorkingTreeStatus(repo); err == nil {
		t.Fatal("ComputeWorkingTreeStatus() error = nil, want walk error")
	}
}

func TestComputeWorkingTreeStatus_ReturnsFilesSortedByPath(t *testing.T) {
	repo := setupTestRepo(t)

	writeDiskFile(t, repo, "z-last.txt", []byte("z\n"))
	writeDiskFile(t, repo, "a-first.txt", []byte("a\n"))
	writeDiskFile(t, repo, "m-middle.txt", []byte("m\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}

	got := make([]string, 0, len(status.Files))
	for _, file := range status.Files {
		got = append(got, file.Path)
	}

	want := []string{"a-first.txt", "m-middle.txt", "z-last.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("status file order = %v, want %v", got, want)
	}
}

func TestComputeWorkingTreeStatus_StagedModeOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("#!/bin/sh\necho hi\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "script.sh", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "script.sh", blobHash: blob, fileSize: uint32(len("#!/bin/sh\necho hi\n")), mode: 0o100755},
	})
	writeDiskFile(t, repo, "script.sh", []byte("#!/bin/sh\necho hi\n"))
	if err := os.Chmod(filepath.Join(repo.workDir, "script.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(script.sh): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["script.sh"].IndexStatus != StatusModified {
		t.Fatalf("script.sh IndexStatus = %q, want %q", got["script.sh"].IndexStatus, StatusModified)
	}
}

func TestComputeWorkingTreeStatus_WorktreeModeOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("#!/bin/sh\necho hi\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "script.sh", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "script.sh", blobHash: blob, fileSize: uint32(len("#!/bin/sh\necho hi\n")), mode: 0o100644},
	})
	writeDiskFile(t, repo, "script.sh", []byte("#!/bin/sh\necho hi\n"))
	if err := os.Chmod(filepath.Join(repo.workDir, "script.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(script.sh): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["script.sh"].WorkStatus != StatusModified {
		t.Fatalf("script.sh WorkStatus = %q, want %q", got["script.sh"].WorkStatus, StatusModified)
	}
}

func TestComputeWorkingTreeStatus_StagedTypeChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("replace me\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	linkHash := Hash(computeExpectedBlobHash([]byte("target.txt")))
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: linkHash, fileSize: uint32(len("target.txt")), mode: 0o120000},
	})
	if err := os.Symlink("target.txt", filepath.Join(repo.workDir, "tracked.txt")); err != nil {
		t.Fatalf("Symlink(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["tracked.txt"].IndexStatus != StatusTypeChanged {
		t.Fatalf("tracked.txt IndexStatus = %q, want %q", got["tracked.txt"].IndexStatus, StatusTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_WorktreeTypeChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("replace me\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("replace me\n")), mode: 0o100644},
	})
	if err := os.Symlink("target.txt", filepath.Join(repo.workDir, "tracked.txt")); err != nil {
		t.Fatalf("Symlink(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["tracked.txt"].WorkStatus != StatusTypeChanged {
		t.Fatalf("tracked.txt WorkStatus = %q, want %q", got["tracked.txt"].WorkStatus, StatusTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_CollapsesUntrackedDirectories(t *testing.T) {
	repo := setupTestRepo(t)

	writeDiskFile(t, repo, "nested/untracked-a.txt", []byte("a\n"))
	writeDiskFile(t, repo, "nested/untracked-b.txt", []byte("b\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if !got["nested/"].IsUntracked {
		t.Fatalf("nested/ IsUntracked = %v, want true", got["nested/"].IsUntracked)
	}
	if _, ok := got["nested/untracked-a.txt"]; ok {
		t.Fatal("nested/untracked-a.txt should be collapsed into nested/")
	}
	if _, ok := got["nested/untracked-b.txt"]; ok {
		t.Fatal("nested/untracked-b.txt should be collapsed into nested/")
	}
}

func TestComputeWorkingTreeStatus_CleanSubmoduleIsNotReportedModified(t *testing.T) {
	repo := setupTestRepo(t)
	submoduleCommit := Hash(strings.Repeat("b", 40))
	root := createTree(t, repo, []TreeEntry{{ID: submoduleCommit, Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "mod", blobHash: submoduleCommit, fileSize: 0, mode: 0o160000},
	})
	if err := os.Mkdir(filepath.Join(repo.workDir, "mod"), 0o755); err != nil {
		t.Fatalf("Mkdir(mod): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	if len(status.Files) != 0 {
		t.Fatalf("expected clean submodule to be omitted from status, got %#v", status.Files)
	}
}
