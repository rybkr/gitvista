package repositoryview

import (
	"testing"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func makeCommit(hash gitcore.Hash, parents []gitcore.Hash, when time.Time, message string) *gitcore.Commit {
	return &gitcore.Commit{
		ID:        hash,
		Parents:   parents,
		Committer: gitcore.Signature{When: when},
		Author:    gitcore.Signature{When: when},
		Message:   message,
	}
}

func TestBuildCommitBranchAttribution_AssignsFromRefsAndMergeMessage(t *testing.T) {
	now := time.Now()
	root := makeCommit(gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now.Add(-5*time.Hour), "root")
	base := makeCommit(gitcore.Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []gitcore.Hash{root.ID}, now.Add(-4*time.Hour), "base")
	feature1 := makeCommit(gitcore.Hash("cccccccccccccccccccccccccccccccccccccccc"), []gitcore.Hash{base.ID}, now.Add(-3*time.Hour), "feature step 1")
	feature2 := makeCommit(gitcore.Hash("dddddddddddddddddddddddddddddddddddddddd"), []gitcore.Hash{feature1.ID}, now.Add(-2*time.Hour), "feature step 2")
	merge := makeCommit(gitcore.Hash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"), []gitcore.Hash{base.ID, feature2.ID}, now.Add(-time.Hour), "Merge branch 'feature/security' into dev")

	attribution := buildCommitBranchAttribution(
		map[gitcore.Hash]*gitcore.Commit{
			root.ID:     root,
			base.ID:     base,
			feature1.ID: feature1,
			feature2.ID: feature2,
			merge.ID:    merge,
		},
		map[string]gitcore.Hash{
			"refs/heads/dev": merge.ID,
		},
		"refs/heads/dev",
	)

	if got := attribution[merge.ID].Label; got != "dev" {
		t.Fatalf("merge label = %q, want %q", got, "dev")
	}
	if got := attribution[feature2.ID].Label; got != "feature/security" {
		t.Fatalf("feature2 label = %q, want %q", got, "feature/security")
	}
	if got := attribution[feature2.ID].Source; got != branchLabelSourceMergeMessage {
		t.Fatalf("feature2 source = %q, want %q", got, branchLabelSourceMergeMessage)
	}
}

func TestRepositoryDeltaIsEmpty(t *testing.T) {
	if !NewRepositoryDelta().IsEmpty() {
		t.Fatal("empty delta should report IsEmpty")
	}

	delta := NewRepositoryDelta()
	delta.AddedBranches["refs/heads/main"] = gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if delta.IsEmpty() {
		t.Fatal("delta with branch changes should not report IsEmpty")
	}
}

func TestBuildGraphSummary(t *testing.T) {
	now := time.Now()
	commit1 := makeCommit(gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now.Add(-2*time.Hour), "first")
	commit2 := makeCommit(gitcore.Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []gitcore.Hash{commit1.ID}, now.Add(-time.Hour), "second")
	commit3 := makeCommit(gitcore.Hash("cccccccccccccccccccccccccccccccccccccccc"), []gitcore.Hash{commit2.ID}, now, "third")

	summary := buildGraphSummary(
		map[gitcore.Hash]*gitcore.Commit{
			commit1.ID: commit1,
			commit2.ID: commit2,
			commit3.ID: commit3,
		},
		map[gitcore.Hash]branchAttribution{
			commit3.ID: {Label: "main", Source: branchLabelSourceHeadRef},
		},
		map[string]gitcore.Hash{
			"refs/heads/main":          commit3.ID,
			"refs/remotes/origin/main": commit3.ID,
		},
		map[string]string{"v1.0": string(commit1.ID)},
		commit3.ID,
		[]*gitcore.StashEntry{{Hash: commit1.ID, Message: "WIP"}},
	)

	if summary.TotalCommits != 3 {
		t.Fatalf("TotalCommits = %d, want 3", summary.TotalCommits)
	}
	if summary.HeadHash != string(commit3.ID) {
		t.Fatalf("HeadHash = %q, want %q", summary.HeadHash, commit3.ID)
	}
	if summary.OldestTimestamp != commit1.Committer.When.Unix() {
		t.Fatalf("OldestTimestamp = %d, want %d", summary.OldestTimestamp, commit1.Committer.When.Unix())
	}
	if summary.NewestTimestamp != commit3.Committer.When.Unix() {
		t.Fatalf("NewestTimestamp = %d, want %d", summary.NewestTimestamp, commit3.Committer.When.Unix())
	}
	if len(summary.Skeleton) != 3 {
		t.Fatalf("len(Skeleton) = %d, want 3", len(summary.Skeleton))
	}

	found := make(map[gitcore.Hash]CommitSkeleton, len(summary.Skeleton))
	for _, entry := range summary.Skeleton {
		found[entry.Hash] = entry
	}
	if got := found[commit3.ID].BranchLabel; got != "main" {
		t.Fatalf("commit3 branchLabel = %q, want %q", got, "main")
	}
	if got := found[commit3.ID].BranchLabelSource; got != branchLabelSourceHeadRef {
		t.Fatalf("commit3 branchLabelSource = %q, want %q", got, branchLabelSourceHeadRef)
	}
	if got := summary.Tags["v1.0"]; got != string(commit1.ID) {
		t.Fatalf("Tags[v1.0] = %q, want %q", got, commit1.ID)
	}
	if len(summary.Stashes) != 1 || summary.Stashes[0].Message != "WIP" {
		t.Fatalf("unexpected stashes: %+v", summary.Stashes)
	}
}

func TestAttributedCommitsPreservesBranchLabels(t *testing.T) {
	now := time.Now()
	commit := makeCommit(gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now, "tip")

	result := attributedCommits(
		map[gitcore.Hash]*gitcore.Commit{commit.ID: commit},
		[]gitcore.Hash{commit.ID},
		map[gitcore.Hash]branchAttribution{
			commit.ID: {Label: "main", Source: branchLabelSourceHeadRef},
		},
	)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0] == commit {
		t.Fatal("AttributedCommits returned original commit pointer")
	}
	if got := result[0].BranchLabel; got != "main" {
		t.Fatalf("BranchLabel = %q, want %q", got, "main")
	}
	if got := result[0].BranchLabelSource; got != branchLabelSourceHeadRef {
		t.Fatalf("BranchLabelSource = %q, want %q", got, branchLabelSourceHeadRef)
	}
}

func TestDiffRepositories(t *testing.T) {
	now := time.Now()
	oldCommit := makeCommit(gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now.Add(-2*time.Hour), "old")
	newCommit := makeCommit(gitcore.Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []gitcore.Hash{oldCommit.ID}, now, "new")

	delta := diffRepositories(
		map[gitcore.Hash]*gitcore.Commit{
			oldCommit.ID: oldCommit,
			newCommit.ID: newCommit,
		},
		map[gitcore.Hash]*gitcore.Commit{
			oldCommit.ID: oldCommit,
		},
		map[gitcore.Hash]branchAttribution{
			newCommit.ID: {Label: "main", Source: branchLabelSourceHeadRef},
		},
		map[gitcore.Hash]branchAttribution{},
		map[string]gitcore.Hash{"refs/heads/main": newCommit.ID},
		map[string]gitcore.Hash{"refs/heads/main": oldCommit.ID, "refs/heads/feature": oldCommit.ID},
		newCommit.ID,
		map[string]string{"v1.0": string(oldCommit.ID)},
		nil,
	)

	if len(delta.AddedCommits) != 1 || delta.AddedCommits[0].ID != newCommit.ID {
		t.Fatalf("unexpected AddedCommits: %+v", delta.AddedCommits)
	}
	if got := delta.AddedCommits[0].BranchLabel; got != "main" {
		t.Fatalf("AddedCommits[0].BranchLabel = %q, want %q", got, "main")
	}
	if got := delta.AmendedBranches["refs/heads/main"]; got != newCommit.ID {
		t.Fatalf("AmendedBranches[refs/heads/main] = %q, want %q", got, newCommit.ID)
	}
	if got := delta.DeletedBranches["refs/heads/feature"]; got != oldCommit.ID {
		t.Fatalf("DeletedBranches[refs/heads/feature] = %q, want %q", got, oldCommit.ID)
	}
	if delta.HeadHash != string(newCommit.ID) {
		t.Fatalf("HeadHash = %q, want %q", delta.HeadHash, newCommit.ID)
	}
	if len(delta.Stashes) != 0 {
		t.Fatalf("len(Stashes) = %d, want 0", len(delta.Stashes))
	}
}
