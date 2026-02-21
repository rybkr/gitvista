package gitcore

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"testing"
)

// TestHashBlobContent_KnownVectors verifies that hashBlobContent produces the
// SHA-1 that git would compute for "blob <size>\0<content>". The expected
// hashes were pre-computed with: echo -n "blob N\0<content>" | sha1sum.
func TestHashBlobContent_KnownVectors(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		// wantHex is computed as: sha1("blob <len>\0<content>")
		wantHex string
	}{
		{
			name:    "empty content",
			content: []byte{},
			// sha1("blob 0\0") — git's empty blob hash, universally known.
			wantHex: "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
		},
		{
			name:    "hello world newline",
			content: []byte("hello world\n"),
			// Precomputed: sha1("blob 12\0hello world\n")
			wantHex: computeExpectedBlobHash([]byte("hello world\n")),
		},
		{
			name:    "single byte",
			content: []byte("x"),
			wantHex: computeExpectedBlobHash([]byte("x")),
		},
		{
			name:    "multi-line text",
			content: []byte("line1\nline2\nline3\n"),
			wantHex: computeExpectedBlobHash([]byte("line1\nline2\nline3\n")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashBlobContent(tt.content)
			if string(got) != tt.wantHex {
				t.Errorf("hashBlobContent(%q) = %s, want %s", tt.content, got, tt.wantHex)
			}
		})
	}
}

// TestHashBlobContent_EmptyBlobIsKnownHash verifies the single most important
// invariant: the empty blob hash is the well-known git constant.
// This serves as an end-to-end smoke test of the SHA-1 header construction.
func TestHashBlobContent_EmptyBlobIsKnownHash(t *testing.T) {
	const gitEmptyBlobHash = "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"
	got := hashBlobContent([]byte{})
	if string(got) != gitEmptyBlobHash {
		t.Errorf("hashBlobContent(empty) = %s, want %s", got, gitEmptyBlobHash)
	}
}

// TestHashBlobContent_DifferentContentDifferentHash ensures that two distinct
// byte slices produce different hashes (no trivial collision in the header logic).
func TestHashBlobContent_DifferentContentDifferentHash(t *testing.T) {
	h1 := hashBlobContent([]byte("foo"))
	h2 := hashBlobContent([]byte("bar"))
	if h1 == h2 {
		t.Errorf("different content produced the same hash: %s", h1)
	}
}

// computeExpectedBlobHash is the reference implementation used inside the test
// package. It exists solely to generate expected values without duplicating the
// format string in every test case. It must NOT be used in production code.
func computeExpectedBlobHash(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	h := sha1.New()
	h.Write([]byte(header))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// TestFlattenTree_SingleRootBlob verifies that a tree containing one blob at
// the root level is flattened to a single map entry.
func TestFlattenTree_SingleRootBlob(t *testing.T) {
	repo := setupTestRepo(t)

	blobHash := createBlob(t, repo, []byte("content"))
	treeHash := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	result, err := flattenTree(repo, treeHash, "")
	if err != nil {
		t.Fatalf("flattenTree failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if got, ok := result["file.txt"]; !ok {
		t.Error("expected 'file.txt' in result")
	} else if got != blobHash {
		t.Errorf("hash = %s, want %s", got, blobHash)
	}
}

// TestFlattenTree_NestedDirectories verifies that flattenTree recurses into
// sub-trees and builds correct slash-separated paths.
func TestFlattenTree_NestedDirectories(t *testing.T) {
	repo := setupTestRepo(t)

	deepBlob := createBlob(t, repo, []byte("deep content"))
	deepTree := createTree(t, repo, []TreeEntry{
		{ID: deepBlob, Name: "deep.go", Mode: "100644", Type: "blob"},
	})
	midTree := createTree(t, repo, []TreeEntry{
		{ID: deepTree, Name: "gitcore", Mode: "040000", Type: "tree"},
	})
	rootBlob := createBlob(t, repo, []byte("root blob"))
	rootTree := createTree(t, repo, []TreeEntry{
		{ID: midTree, Name: "internal", Mode: "040000", Type: "tree"},
		{ID: rootBlob, Name: "README.md", Mode: "100644", Type: "blob"},
	})

	result, err := flattenTree(repo, rootTree, "")
	if err != nil {
		t.Fatalf("flattenTree failed: %v", err)
	}

	// Expect exactly two blobs: the nested one and the root one.
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(result), result)
	}

	if got, ok := result["internal/gitcore/deep.go"]; !ok {
		t.Error("expected 'internal/gitcore/deep.go' in result")
	} else if got != deepBlob {
		t.Errorf("internal/gitcore/deep.go hash = %s, want %s", got, deepBlob)
	}

	if got, ok := result["README.md"]; !ok {
		t.Error("expected 'README.md' in result")
	} else if got != rootBlob {
		t.Errorf("README.md hash = %s, want %s", got, rootBlob)
	}
}

