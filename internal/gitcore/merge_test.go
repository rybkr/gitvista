package gitcore

import (
	"testing"
	"time"
)

// addCommit registers a commit in the repository's commit map and list.
func addCommit(repo *Repository, c *Commit) {
	repo.commits = append(repo.commits, c)
	repo.commitMap[c.ID] = c
}

// makeCommit creates a minimal Commit with the given hash, parents, tree, and a fixed timestamp offset.
func makeCommit(hash Hash, parents []Hash, tree Hash, minutesAgo int) *Commit {
	return &Commit{
		ID:      hash,
		Tree:    tree,
		Parents: parents,
		Author:  Signature{Name: "Test", Email: "test@test.com", When: time.Now().Add(-time.Duration(minutesAgo) * time.Minute)},
		Committer: Signature{Name: "Test", Email: "test@test.com", When: time.Now().Add(-time.Duration(minutesAgo) * time.Minute)},
		Message: "commit " + string(hash[:7]),
	}
}

// TestMergeBase_LinearHistory tests merge-base on a linear chain: A -> B -> C.
func TestMergeBase_LinearHistory(t *testing.T) {
	repo := setupTestRepo(t)

	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashC := Hash("cccccccccccccccccccccccccccccccccccccccc")

	treeA := createTree(t, repo, []TreeEntry{})
	treeB := createTree(t, repo, []TreeEntry{})
	treeC := createTree(t, repo, []TreeEntry{})

	addCommit(repo, makeCommit(hashA, nil, treeA, 30))
	addCommit(repo, makeCommit(hashB, []Hash{hashA}, treeB, 20))
	addCommit(repo, makeCommit(hashC, []Hash{hashB}, treeC, 10))

	base, err := MergeBase(repo, hashB, hashC)
	if err != nil {
		t.Fatalf("MergeBase failed: %v", err)
	}
	if base != hashB {
		t.Errorf("MergeBase = %s, want %s", base, hashB)
	}
}

// TestMergeBase_DiamondHistory tests merge-base on a diamond:
//
//	A -> B -> D
//	A -> C -> D
//
// merge-base(B, C) should be A.
func TestMergeBase_DiamondHistory(t *testing.T) {
	repo := setupTestRepo(t)

	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashC := Hash("cccccccccccccccccccccccccccccccccccccccc")

	tree := createTree(t, repo, []TreeEntry{})

	addCommit(repo, makeCommit(hashA, nil, tree, 30))
	addCommit(repo, makeCommit(hashB, []Hash{hashA}, tree, 20))
	addCommit(repo, makeCommit(hashC, []Hash{hashA}, tree, 10))

	base, err := MergeBase(repo, hashB, hashC)
	if err != nil {
		t.Fatalf("MergeBase failed: %v", err)
	}
	if base != hashA {
		t.Errorf("MergeBase = %s, want %s", base, hashA)
	}
}

// TestMergeBase_SameCommit tests that merge-base of a commit with itself is itself.
func TestMergeBase_SameCommit(t *testing.T) {
	repo := setupTestRepo(t)

	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	tree := createTree(t, repo, []TreeEntry{})
	addCommit(repo, makeCommit(hashA, nil, tree, 10))

	base, err := MergeBase(repo, hashA, hashA)
	if err != nil {
		t.Fatalf("MergeBase failed: %v", err)
	}
	if base != hashA {
		t.Errorf("MergeBase = %s, want %s", base, hashA)
	}
}

// TestMergeBase_NoCommonAncestor tests that two disconnected commits return an error.
func TestMergeBase_NoCommonAncestor(t *testing.T) {
	repo := setupTestRepo(t)

	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	treeA := createTree(t, repo, []TreeEntry{})
	treeB := createTree(t, repo, []TreeEntry{})

	addCommit(repo, makeCommit(hashA, nil, treeA, 20))
	addCommit(repo, makeCommit(hashB, nil, treeB, 10))

	_, err := MergeBase(repo, hashA, hashB)
	if err == nil {
		t.Fatal("expected error for no common ancestor, got nil")
	}
}

