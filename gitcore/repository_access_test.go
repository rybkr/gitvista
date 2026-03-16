package gitcore

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryAccessCounts(t *testing.T) {
	repo := newRepoSkeleton(t)
	repo.commits = []*Commit{
		{ID: mustHash(t, testHash1)},
		{ID: mustHash(t, testHash2)},
	}
	repo.refs["refs/heads/main"] = mustHash(t, testHash1)
	repo.refs["refs/heads/feature"] = mustHash(t, testHash2)
	repo.refs["refs/remotes/origin/main"] = mustHash(t, testHash1)
	repo.refs["refs/tags/v1.0"] = mustHash(t, testHash1)
	repo.refs["refs/tags/v1.1"] = mustHash(t, testHash2)
	repo.stashes = []*StashEntry{
		{Hash: mustHash(t, testHash1), Message: "stash@{0}"},
	}

	if got := repo.CommitCount(); got != 2 {
		t.Fatalf("CommitCount() = %d, want 2", got)
	}
	if got := repo.BranchCount(); got != 2 {
		t.Fatalf("BranchCount() = %d, want 2", got)
	}
	if got := repo.TagCount(); got != 2 {
		t.Fatalf("TagCount() = %d, want 2", got)
	}
	if got := repo.StashCount(); got != 1 {
		t.Fatalf("StashCount() = %d, want 1", got)
	}
}

func TestRepositoryAccessHeadState(t *testing.T) {
	repo := newRepoSkeleton(t)
	head := mustHash(t, testHash4)
	repo.head = head
	repo.headRef = "refs/heads/main"

	if got := repo.Head(); got != head {
		t.Fatalf("Head() = %s, want %s", got, head)
	}
	if got := repo.HeadRef(); got != "refs/heads/main" {
		t.Fatalf("HeadRef() = %q, want %q", got, "refs/heads/main")
	}
	if repo.HeadDetached() {
		t.Fatal("HeadDetached() = true, want false")
	}

	repo.headDetached = true
	repo.headRef = ""

	if !repo.HeadDetached() {
		t.Fatal("HeadDetached() = false, want true")
	}
	if got := repo.HeadRef(); got != "" {
		t.Fatalf("HeadRef() = %q, want empty", got)
	}
}

func TestRepositoryAccessPathsAndBareState(t *testing.T) {
	repo := newRepoSkeleton(t)

	if got := repo.Name(); got != filepath.Base(repo.workDir) {
		t.Fatalf("Name() = %q, want %q", got, filepath.Base(repo.workDir))
	}
	if got := repo.GitDir(); got != repo.gitDir {
		t.Fatalf("GitDir() = %q, want %q", got, repo.gitDir)
	}
	if got := repo.WorkDir(); got != repo.workDir {
		t.Fatalf("WorkDir() = %q, want %q", got, repo.workDir)
	}
	if repo.IsBare() {
		t.Fatal("IsBare() = true, want false")
	}

	bare := NewEmptyRepository()
	bare.gitDir = "/tmp/repo.git"
	bare.workDir = "/tmp/repo.git"
	if !bare.IsBare() {
		t.Fatal("IsBare() = false, want true")
	}
}

func TestRepositoryAccessCollections(t *testing.T) {
	repo := newRepoSkeleton(t)

	commit1 := &Commit{ID: mustHash(t, testHash1)}
	commit2 := &Commit{ID: mustHash(t, testHash2)}
	repo.commitMap[commit1.ID] = commit1
	repo.commitMap[commit2.ID] = commit2

	repo.refs["refs/heads/main"] = commit1.ID
	repo.refs["refs/heads/feature"] = commit2.ID
	repo.refs["refs/remotes/origin/main"] = commit1.ID
	repo.refs["refs/tags/v1.0"] = commit1.ID
	repo.refs["refs/tags/v1.1"] = commit2.ID

	tagObject := &Tag{ID: mustHash(t, testHash3), Object: commit2.ID}
	repo.tags = []*Tag{tagObject}
	repo.refs["refs/tags/annotated"] = tagObject.ID

	stash := &StashEntry{Hash: commit1.ID, Message: "stash@{0}"}
	repo.stashes = []*StashEntry{stash}

	commits := repo.Commits()
	if len(commits) != 2 || commits[commit1.ID] != commit1 || commits[commit2.ID] != commit2 {
		t.Fatalf("Commits() = %#v", commits)
	}
	delete(commits, commit1.ID)
	if len(repo.commitMap) != 2 {
		t.Fatal("Commits() should return a copy of the map")
	}

	branches := repo.Branches()
	if len(branches) != 2 || branches["main"] != commit1.ID || branches["feature"] != commit2.ID {
		t.Fatalf("Branches() = %#v", branches)
	}
	if _, ok := branches["origin/main"]; ok {
		t.Fatalf("Branches() unexpectedly included remote branch: %#v", branches)
	}

	graphBranches := repo.GraphBranches()
	if len(graphBranches) != 3 {
		t.Fatalf("GraphBranches() len = %d, want 3", len(graphBranches))
	}
	if graphBranches["refs/heads/main"] != commit1.ID || graphBranches["refs/remotes/origin/main"] != commit1.ID {
		t.Fatalf("GraphBranches() = %#v", graphBranches)
	}

	tags := repo.Tags()
	if len(tags) != 3 {
		t.Fatalf("Tags() len = %d, want 3", len(tags))
	}
	if tags["v1.0"] != string(commit1.ID) || tags["annotated"] != string(commit2.ID) {
		t.Fatalf("Tags() = %#v", tags)
	}

	stashes := repo.Stashes()
	if len(stashes) != 1 || stashes[0] != stash {
		t.Fatalf("Stashes() = %#v", stashes)
	}
	stashes[0] = nil
	if repo.stashes[0] == nil {
		t.Fatal("Stashes() should return a copied slice")
	}
}