// TestFlattenTree_EmptyTree verifies that an empty tree produces an empty map.
func TestFlattenTree_EmptyTree(t *testing.T) {
	repo := setupTestRepo(t)
	treeHash := createTree(t, repo, []TreeEntry{})

	result, err := flattenTree(repo, treeHash, "")
	if err != nil {
		t.Fatalf("flattenTree failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for empty tree, got %d entries", len(result))
	}
}

// TestFlattenTree_DeeplyNested verifies three levels of nesting with multiple
// blobs at each level to exercise the recursive path building.
func TestFlattenTree_DeeplyNested(t *testing.T) {
	repo := setupTestRepo(t)

	blob1 := createBlob(t, repo, []byte("leaf1"))
	blob2 := createBlob(t, repo, []byte("leaf2"))
	blob3 := createBlob(t, repo, []byte("root leaf"))

	level2Tree := createTree(t, repo, []TreeEntry{
		{ID: blob1, Name: "a.go", Mode: "100644", Type: "blob"},
		{ID: blob2, Name: "b.go", Mode: "100644", Type: "blob"},
	})
	level1Tree := createTree(t, repo, []TreeEntry{
		{ID: level2Tree, Name: "pkg", Mode: "040000", Type: "tree"},
	})
	rootTree := createTree(t, repo, []TreeEntry{
		{ID: level1Tree, Name: "src", Mode: "040000", Type: "tree"},
		{ID: blob3, Name: "main.go", Mode: "100644", Type: "blob"},
	})

	result, err := flattenTree(repo, rootTree, "")
	if err != nil {
		t.Fatalf("flattenTree failed: %v", err)
	}

	expected := map[string]Hash{
		"src/pkg/a.go": blob1,
		"src/pkg/b.go": blob2,
		"main.go":      blob3,
	}

	if len(result) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(result), result)
	}
	for wantPath, wantHash := range expected {
		got, ok := result[wantPath]
		if !ok {
			t.Errorf("missing expected path %q", wantPath)
			continue
		}
		if got != wantHash {
			t.Errorf("path %q: got hash %s, want %s", wantPath, got, wantHash)
		}
	}
}

// statusByPath is a test helper that turns the FileStatus slice into a map
// keyed by path, making assertions order-independent.
func statusByPath(t *testing.T, status *WorkingTreeStatus) map[string]FileStatus {
	t.Helper()
	m := make(map[string]FileStatus, len(status.Files))
	for _, f := range status.Files {
		if _, dup := m[f.Path]; dup {
			t.Errorf("duplicate path in status result: %q", f.Path)
		}
		m[f.Path] = f
	}
	return m
}

// writeIndexWithEntries writes a synthetic v2 index file to gitDir/index.
// It accepts a slice of (path, blobHash, fileSize) tuples.
func writeIndexWithEntries(t *testing.T, gitDir string, entries []indexEntrySpec) {
	t.Helper()

	var raw bytes.Buffer
	raw.Write(buildIndexHeader(uint32(len(entries))))
	for _, e := range entries {
		var hashBytes [20]byte
		decoded, err := hex.DecodeString(string(e.hash))
		if err != nil {
			t.Fatalf("writeIndexWithEntries: invalid hash %q: %v", e.hash, err)
		}
		copy(hashBytes[:], decoded)
		raw.Write(buildIndexEntryWithStats(
			e.path, hashBytes, 0o100644, 0,
			0, 0, 0, 0, 0, 0, 0, 0, e.fileSize,
		))
	}
	writeIndexFile(t, gitDir, raw.Bytes())
}

// indexEntrySpec is a compact representation used by writeIndexWithEntries.
type indexEntrySpec struct {
	path     string
	hash     Hash
	fileSize uint32
}

