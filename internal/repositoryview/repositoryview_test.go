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