func TestRepositoryAccessRemotesWithoutConfig(t *testing.T) {
	repo := newRepoSkeleton(t)

	if got := repo.Remotes(); len(got) != 0 {
		t.Fatalf("Remotes() = %#v, want empty map", got)
	}
}

func TestLsTreeReturnsRootTreeEntriesForCommitRevision(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	treeID := mustHash(t, testHash2)
	commitID := mustHash(t, testHash3)
	subtreeID := mustHash(t, testHash4)
	treeBody := treeBodyWithEntries(
		treeEntry("100644", "README.md", blobID),
		treeEntry("040000", "docs", subtreeID),
	)
	commitBody := []byte("tree " + string(treeID) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\ninitial commit\n")

	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("hello world\n"))
	writeLooseObject(t, repo.gitDir, subtreeID, "tree", treeBodyWithEntries())
	writeLooseObject(t, repo.gitDir, treeID, "tree", treeBody)
	writeLooseObject(t, repo.gitDir, commitID, "commit", commitBody)
	repo.commitMap[commitID] = &Commit{ID: commitID, Tree: treeID}

	entries, err := repo.LsTree(LsTreeOptions{Revision: string(commitID)})
	if err != nil {
		t.Fatalf("LsTree() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("LsTree() len = %d, want 2", len(entries))
	}
	if entries[0].Name != "README.md" || entries[0].Mode != "100644" || entries[0].Type != ObjectTypeBlob || entries[0].ID != blobID {
		t.Fatalf("LsTree()[0] = %+v", entries[0])
	}
	if entries[1].Name != "docs" || entries[1].Mode != "040000" || entries[1].Type != ObjectTypeTree || entries[1].ID != subtreeID {
		t.Fatalf("LsTree()[1] = %+v", entries[1])
	}

	entries[0].Name = "mutated"
	again, err := repo.LsTree(LsTreeOptions{Revision: string(commitID)})
	if err != nil {
		t.Fatalf("LsTree() second call error: %v", err)
	}
	if again[0].Name != "README.md" {
		t.Fatalf("LsTree() did not return a defensive copy: %+v", again[0])
	}
}

func TestLsTreeResolvesHeadBranchAndTagToCommit(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	treeID := mustHash(t, testHash2)
	commitID := mustHash(t, testHash3)
	commitBody := []byte("tree " + string(treeID) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\ninitial commit\n")

	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("hello world\n"))
	writeLooseObject(t, repo.gitDir, treeID, "tree", treeBodyWithEntries(treeEntry("100644", "README.md", blobID)))
	writeLooseObject(t, repo.gitDir, commitID, "commit", commitBody)
	repo.commitMap[commitID] = &Commit{ID: commitID, Tree: treeID}
	repo.head = commitID
	repo.refs["refs/heads/main"] = commitID
	repo.refs["refs/tags/v1.0"] = commitID

	for _, revision := range []string{"HEAD", "main", "v1.0"} {
		t.Run(revision, func(t *testing.T) {
			entries, err := repo.LsTree(LsTreeOptions{Revision: revision})
			if err != nil {
				t.Fatalf("LsTree(%q) error: %v", revision, err)
			}
			if len(entries) != 1 || entries[0].Name != "README.md" {
				t.Fatalf("LsTree(%q) = %+v", revision, entries)
			}
		})
	}
}

func TestLsTreeRejectsMissingAndAmbiguousRevisions(t *testing.T) {
	repo := newRepoSkeleton(t)

	commit1 := mustHash(t, "abcdef0000000000000000000000000000000000")
	commit2 := mustHash(t, "abcdef1111111111111111111111111111111111")
	repo.commitMap[commit1] = &Commit{ID: commit1}
	repo.commitMap[commit2] = &Commit{ID: commit2}

	if _, err := repo.LsTree(LsTreeOptions{Revision: "missing"}); err == nil {
		t.Fatal("expected missing revision error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected missing revision error: %v", err)
	}

	if _, err := repo.LsTree(LsTreeOptions{Revision: "abcdef"}); err == nil {
		t.Fatal("expected ambiguous revision error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected ambiguous revision error: %v", err)
	}
}

func TestLsTreeRejectsNonCommitRevision(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("hello world\n"))
	repo.refs["refs/tags/blob-tag"] = blobID

	if _, err := repo.LsTree(LsTreeOptions{Revision: "blob-tag"}); err == nil {
		t.Fatal("expected non-commit error")
	} else if !strings.Contains(err.Error(), "is not a commit") {
		t.Fatalf("unexpected non-commit error: %v", err)
	}
}