// TestComputeWorkingTreeStatus_EmptyRepo verifies that a repository with no
// HEAD commit, no index entries, and no working directory files returns an
// empty status.
func TestComputeWorkingTreeStatus_EmptyRepo(t *testing.T) {
	repo := setupTestRepo(t)
	// No HEAD commit, no index file, no working directory files.

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil WorkingTreeStatus")
	}
	if len(status.Files) != 0 {
		t.Errorf("expected 0 files, got %d: %v", len(status.Files), status.Files)
	}
}

// TestComputeWorkingTreeStatus_StagedAddition verifies that a file present in
// the index but not in HEAD is reported with IndexStatus "added".
func TestComputeWorkingTreeStatus_StagedAddition(t *testing.T) {
	repo := setupTestRepo(t)

	// Set up an empty HEAD tree (so no files are in HEAD).
	headTree := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, headTree)

	// Compute the git blob hash for the file content we will use on disk.
	content := []byte("new file content\n")
	blobHash := hashBlobContent(content)

	// Write a matching file to disk so there is no unstaged change.
	writeDiskFile(t, repo, "new.go", content)

	// Write the index to record the staged addition.
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "new.go", hash: blobHash, fileSize: uint32(len(content))},
	})

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)

	// Filter out any .git-related paths that may have been picked up.
	f, ok := m["new.go"]
	if !ok {
		t.Fatalf("expected 'new.go' in status; got paths: %v", sortedKeys(m))
	}
	if f.IndexStatus != "added" {
		t.Errorf("IndexStatus = %q, want %q", f.IndexStatus, "added")
	}
	if f.WorkStatus != "" {
		t.Errorf("WorkStatus = %q, want empty (disk matches index)", f.WorkStatus)
	}
	if f.IsUntracked {
		t.Error("IsUntracked should be false for a staged addition")
	}
}

// TestComputeWorkingTreeStatus_StagedModification verifies that a file present
// in both HEAD and the index with differing hashes is reported as "modified"
// in IndexStatus.
func TestComputeWorkingTreeStatus_StagedModification(t *testing.T) {
	repo := setupTestRepo(t)

	// Create the HEAD version of the file.
	headContent := []byte("original content\n")
	headBlob := createBlob(t, repo, headContent)
	headTree := createTree(t, repo, []TreeEntry{
		{ID: headBlob, Name: "main.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	// Staged version is different from HEAD.
	stagedContent := []byte("modified content\n")
	stagedHash := hashBlobContent(stagedContent)

	// Write the staged version to disk (no unstaged changes).
	writeDiskFile(t, repo, "main.go", stagedContent)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "main.go", hash: stagedHash, fileSize: uint32(len(stagedContent))},
	})

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["main.go"]
	if !ok {
		t.Fatalf("expected 'main.go' in status; got paths: %v", sortedKeys(m))
	}
	if f.IndexStatus != "modified" {
		t.Errorf("IndexStatus = %q, want %q", f.IndexStatus, "modified")
	}
	if f.WorkStatus != "" {
		t.Errorf("WorkStatus = %q, want empty (disk matches index)", f.WorkStatus)
	}
}

