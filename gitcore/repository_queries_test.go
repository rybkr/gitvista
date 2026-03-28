package gitcore

import (
	"os"
	"path/filepath"
	"slices"
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
	if len(commits) != 2 || commits[commit1.ID] == nil || commits[commit2.ID] == nil {
		t.Fatalf("Commits() = %#v", commits)
	}
	if commits[commit1.ID] == commit1 || commits[commit2.ID] == commit2 {
		t.Fatal("Commits() should return cloned commit values")
	}
	delete(commits, commit1.ID)
	if len(repo.commitMap) != 2 {
		t.Fatal("Commits() should return a copy of the map")
	}
	commits[commit2.ID].Message = "changed"
	commits[commit2.ID].Parents = append(commits[commit2.ID].Parents, mustHash(t, testHash4))
	if repo.commitMap[commit2.ID].Message == "changed" || len(repo.commitMap[commit2.ID].Parents) != 0 {
		t.Fatal("Commits() should not expose mutable internal commit state")
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
	if len(stashes) != 1 || stashes[0] == nil {
		t.Fatalf("Stashes() = %#v", stashes)
	}
	if stashes[0] == stash {
		t.Fatal("Stashes() should return cloned stash values")
	}
	stashes[0] = nil
	if repo.stashes[0] == nil {
		t.Fatal("Stashes() should return a copied slice")
	}
	stashes = repo.Stashes()
	stashes[0].Message = "changed"
	if repo.stashes[0].Message == "changed" {
		t.Fatal("Stashes() should not expose mutable internal stash state")
	}
}

func TestRepositoryAccessGetCommitReturnsClone(t *testing.T) {
	repo := newRepoSkeleton(t)
	commit := &Commit{
		ID:      mustHash(t, testHash1),
		Parents: []Hash{mustHash(t, testHash2)},
		Message: "original",
	}
	repo.commitMap[commit.ID] = commit

	got, err := repo.GetCommit(commit.ID)
	if err != nil {
		t.Fatalf("GetCommit() error = %v", err)
	}
	if got == commit {
		t.Fatal("GetCommit() should return a cloned commit")
	}

	got.Message = "changed"
	got.Parents[0] = mustHash(t, testHash3)
	if repo.commitMap[commit.ID].Message != "original" {
		t.Fatal("GetCommit() should not expose mutable commit message state")
	}
	if repo.commitMap[commit.ID].Parents[0] != mustHash(t, testHash2) {
		t.Fatal("GetCommit() should not expose mutable parent slice state")
	}
}

func TestRepositoryAccessRemotesWithoutConfig(t *testing.T) {
	repo := newRepoSkeleton(t)

	if got := repo.Remotes(); len(got) != 0 {
		t.Fatalf("Remotes() = %#v, want empty map", got)
	}
}

func TestRepositoryAccessDescriptionAndTagNames(t *testing.T) {
	repo := newRepoSkeleton(t)

	writeTextFile(t, filepath.Join(repo.gitDir, "description"), "example repository\n")
	repo.refs["refs/tags/v1.0"] = mustHash(t, testHash1)
	repo.refs["refs/tags/v1.1"] = mustHash(t, testHash2)

	if got := repo.Description(); got != "example repository" {
		t.Fatalf("Description() = %q, want %q", got, "example repository")
	}

	tagNames := repo.TagNames()
	if len(tagNames) != 2 {
		t.Fatalf("TagNames() len = %d, want 2", len(tagNames))
	}
	if !slices.Equal(tagNames, []string{"v1.0", "v1.1"}) {
		t.Fatalf("TagNames() = %#v", tagNames)
	}

	writeTextFile(t, filepath.Join(repo.gitDir, "description"), "Unnamed repository; edit this file 'description' to name the repository.\n")
	if got := repo.Description(); got != "" {
		t.Fatalf("Description() placeholder = %q, want empty", got)
	}

	if err := os.Remove(filepath.Join(repo.gitDir, "description")); err != nil {
		t.Fatalf("remove description: %v", err)
	}
	if got := repo.Description(); got != "" {
		t.Fatalf("Description() missing file = %q, want empty", got)
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

func TestGetTreeRejectsInvalidHash(t *testing.T) {
	repo := newRepoSkeleton(t)

	if _, err := repo.GetTree(Hash("abc")); err == nil {
		t.Fatal("expected invalid hash error")
	} else if !strings.Contains(err.Error(), "invalid object hash") {
		t.Fatalf("unexpected invalid hash error: %v", err)
	}
}

func TestGetBlobRejectsInvalidHash(t *testing.T) {
	repo := newRepoSkeleton(t)

	if _, err := repo.GetBlob(Hash("abc")); err == nil {
		t.Fatal("expected invalid hash error")
	} else if !strings.Contains(err.Error(), "blob not found") {
		t.Fatalf("unexpected invalid hash error: %v", err)
	}
}

func TestRepositoryQueryHelpersAndResolveTreeAtPath(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	subtreeID := mustHash(t, testHash2)
	rootTreeID := mustHash(t, testHash3)
	commitID := mustHash(t, testHash4)

	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("hello world\n"))
	writeLooseObject(t, repo.gitDir, subtreeID, "tree", treeBodyWithEntries(treeEntry("100644", "nested.txt", blobID)))
	writeLooseObject(t, repo.gitDir, rootTreeID, "tree", treeBodyWithEntries(
		treeEntry("040000", "docs", subtreeID),
		treeEntry("100644", "README.md", blobID),
	))
	writeLooseObject(t, repo.gitDir, commitID, "commit", []byte("tree "+string(rootTreeID)+"\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nmsg\n"))
	repo.commitMap[commitID] = &Commit{ID: commitID, Tree: rootTreeID}
	repo.refs["refs/heads/main"] = commitID

	if got := cloneCommit(nil); got != nil {
		t.Fatalf("cloneCommit(nil) = %#v, want nil", got)
	}
	if got := cloneStashEntry(nil); got != nil {
		t.Fatalf("cloneStashEntry(nil) = %#v, want nil", got)
	}
	noParents := &Commit{ID: commitID}
	cloned := cloneCommit(noParents)
	if cloned == nil || cloned == noParents || cloned.Parents != nil {
		t.Fatalf("cloneCommit(no parents) = %#v", cloned)
	}

	if _, err := repo.GetCommit(blobID); err == nil || !strings.Contains(err.Error(), "commit not found") {
		t.Fatalf("GetCommit(missing) error = %v", err)
	}
	if _, err := repo.getCommit(blobID); err == nil || !strings.Contains(err.Error(), "commit not found") {
		t.Fatalf("getCommit(missing) error = %v", err)
	}

	tree, err := repo.GetTree(rootTreeID)
	if err != nil || len(tree.Entries) != 2 {
		t.Fatalf("GetTree() = %+v, %v", tree, err)
	}
	if _, err := repo.GetTree(blobID); err == nil || !strings.Contains(err.Error(), "is not a tree") {
		t.Fatalf("GetTree(blob) error = %v", err)
	}
	if _, err := repo.getTree(blobID); err == nil || !strings.Contains(err.Error(), "is not a tree") {
		t.Fatalf("getTree(blob) error = %v", err)
	}

	blobData, err := repo.GetBlob(blobID)
	if err != nil || string(blobData) != "hello world\n" {
		t.Fatalf("GetBlob() = %q, %v", string(blobData), err)
	}
	if _, err := repo.GetBlob(rootTreeID); err == nil || !strings.Contains(err.Error(), "is not a blob") {
		t.Fatalf("GetBlob(tree) error = %v", err)
	}

	if typ, err := repo.objectType(blobID); err != nil || typ != ObjectTypeBlob {
		t.Fatalf("objectType(blob) = %v, %v", typ, err)
	}
	if _, err := repo.objectType(Hash("abc")); err == nil {
		t.Fatal("expected objectType invalid hash error")
	}

	if _, err := repo.LsTree(LsTreeOptions{Revision: string(commitID)}); err != nil {
		t.Fatalf("LsTree(commit) error = %v", err)
	}
	delete(repo.refs, "refs/heads/main")
	repo.refs["refs/heads/missing"] = mustHash(t, testHash5)
	if _, err := repo.LsTree(LsTreeOptions{Revision: "missing"}); err == nil || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("LsTree(missing object) error = %v", err)
	}
	delete(repo.refs, "refs/heads/missing")
	repo.refs["refs/heads/main"] = commitID
	delete(repo.commitMap, commitID)
	if _, err := repo.LsTree(LsTreeOptions{Revision: "main"}); err == nil || !strings.Contains(err.Error(), "commit not found") {
		t.Fatalf("LsTree(missing cached commit) error = %v", err)
	}
	repo.commitMap[commitID] = &Commit{ID: commitID, Tree: Hash("abc")}
	if _, err := repo.LsTree(LsTreeOptions{Revision: string(commitID)}); err == nil || !strings.Contains(err.Error(), "failed to read tree object") {
		t.Fatalf("LsTree(bad tree) error = %v", err)
	}
	repo.commitMap[commitID] = &Commit{ID: commitID, Tree: rootTreeID}

	if tree, err := repo.resolveTreeAtPath(rootTreeID, ""); err != nil || tree.ID != rootTreeID {
		t.Fatalf("resolveTreeAtPath(root) = %+v, %v", tree, err)
	}
	if tree, err := repo.resolveTreeAtPath(rootTreeID, "/docs/"); err != nil || tree.ID != subtreeID {
		t.Fatalf("resolveTreeAtPath(/docs/) = %+v, %v", tree, err)
	}
	if _, err := repo.resolveTreeAtPath(rootTreeID, "README.md"); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("resolveTreeAtPath(file) error = %v", err)
	}
	if _, err := repo.resolveTreeAtPath(rootTreeID, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("resolveTreeAtPath(missing) error = %v", err)
	}
	if _, err := repo.resolveTreeAtPath(Hash("abc"), "docs"); err == nil || !strings.Contains(err.Error(), "failed to read tree") {
		t.Fatalf("resolveTreeAtPath(bad root) error = %v", err)
	}
}
