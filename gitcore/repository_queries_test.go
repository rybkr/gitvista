package gitcore

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

func TestGetFileBlameForNestedDirectory(t *testing.T) {
	repo := newRepoSkeleton(t)

	blob1 := mustHash(t, testHash1)
	blob2 := mustHash(t, testHash2)
	blob3 := mustHash(t, testHash3)
	srcTree1 := mustHash(t, testHash4)
	srcTree2 := mustHash(t, testHash5)
	rootTree1 := mustHash(t, testHash6)
	rootTree2 := mustHash(t, testHash7)
	rootTree3 := mustHash(t, "8888888888888888888888888888888888888888")
	commit1 := mustHash(t, "9999999999999999999999999999999999999999")
	commit2 := mustHash(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	commit3 := mustHash(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	writeLooseObject(t, repo.gitDir, blob1, "blob", []byte("one\n"))
	writeLooseObject(t, repo.gitDir, blob2, "blob", []byte("two\n"))
	writeLooseObject(t, repo.gitDir, blob3, "blob", []byte("three\n"))
	writeLooseObject(t, repo.gitDir, srcTree1, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blob1),
	))
	writeLooseObject(t, repo.gitDir, srcTree2, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blob2),
		treeEntry("100644", "extra.txt", blob3),
	))
	writeLooseObject(t, repo.gitDir, rootTree1, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTree1),
	))
	writeLooseObject(t, repo.gitDir, rootTree2, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTree2),
	))
	writeLooseObject(t, repo.gitDir, rootTree3, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTree2),
	))

	writeLooseObject(t, repo.gitDir, commit1, "commit", []byte("tree "+string(rootTree1)+"\nauthor Alice <alice@example.com> 1700000000 +0000\ncommitter Alice <alice@example.com> 1700000000 +0000\n\ninitial\n"))
	writeLooseObject(t, repo.gitDir, commit2, "commit", []byte("tree "+string(rootTree2)+"\nparent "+string(commit1)+"\nauthor Bob <bob@example.com> 1700003600 +0000\ncommitter Bob <bob@example.com> 1700003600 +0000\n\nupdate src\nmore detail\n"))
	writeLooseObject(t, repo.gitDir, commit3, "commit", []byte("tree "+string(rootTree3)+"\nparent "+string(commit2)+"\nauthor Carol <carol@example.com> 1700007200 +0000\ncommitter Carol <carol@example.com> 1700007200 +0000\n\nno src changes\n"))

	repo.commitMap[commit1] = &Commit{
		ID:        commit1,
		Tree:      rootTree1,
		Author:    Signature{Name: "Alice", Email: "alice@example.com", When: time.Unix(1700000000, 0)},
		Committer: Signature{When: time.Unix(1700000000, 0)},
		Message:   "initial\n",
	}
	repo.commitMap[commit2] = &Commit{
		ID:        commit2,
		Tree:      rootTree2,
		Parents:   []Hash{commit1},
		Author:    Signature{Name: "Bob", Email: "bob@example.com", When: time.Unix(1700003600, 0)},
		Committer: Signature{When: time.Unix(1700003600, 0)},
		Message:   "update src\nmore detail\n",
	}
	repo.commitMap[commit3] = &Commit{
		ID:        commit3,
		Tree:      rootTree3,
		Parents:   []Hash{commit2},
		Author:    Signature{Name: "Carol", Email: "carol@example.com", When: time.Unix(1700007200, 0)},
		Committer: Signature{When: time.Unix(1700007200, 0)},
		Message:   "no src changes\n",
	}

	blame, err := repo.GetFileBlame(commit3, "src")
	if err != nil {
		t.Fatalf("GetFileBlame() error: %v", err)
	}

	if len(blame) != 2 {
		t.Fatalf("GetFileBlame() len = %d, want 2", len(blame))
	}
	if blame["file.txt"].CommitHash != commit2 || blame["file.txt"].AuthorName != "Bob" {
		t.Fatalf("file.txt blame = %+v", blame["file.txt"])
	}
	if blame["file.txt"].CommitMessage != "update src" {
		t.Fatalf("file.txt commit message = %q", blame["file.txt"].CommitMessage)
	}
	if blame["extra.txt"].CommitHash != commit2 {
		t.Fatalf("extra.txt blame = %+v", blame["extra.txt"])
	}
}