// TestMergePreview_CleanMerge tests a clean merge where ours and theirs change different files.
func TestMergePreview_CleanMerge(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobOurs := createBlob(t, repo, []byte("ours content"))
	blobTheirs := createBlob(t, repo, []byte("theirs content"))

	baseTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "file-a.txt", Mode: "100644", Type: "blob"},
		{ID: blobBase, Name: "file-b.txt", Mode: "100644", Type: "blob"},
	})

	oursTree := createTree(t, repo, []TreeEntry{
		{ID: blobOurs, Name: "file-a.txt", Mode: "100644", Type: "blob"},
		{ID: blobBase, Name: "file-b.txt", Mode: "100644", Type: "blob"},
	})

	theirsTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "file-a.txt", Mode: "100644", Type: "blob"},
		{ID: blobTheirs, Name: "file-b.txt", Mode: "100644", Type: "blob"},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")

	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview failed: %v", err)
	}

	if result.MergeBaseHash != hashBase {
		t.Errorf("MergeBaseHash = %s, want %s", result.MergeBaseHash, hashBase)
	}
	if result.Stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", result.Stats.TotalFiles)
	}
	if result.Stats.Conflicts != 0 {
		t.Errorf("Conflicts = %d, want 0", result.Stats.Conflicts)
	}

	for _, entry := range result.Entries {
		if entry.ConflictType != ConflictNone {
			t.Errorf("file %s: conflict = %s, want none", entry.Path, entry.ConflictType)
		}
	}
}

// TestMergePreview_ContentConflict tests that both sides modifying
// the same file to different content is classified as a conflict.
func TestMergePreview_ContentConflict(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobOurs := createBlob(t, repo, []byte("ours version"))
	blobTheirs := createBlob(t, repo, []byte("theirs version"))

	baseTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "shared.txt", Mode: "100644", Type: "blob"},
	})

	oursTree := createTree(t, repo, []TreeEntry{
		{ID: blobOurs, Name: "shared.txt", Mode: "100644", Type: "blob"},
	})

	theirsTree := createTree(t, repo, []TreeEntry{
		{ID: blobTheirs, Name: "shared.txt", Mode: "100644", Type: "blob"},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")

	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview failed: %v", err)
	}

	if result.Stats.Conflicts != 1 {
		t.Errorf("Conflicts = %d, want 1", result.Stats.Conflicts)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictConflicting {
		t.Errorf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictConflicting)
	}
}

// TestMergePreview_DeleteModifyConflict tests one side deleting
// a file while the other modifies it.
func TestMergePreview_DeleteModifyConflict(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobModified := createBlob(t, repo, []byte("modified content"))

	baseTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	// Ours deletes the file.
	oursTree := createTree(t, repo, []TreeEntry{})

	// Theirs modifies the file.
	theirsTree := createTree(t, repo, []TreeEntry{
		{ID: blobModified, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")

	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview failed: %v", err)
	}

	if result.Stats.Conflicts != 1 {
		t.Errorf("Conflicts = %d, want 1", result.Stats.Conflicts)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictDeleteModify {
		t.Errorf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictDeleteModify)
	}
}

// TestMergePreview_BothAdded tests both sides adding the same path with different content.
func TestMergePreview_BothAdded(t *testing.T) {
	repo := setupTestRepo(t)

	blobOurs := createBlob(t, repo, []byte("ours new file"))
	blobTheirs := createBlob(t, repo, []byte("theirs new file"))

	baseTree := createTree(t, repo, []TreeEntry{})

	oursTree := createTree(t, repo, []TreeEntry{
		{ID: blobOurs, Name: "new.txt", Mode: "100644", Type: "blob"},
	})

	theirsTree := createTree(t, repo, []TreeEntry{
		{ID: blobTheirs, Name: "new.txt", Mode: "100644", Type: "blob"},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")

	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview failed: %v", err)
	}

	if result.Stats.Conflicts != 1 {
		t.Errorf("Conflicts = %d, want 1", result.Stats.Conflicts)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictBothAdded {
		t.Errorf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictBothAdded)
	}
}

// TestMergePreview_IdenticalChanges tests that both sides making
// the same change (same resulting hash) is a trivial merge, not a conflict.
func TestMergePreview_IdenticalChanges(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("original"))
	blobSame := createBlob(t, repo, []byte("same fix"))

	baseTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	// Both sides have the same resulting tree.
	sameTree := createTree(t, repo, []TreeEntry{
		{ID: blobSame, Name: "file.txt", Mode: "100644", Type: "blob"},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")

	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, sameTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, sameTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview failed: %v", err)
	}

	if result.Stats.Conflicts != 0 {
		t.Errorf("Conflicts = %d, want 0 (identical changes should be trivial)", result.Stats.Conflicts)
	}
}