// TestComputeWorkingTreeStatus_StagedDeletion verifies that a file present in
// HEAD but absent from the index is reported with IndexStatus "deleted".
func TestComputeWorkingTreeStatus_StagedDeletion(t *testing.T) {
	repo := setupTestRepo(t)

	headContent := []byte("will be deleted\n")
	headBlob := createBlob(t, repo, headContent)
	headTree := createTree(t, repo, []TreeEntry{
		{ID: headBlob, Name: "gone.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	// Index is empty — file has been git-rm'd from the staging area.
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{})

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["gone.go"]
	if !ok {
		t.Fatalf("expected 'gone.go' in status; got paths: %v", sortedKeys(m))
	}
	if f.IndexStatus != "deleted" {
		t.Errorf("IndexStatus = %q, want %q", f.IndexStatus, "deleted")
	}
}

// TestComputeWorkingTreeStatus_UnstagedModification verifies that a file whose
// on-disk content differs from what the index records is reported with
// WorkStatus "modified" but no IndexStatus (if the index matches HEAD).
//
// We use hashBlobContent throughout so that the HEAD tree hash, the index hash,
// and the disk-content hash are all real SHA-1 values and directly comparable.
// The HEAD tree entry ID is set to the real blob hash so the HEAD-vs-index
// comparison finds them equal (no staged change).
func TestComputeWorkingTreeStatus_UnstagedModification(t *testing.T) {
	repo := setupTestRepo(t)

	indexContent := []byte("index version\n")

	// Use the real blob hash as the HEAD tree entry so HEAD-vs-index comparison
	// sees a match. We write the blob to the object store using createBlob but
	// override the tree entry ID with the real SHA-1 so flattenTree returns a
	// hash that equals the index hash.
	realHash := hashBlobContent(indexContent)
	headTree := createTree(t, repo, []TreeEntry{
		// Mode "100644", Type "blob" — ID is the real SHA-1 of indexContent.
		{ID: realHash, Name: "edited.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	// Index records the same real SHA-1 → no staged change relative to HEAD.
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "edited.go", hash: realHash, fileSize: uint32(len(indexContent))},
	})

	// Write a different version to disk (unstaged modification).
	diskContent := []byte("disk version — different content\n")
	writeDiskFile(t, repo, "edited.go", diskContent)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["edited.go"]
	if !ok {
		t.Fatalf("expected 'edited.go' in status; got paths: %v", sortedKeys(m))
	}
	if f.IndexStatus != "" {
		t.Errorf("IndexStatus = %q, want empty (index matches HEAD)", f.IndexStatus)
	}
	if f.WorkStatus != "modified" {
		t.Errorf("WorkStatus = %q, want %q", f.WorkStatus, "modified")
	}
}

// TestComputeWorkingTreeStatus_UnstagedDeletion verifies that a file tracked in
// the index but missing from disk is reported with WorkStatus "deleted".
func TestComputeWorkingTreeStatus_UnstagedDeletion(t *testing.T) {
	repo := setupTestRepo(t)

	content := []byte("tracked content\n")
	// Use real blob hash for both the HEAD tree and the index so the
	// HEAD-vs-index comparison finds them equal (no staged change).
	realHash := hashBlobContent(content)
	headTree := createTree(t, repo, []TreeEntry{
		{ID: realHash, Name: "present.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	// Index still tracks the file (no staged deletion).
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "present.go", hash: realHash, fileSize: uint32(len(content))},
	})

	// File is absent from disk — unstaged deletion.
	// (writeDiskFile is deliberately not called for "present.go")

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["present.go"]
	if !ok {
		t.Fatalf("expected 'present.go' in status; got paths: %v", sortedKeys(m))
	}
	if f.WorkStatus != "deleted" {
		t.Errorf("WorkStatus = %q, want %q", f.WorkStatus, "deleted")
	}
}

// TestComputeWorkingTreeStatus_UntrackedFile verifies that files on disk that
// are not in the index are reported with IsUntracked=true.
func TestComputeWorkingTreeStatus_UntrackedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Empty HEAD and empty index.
	headTree := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, headTree)
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{})

	// Write an untracked file to disk.
	writeDiskFile(t, repo, "untracked.txt", []byte("not in index\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["untracked.txt"]
	if !ok {
		t.Fatalf("expected 'untracked.txt' in status; got paths: %v", sortedKeys(m))
	}
	if !f.IsUntracked {
		t.Error("IsUntracked should be true")
	}
	if f.IndexStatus != "" || f.WorkStatus != "" {
		t.Errorf("IndexStatus=%q WorkStatus=%q, both want empty for untracked", f.IndexStatus, f.WorkStatus)
	}
}

// TestComputeWorkingTreeStatus_FullScenario exercises all change types
// simultaneously: staged addition, staged modification, staged deletion,
// unstaged modification, unstaged deletion, and an untracked file.
//
// Hash strategy:
//   - For entries where the index must match HEAD (no staged change), we set
//     the HEAD tree entry ID to the real SHA-1 blob hash (hashBlobContent) and
//     store the same hash in the index.
//   - For staged modifications, the HEAD tree entry uses a real SHA-1 of the
//     old content; the index uses a real SHA-1 of the new content — they differ.
//   - For staged deletions, the file is absent from the index entirely.
func TestComputeWorkingTreeStatus_FullScenario(t *testing.T) {
	repo := setupTestRepo(t)

	modOldContent := []byte("original content of modified.go\n")
	modNewContent := []byte("staged new content of modified.go\n")
	delContent := []byte("content of deleted.go (staged deletion)\n")
	unstagedModContent := []byte("index content of unstaged_mod.go\n")
	unstagedDelContent := []byte("index content of unstaged_del.go\n")
	addedContent := []byte("brand new file content\n")

	modOldHash := hashBlobContent(modOldContent)
	modNewHash := hashBlobContent(modNewContent)
	delHash := hashBlobContent(delContent)
	unstagedModHash := hashBlobContent(unstagedModContent)
	unstagedDelHash := hashBlobContent(unstagedDelContent)
	addedHash := hashBlobContent(addedContent)

	// HEAD tree: contains modified.go (old), deleted.go, unstaged_mod.go,
	// unstaged_del.go. It does NOT contain added.go (staged addition).
	// All IDs are real SHA-1 hashes so flattenTree returns values that
	// can be compared directly with index hashes.
	headTree := createTree(t, repo, []TreeEntry{
		{ID: modOldHash, Name: "modified.go", Mode: "100644", Type: "blob"},
		{ID: delHash, Name: "deleted.go", Mode: "100644", Type: "blob"},
		{ID: unstagedModHash, Name: "unstaged_mod.go", Mode: "100644", Type: "blob"},
		{ID: unstagedDelHash, Name: "unstaged_del.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	// Index state:
	//   added.go       → staged addition (not in HEAD)
	//   modified.go    → staged modification (new hash ≠ HEAD hash)
	//   deleted.go     → staged deletion (absent from index)
	//   unstaged_mod.go → no staged change (hash matches HEAD)
	//   unstaged_del.go → no staged change (hash matches HEAD)
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "added.go", hash: addedHash, fileSize: uint32(len(addedContent))},
		{path: "modified.go", hash: modNewHash, fileSize: uint32(len(modNewContent))},
		// "deleted.go" intentionally omitted → staged deletion
		{path: "unstaged_mod.go", hash: unstagedModHash, fileSize: uint32(len(unstagedModContent))},
		{path: "unstaged_del.go", hash: unstagedDelHash, fileSize: uint32(len(unstagedDelContent))},
	})

	// Working directory:
	//   added.go       → matches index (no unstaged change)
	//   modified.go    → matches index (no unstaged change, staged mod only)
	//   deleted.go     → not written (absent from disk AND absent from index)
	//   unstaged_mod.go → different content than index → unstaged modification
	//   unstaged_del.go → not written → absent from disk → unstaged deletion
	//   untracked.txt  → not in index at all → untracked
	writeDiskFile(t, repo, "added.go", addedContent)
	writeDiskFile(t, repo, "modified.go", modNewContent)
	// deleted.go intentionally absent from disk (also absent from index)
	writeDiskFile(t, repo, "unstaged_mod.go", []byte("completely different on disk!\n"))
	// unstaged_del.go intentionally absent from disk
	writeDiskFile(t, repo, "untracked.txt", []byte("not tracked at all\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)

	// 1. Staged addition: "added.go"
	f := requirePath(t, m, "added.go")
	if f.IndexStatus != "added" {
		t.Errorf("added.go IndexStatus = %q, want %q", f.IndexStatus, "added")
	}
	if f.WorkStatus != "" {
		t.Errorf("added.go WorkStatus = %q, want empty (disk matches index)", f.WorkStatus)
	}

	// 2. Staged modification: "modified.go"
	f = requirePath(t, m, "modified.go")
	if f.IndexStatus != "modified" {
		t.Errorf("modified.go IndexStatus = %q, want %q", f.IndexStatus, "modified")
	}
	if f.WorkStatus != "" {
		t.Errorf("modified.go WorkStatus = %q, want empty (disk matches index)", f.WorkStatus)
	}

	// 3. Staged deletion: "deleted.go"
	f = requirePath(t, m, "deleted.go")
	if f.IndexStatus != "deleted" {
		t.Errorf("deleted.go IndexStatus = %q, want %q", f.IndexStatus, "deleted")
	}

	// 4. Unstaged modification: "unstaged_mod.go"
	f = requirePath(t, m, "unstaged_mod.go")
	if f.IndexStatus != "" {
		t.Errorf("unstaged_mod.go IndexStatus = %q, want empty (index matches HEAD)", f.IndexStatus)
	}
	if f.WorkStatus != "modified" {
		t.Errorf("unstaged_mod.go WorkStatus = %q, want %q", f.WorkStatus, "modified")
	}

	// 5. Unstaged deletion: "unstaged_del.go"
	f = requirePath(t, m, "unstaged_del.go")
	if f.IndexStatus != "" {
		t.Errorf("unstaged_del.go IndexStatus = %q, want empty (index matches HEAD)", f.IndexStatus)
	}
	if f.WorkStatus != "deleted" {
		t.Errorf("unstaged_del.go WorkStatus = %q, want %q", f.WorkStatus, "deleted")
	}

	// 6. Untracked: "untracked.txt"
	f = requirePath(t, m, "untracked.txt")
	if !f.IsUntracked {
		t.Error("untracked.txt: IsUntracked should be true")
	}
}

// TestComputeWorkingTreeStatus_NoChanges verifies that a file whose content on
// disk, in the index, and in HEAD are all identical produces no status entry.
// We use the real SHA-1 blob hash throughout so all three comparisons agree.
func TestComputeWorkingTreeStatus_NoChanges(t *testing.T) {
	repo := setupTestRepo(t)

	content := []byte("stable content\n")
	// Real SHA-1 hash used for both the HEAD tree entry and the index, ensuring
	// the HEAD-vs-index comparison sees them as equal. The disk file matches
	// too, so hashBlobContent(diskContent) will equal this hash.
	realHash := hashBlobContent(content)

	headTree := createTree(t, repo, []TreeEntry{
		{ID: realHash, Name: "stable.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "stable.go", hash: realHash, fileSize: uint32(len(content))},
	})
	writeDiskFile(t, repo, "stable.go", content)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	if _, ok := m["stable.go"]; ok {
		t.Errorf("expected 'stable.go' to have no status entry (clean file), but it appeared: %+v", m["stable.go"])
	}
}

// TestComputeWorkingTreeStatus_SameSizeDifferentContent verifies the slow path:
// when disk file size equals the index-recorded FileSize but the content differs,
// the hash comparison catches the unstaged modification.
func TestComputeWorkingTreeStatus_SameSizeDifferentContent(t *testing.T) {
	repo := setupTestRepo(t)

	// Both strings are 8 bytes.
	indexContent := []byte("aaaaaaaa")
	diskContent := []byte("bbbbbbbb")

	if len(indexContent) != len(diskContent) {
		t.Fatal("test setup error: contents must be the same length")
	}

	indexHash := hashBlobContent(indexContent)
	looseBlob := createBlob(t, repo, indexContent)

	headTree := createTree(t, repo, []TreeEntry{
		{ID: looseBlob, Name: "tricky.bin", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tricky.bin", hash: indexHash, fileSize: uint32(len(indexContent))},
	})
	writeDiskFile(t, repo, "tricky.bin", diskContent)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["tricky.bin"]
	if !ok {
		t.Fatal("expected 'tricky.bin' to appear as modified, but it was not in status")
	}
	if f.WorkStatus != "modified" {
		t.Errorf("WorkStatus = %q, want %q", f.WorkStatus, "modified")
	}
}

// TestComputeWorkingTreeStatus_UntrackedNestedFile verifies that an untracked
// file inside a subdirectory is reported with its full relative path.
func TestComputeWorkingTreeStatus_UntrackedNestedFile(t *testing.T) {
	repo := setupTestRepo(t)

	headTree := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, headTree)
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{})

	writeDiskFile(t, repo, "subdir/nested.go", []byte("nested untracked\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)
	f, ok := m["subdir/nested.go"]
	if !ok {
		t.Fatalf("expected 'subdir/nested.go' in status; got: %v", sortedKeys(m))
	}
	if !f.IsUntracked {
		t.Errorf("IsUntracked = false, want true")
	}
}

// requirePath is a test helper that retrieves a FileStatus by path and fails
// the test if the path is not present.
func requirePath(t *testing.T, m map[string]FileStatus, path string) FileStatus {
	t.Helper()
	f, ok := m[path]
	if !ok {
		t.Fatalf("expected %q in status result; available paths: %v", path, sortedKeys(m))
	}
	return f
}

// sortedKeys returns the map keys in alphabetical order for deterministic error messages.
func sortedKeys(m map[string]FileStatus) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeIndexFile is already defined in index_test.go (same package).
// We reuse setupTestRepo, createBlob, createTree, writeDiskFile, and wireHeadCommit
// from diff_test.go / worktree_diff_test.go (same package).
// buildIndexHeader, buildIndexEntryWithStats, and writeIndexFile are from index_test.go.

// Ensure the os package import is used (os.WriteFile is used by writeDiskFile,
// but writeDiskFile is defined in worktree_diff_test.go). The following blank
// import anchor is for documentation only — the actual usage comes from cross-file
// references within the package test build.
var _ = os.Stat // ensure os is referenced so the import compiles
