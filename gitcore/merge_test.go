package gitcore

import (
	"slices"
	"testing"
	"time"
)

func addCommit(repo *Repository, c *Commit) {
	repo.commits = append(repo.commits, c)
	repo.commitMap[c.ID] = c
}

func makeCommit(hash Hash, parents []Hash, tree Hash, minutesAgo int) *Commit {
	when := time.Now().Add(-time.Duration(minutesAgo) * time.Minute)
	return &Commit{
		ID:        hash,
		Tree:      tree,
		Parents:   parents,
		Author:    Signature{Name: "Test", Email: "test@test.com", When: when},
		Committer: Signature{Name: "Test", Email: "test@test.com", When: when},
		Message:   "commit " + string(hash[:7]),
	}
}

func TestMergePreview_CleanMerge(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobOurs := createBlob(t, repo, []byte("ours content"))
	blobTheirs := createBlob(t, repo, []byte("theirs content"))

	baseTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "file-a.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobBase, Name: "file-b.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	oursTree := createTree(t, repo, []TreeEntry{
		{ID: blobOurs, Name: "file-a.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobBase, Name: "file-b.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	theirsTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "file-a.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobTheirs, Name: "file-b.txt", Mode: "100644", Type: ObjectTypeBlob},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")

	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.MergeBaseHash != hashBase {
		t.Fatalf("MergeBaseHash = %s, want %s", result.MergeBaseHash, hashBase)
	}
	if result.Stats.TotalFiles != 2 || result.Stats.Conflicts != 0 {
		t.Fatalf("stats = %+v, want total 2 and no conflicts", result.Stats)
	}
	for _, entry := range result.Entries {
		if entry.ConflictType != ConflictNone {
			t.Fatalf("%s conflict = %s, want none", entry.Path, entry.ConflictType)
		}
	}
}

func TestMergePreview_ContentConflict(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobOurs := createBlob(t, repo, []byte("ours version"))
	blobTheirs := createBlob(t, repo, []byte("theirs version"))

	baseTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "shared.txt", Mode: "100644", Type: ObjectTypeBlob}})
	oursTree := createTree(t, repo, []TreeEntry{{ID: blobOurs, Name: "shared.txt", Mode: "100644", Type: ObjectTypeBlob}})
	theirsTree := createTree(t, repo, []TreeEntry{{ID: blobTheirs, Name: "shared.txt", Mode: "100644", Type: ObjectTypeBlob}})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.Stats.Conflicts != 1 || len(result.Entries) != 1 {
		t.Fatalf("stats = %+v entries=%d, want 1 conflict and 1 entry", result.Stats, len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictConflicting {
		t.Fatalf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictConflicting)
	}
}

func TestMergePreview_DeleteModifyConflict(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobModified := createBlob(t, repo, []byte("modified content"))

	baseTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob}})
	oursTree := createTree(t, repo, []TreeEntry{})
	theirsTree := createTree(t, repo, []TreeEntry{{ID: blobModified, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob}})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.Stats.Conflicts != 1 || len(result.Entries) != 1 {
		t.Fatalf("stats = %+v entries=%d, want 1 conflict and 1 entry", result.Stats, len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictDeleteModify {
		t.Fatalf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictDeleteModify)
	}
}