func TestGetFileBlameForMergeUsesPreservingParent(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobBase := mustHash(t, testHash1)
	blobOurs := mustHash(t, testHash2)
	blobOther := mustHash(t, testHash3)
	srcTreeBase := mustHash(t, testHash4)
	srcTreeOurs := mustHash(t, testHash5)
	srcTreeOther := mustHash(t, testHash6)
	rootTreeBase := mustHash(t, testHash7)
	rootTreeOurs := mustHash(t, "8888888888888888888888888888888888888888")
	rootTreeOther := mustHash(t, "9999999999999999999999999999999999999999")
	rootTreeMerge := mustHash(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	commitBase := mustHash(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	commitOurs := mustHash(t, "cccccccccccccccccccccccccccccccccccccccc")
	commitOther := mustHash(t, "dddddddddddddddddddddddddddddddddddddddd")
	commitMerge := mustHash(t, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")

	writeLooseObject(t, repo.gitDir, blobBase, "blob", []byte("base\n"))
	writeLooseObject(t, repo.gitDir, blobOurs, "blob", []byte("ours\n"))
	writeLooseObject(t, repo.gitDir, blobOther, "blob", []byte("other\n"))
	writeLooseObject(t, repo.gitDir, srcTreeBase, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blobBase),
	))
	writeLooseObject(t, repo.gitDir, srcTreeOurs, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blobOurs),
	))
	writeLooseObject(t, repo.gitDir, srcTreeOther, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blobOther),
	))
	writeLooseObject(t, repo.gitDir, rootTreeBase, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTreeBase),
	))
	writeLooseObject(t, repo.gitDir, rootTreeOurs, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTreeOurs),
	))
	writeLooseObject(t, repo.gitDir, rootTreeOther, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTreeOther),
	))
	writeLooseObject(t, repo.gitDir, rootTreeMerge, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTreeOurs),
	))

	writeLooseObject(t, repo.gitDir, commitBase, "commit", []byte("tree "+string(rootTreeBase)+"\nauthor Alice <alice@example.com> 1700000000 +0000\ncommitter Alice <alice@example.com> 1700000000 +0000\n\nbase\n"))
	writeLooseObject(t, repo.gitDir, commitOurs, "commit", []byte("tree "+string(rootTreeOurs)+"\nparent "+string(commitBase)+"\nauthor Bob <bob@example.com> 1700003600 +0000\ncommitter Bob <bob@example.com> 1700003600 +0000\n\nours change\n"))
	writeLooseObject(t, repo.gitDir, commitOther, "commit", []byte("tree "+string(rootTreeOther)+"\nparent "+string(commitBase)+"\nauthor Carol <carol@example.com> 1700007200 +0000\ncommitter Carol <carol@example.com> 1700007200 +0000\n\nother change\n"))
	writeLooseObject(t, repo.gitDir, commitMerge, "commit", []byte("tree "+string(rootTreeMerge)+"\nparent "+string(commitOurs)+"\nparent "+string(commitOther)+"\nauthor Dana <dana@example.com> 1700010800 +0000\ncommitter Dana <dana@example.com> 1700010800 +0000\n\nmerge branch\n"))

	repo.commitMap[commitBase] = &Commit{
		ID:        commitBase,
		Tree:      rootTreeBase,
		Author:    Signature{Name: "Alice", Email: "alice@example.com", When: time.Unix(1700000000, 0)},
		Committer: Signature{When: time.Unix(1700000000, 0)},
		Message:   "base\n",
	}
	repo.commitMap[commitOurs] = &Commit{
		ID:        commitOurs,
		Tree:      rootTreeOurs,
		Parents:   []Hash{commitBase},
		Author:    Signature{Name: "Bob", Email: "bob@example.com", When: time.Unix(1700003600, 0)},
		Committer: Signature{When: time.Unix(1700003600, 0)},
		Message:   "ours change\n",
	}
	repo.commitMap[commitOther] = &Commit{
		ID:        commitOther,
		Tree:      rootTreeOther,
		Parents:   []Hash{commitBase},
		Author:    Signature{Name: "Carol", Email: "carol@example.com", When: time.Unix(1700007200, 0)},
		Committer: Signature{When: time.Unix(1700007200, 0)},
		Message:   "other change\n",
	}
	repo.commitMap[commitMerge] = &Commit{
		ID:        commitMerge,
		Tree:      rootTreeMerge,
		Parents:   []Hash{commitOurs, commitOther},
		Author:    Signature{Name: "Dana", Email: "dana@example.com", When: time.Unix(1700010800, 0)},
		Committer: Signature{When: time.Unix(1700010800, 0)},
		Message:   "merge branch\n",
	}

	blame, err := repo.GetFileBlame(commitMerge, "src")
	if err != nil {
		t.Fatalf("GetFileBlame() error: %v", err)
	}

	entry := blame["file.txt"]
	if entry == nil {
		t.Fatal("missing blame entry for file.txt")
	}
	if entry.CommitHash != commitOurs || entry.AuthorName != "Bob" {
		t.Fatalf("file.txt blame = %+v, want commit %s by Bob", entry, commitOurs)
	}
	if entry.CommitMessage != "ours change" {
		t.Fatalf("file.txt commit message = %q, want %q", entry.CommitMessage, "ours change")
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
