package gitcore

import (
	"strings"
	"testing"
	"time"
)

func TestFirstLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "empty", message: "", want: ""},
		{name: "single line", message: "subject", want: "subject"},
		{name: "multi line", message: "subject\nbody\n", want: "subject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := firstLine(tt.message); got != tt.want {
				t.Fatalf("firstLine(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestNewBlameEntry(t *testing.T) {
	t.Parallel()

	when := time.Unix(1700000000, 0)
	commit := &Commit{
		ID:      mustHash(t, testHash1),
		Message: "subject\nbody\n",
		Author: Signature{
			Name: "Jane Doe",
			When: when,
		},
	}

	got := newBlameEntry(commit)
	if got.CommitHash != commit.ID {
		t.Fatalf("CommitHash = %s, want %s", got.CommitHash, commit.ID)
	}
	if got.CommitMessage != "subject" {
		t.Fatalf("CommitMessage = %q, want %q", got.CommitMessage, "subject")
	}
	if got.AuthorName != "Jane Doe" {
		t.Fatalf("AuthorName = %q, want %q", got.AuthorName, "Jane Doe")
	}
	if !got.When.Equal(when) {
		t.Fatalf("When = %v, want %v", got.When, when)
	}
}

func TestBlameUnresolvedOnlyFillsMissingEntries(t *testing.T) {
	t.Parallel()

	commit := &Commit{
		ID:      mustHash(t, testHash2),
		Message: "fallback\n",
		Author: Signature{
			Name: "Fallback Author",
			When: time.Unix(1700003600, 0),
		},
	}
	existing := &BlameEntry{
		CommitHash:    mustHash(t, testHash1),
		CommitMessage: "existing",
		AuthorName:    "Original",
		When:          time.Unix(1700000000, 0),
	}
	entries := map[string]Hash{
		"keep.txt": mustHash(t, testHash3),
		"fill.txt": mustHash(t, testHash4),
	}
	blame := map[string]*BlameEntry{
		"keep.txt": existing,
	}

	blameUnresolved(blame, entries, commit)

	if blame["keep.txt"] != existing {
		t.Fatal("blameUnresolved overwrote an existing blame entry")
	}
	if blame["fill.txt"] == nil {
		t.Fatal("blameUnresolved did not populate missing entry")
	}
	if blame["fill.txt"].CommitHash != commit.ID {
		t.Fatalf("fill.txt commit = %s, want %s", blame["fill.txt"].CommitHash, commit.ID)
	}
}

func TestGetFileBlameCommitNotFound(t *testing.T) {
	t.Parallel()

	repo := newRepoSkeleton(t)
	missing := mustHash(t, testHash1)

	_, err := repo.GetFileBlame(missing, "src")
	if err == nil {
		t.Fatal("GetFileBlame() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "commit not found") {
		t.Fatalf("GetFileBlame() error = %v, want commit not found", err)
	}
}

func TestGetFileBlameResolveTreeError(t *testing.T) {
	repo := newRepoSkeleton(t)

	rootTree := mustHash(t, testHash1)
	commitID := mustHash(t, testHash2)

	writeLooseObject(t, repo.gitDir, rootTree, "tree", treeBodyWithEntries(
		treeEntry("100644", "README.md", mustHash(t, testHash3)),
	))

	repo.commitMap[commitID] = &Commit{
		ID:      commitID,
		Tree:    rootTree,
		Message: "root commit\n",
		Author: Signature{
			Name: "Alice",
			When: time.Unix(1700000000, 0),
		},
	}

	_, err := repo.GetFileBlame(commitID, "src")
	if err == nil {
		t.Fatal("GetFileBlame() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `failed to resolve tree at path "src"`) {
		t.Fatalf("GetFileBlame() error = %v, want wrapped resolve error", err)
	}
}

func TestGetFileBlameUsesTargetCommitWhenParentCommitMissing(t *testing.T) {
	repo := newRepoSkeleton(t)

	blob := mustHash(t, testHash1)
	srcTree := mustHash(t, testHash2)
	rootTree := mustHash(t, testHash3)
	commitID := mustHash(t, testHash4)
	missingParent := mustHash(t, testHash5)

	writeLooseObject(t, repo.gitDir, blob, "blob", []byte("content\n"))
	writeLooseObject(t, repo.gitDir, srcTree, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blob),
	))
	writeLooseObject(t, repo.gitDir, rootTree, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTree),
	))

	repo.commitMap[commitID] = &Commit{
		ID:      commitID,
		Tree:    rootTree,
		Parents: []Hash{missingParent},
		Message: "current change\n",
		Author: Signature{
			Name: "Bob",
			When: time.Unix(1700003600, 0),
		},
	}

	blame, err := repo.GetFileBlame(commitID, "src")
	if err != nil {
		t.Fatalf("GetFileBlame() error = %v", err)
	}

	entry := blame["file.txt"]
	if entry == nil {
		t.Fatal("missing blame entry for file.txt")
	}
	if entry.CommitHash != commitID {
		t.Fatalf("file.txt commit = %s, want %s", entry.CommitHash, commitID)
	}
	if entry.CommitMessage != "current change" {
		t.Fatalf("file.txt message = %q, want %q", entry.CommitMessage, "current change")
	}
}

func TestGetFileBlameUsesTargetCommitWhenParentLacksPath(t *testing.T) {
	repo := newRepoSkeleton(t)

	blob := mustHash(t, testHash1)
	srcTree := mustHash(t, testHash2)
	parentRootTree := mustHash(t, testHash3)
	targetRootTree := mustHash(t, testHash4)
	parentCommit := mustHash(t, testHash5)
	targetCommit := mustHash(t, testHash6)

	writeLooseObject(t, repo.gitDir, blob, "blob", []byte("content\n"))
	writeLooseObject(t, repo.gitDir, srcTree, "tree", treeBodyWithEntries(
		treeEntry("100644", "file.txt", blob),
	))
	writeLooseObject(t, repo.gitDir, parentRootTree, "tree", treeBodyWithEntries(
		treeEntry("100644", "README.md", mustHash(t, testHash7)),
	))
	writeLooseObject(t, repo.gitDir, targetRootTree, "tree", treeBodyWithEntries(
		treeEntry("040000", "src", srcTree),
	))

	repo.commitMap[parentCommit] = &Commit{
		ID:      parentCommit,
		Tree:    parentRootTree,
		Message: "before src\n",
		Author: Signature{
			Name: "Alice",
			When: time.Unix(1700000000, 0),
		},
	}
	repo.commitMap[targetCommit] = &Commit{
		ID:      targetCommit,
		Tree:    targetRootTree,
		Parents: []Hash{parentCommit},
		Message: "add src\n",
		Author: Signature{
			Name: "Bob",
			When: time.Unix(1700003600, 0),
		},
	}

	blame, err := repo.GetFileBlame(targetCommit, "src")
	if err != nil {
		t.Fatalf("GetFileBlame() error = %v", err)
	}

	entry := blame["file.txt"]
	if entry == nil {
		t.Fatal("missing blame entry for file.txt")
	}
	if entry.CommitHash != targetCommit {
		t.Fatalf("file.txt commit = %s, want %s", entry.CommitHash, targetCommit)
	}
	if entry.AuthorName != "Bob" {
		t.Fatalf("file.txt author = %q, want %q", entry.AuthorName, "Bob")
	}
}