func TestMergePreview_BothAdded(t *testing.T) {
	repo := setupTestRepo(t)

	blobOurs := createBlob(t, repo, []byte("ours new file"))
	blobTheirs := createBlob(t, repo, []byte("theirs new file"))

	baseTree := createTree(t, repo, []TreeEntry{})
	oursTree := createTree(t, repo, []TreeEntry{{ID: blobOurs, Name: "new.txt", Mode: "100644", Type: ObjectTypeBlob}})
	theirsTree := createTree(t, repo, []TreeEntry{{ID: blobTheirs, Name: "new.txt", Mode: "100644", Type: ObjectTypeBlob}})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.Stats.Conflicts != 1 || len(result.Entries) != 1 {
		t.Fatalf("stats = %+v entries=%d, want 1 conflict and 1 entry", result.Stats, len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictBothAdded {
		t.Fatalf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictBothAdded)
	}
}

func TestMergePreview_RenameVsModify(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base content"))
	blobModified := createBlob(t, repo, []byte("modified content"))

	baseTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "file.go", Mode: "100644", Type: ObjectTypeBlob}})
	oursTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "renamed.go", Mode: "100644", Type: ObjectTypeBlob}})
	theirsTree := createTree(t, repo, []TreeEntry{{ID: blobModified, Name: "file.go", Mode: "100644", Type: ObjectTypeBlob}})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.Stats.Conflicts != 1 || result.Stats.TotalFiles != 1 || len(result.Entries) != 1 {
		t.Fatalf("stats = %+v entries=%d, want 1 conflict and 1 file", result.Stats, len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictRenameModify {
		t.Fatalf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictRenameModify)
	}
}

func TestMergePreview_RenameVsRename(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("shared content"))
	baseTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "file.go", Mode: "100644", Type: ObjectTypeBlob}})
	oursTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "ours_name.go", Mode: "100644", Type: ObjectTypeBlob}})
	theirsTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "theirs_name.go", Mode: "100644", Type: ObjectTypeBlob}})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.Stats.Conflicts != 1 || len(result.Entries) != 1 {
		t.Fatalf("stats = %+v entries=%d, want 1 conflict and 1 entry", result.Stats, len(result.Entries))
	}
	if result.Entries[0].ConflictType != ConflictRenameRename {
		t.Fatalf("ConflictType = %s, want %s", result.Entries[0].ConflictType, ConflictRenameRename)
	}
}

func TestMergePreview_IdenticalChanges(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("original"))
	blobSame := createBlob(t, repo, []byte("same fix"))

	baseTree := createTree(t, repo, []TreeEntry{{ID: blobBase, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob}})
	sameTree := createTree(t, repo, []TreeEntry{{ID: blobSame, Name: "file.txt", Mode: "100644", Type: ObjectTypeBlob}})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, sameTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, sameTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}
	if result.Stats.Conflicts != 0 {
		t.Fatalf("Conflicts = %d, want 0", result.Stats.Conflicts)
	}
}

func TestMergePreview_ReturnsEntriesSortedByPath(t *testing.T) {
	repo := setupTestRepo(t)

	blobBase := createBlob(t, repo, []byte("base"))
	blobOurs := createBlob(t, repo, []byte("ours"))
	blobTheirs := createBlob(t, repo, []byte("theirs"))

	baseTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "a.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobBase, Name: "m.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobBase, Name: "z.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	oursTree := createTree(t, repo, []TreeEntry{
		{ID: blobOurs, Name: "a.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobBase, Name: "m.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobBase, Name: "z.txt", Mode: "100644", Type: ObjectTypeBlob},
	})
	theirsTree := createTree(t, repo, []TreeEntry{
		{ID: blobBase, Name: "a.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobTheirs, Name: "m.txt", Mode: "100644", Type: ObjectTypeBlob},
		{ID: blobTheirs, Name: "z.txt", Mode: "100644", Type: ObjectTypeBlob},
	})

	hashBase := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashOurs := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashTheirs := Hash("cccccccccccccccccccccccccccccccccccccccc")
	addCommit(repo, makeCommit(hashBase, nil, baseTree, 30))
	addCommit(repo, makeCommit(hashOurs, []Hash{hashBase}, oursTree, 20))
	addCommit(repo, makeCommit(hashTheirs, []Hash{hashBase}, theirsTree, 10))

	result, err := MergePreview(repo, hashOurs, hashTheirs)
	if err != nil {
		t.Fatalf("MergePreview() error = %v", err)
	}

	got := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		got = append(got, entry.Path)
	}

	want := []string{"a.txt", "m.txt", "z.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("entry order = %v, want %v", got, want)
	}
}
