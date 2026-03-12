package gitcore

import "testing"

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
