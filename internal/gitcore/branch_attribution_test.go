package gitcore

import (
	"testing"
	"time"
)

func makeAttributedCommit(hash Hash, parents []Hash, when time.Time, message string) *Commit {
	return &Commit{
		ID:        hash,
		Parents:   parents,
		Committer: Signature{When: when},
		Author:    Signature{When: when},
		Message:   message,
	}
}

func TestBuildGraphSummary_AssignsBranchLabelsFromRefs(t *testing.T) {
	now := time.Now()
	root := makeAttributedCommit(Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now.Add(-3*time.Hour), "root")
	base := makeAttributedCommit(Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []Hash{root.ID}, now.Add(-2*time.Hour), "base")
	mainTip := makeAttributedCommit(Hash("cccccccccccccccccccccccccccccccccccccccc"), []Hash{base.ID}, now.Add(-time.Hour), "main")
	devTip := makeAttributedCommit(Hash("dddddddddddddddddddddddddddddddddddddddd"), []Hash{base.ID}, now.Add(-30*time.Minute), "dev")

	repo := &Repository{
		headRef: "refs/heads/dev",
		head:    devTip.ID,
		commits: []*Commit{root, base, mainTip, devTip},
		commitMap: map[Hash]*Commit{
			root.ID:    root,
			base.ID:    base,
			mainTip.ID: mainTip,
			devTip.ID:  devTip,
		},
		refs: map[string]Hash{
			"refs/heads/main":          mainTip.ID,
			"refs/heads/dev":           devTip.ID,
			"refs/remotes/origin/dev":  devTip.ID,
			"refs/remotes/origin/main": mainTip.ID,
		},
	}

	summary := repo.BuildGraphSummary()
	found := make(map[Hash]CommitSkeleton, len(summary.Skeleton))
	for _, entry := range summary.Skeleton {
		found[entry.Hash] = entry
	}

	if got := found[devTip.ID].BranchLabel; got != "dev" {
		t.Fatalf("dev tip branchLabel = %q, want %q", got, "dev")
	}
	if got := found[devTip.ID].BranchLabelSource; got != branchLabelSourceHeadRef {
		t.Fatalf("dev tip branchLabelSource = %q, want %q", got, branchLabelSourceHeadRef)
	}
	if got := found[mainTip.ID].BranchLabel; got != "main" {
		t.Fatalf("main tip branchLabel = %q, want %q", got, "main")
	}
	if got := found[mainTip.ID].BranchLabelSource; got != branchLabelSourceLocalRef {
		t.Fatalf("main tip branchLabelSource = %q, want %q", got, branchLabelSourceLocalRef)
	}
}

func TestBuildGraphSummary_AssignsBranchLabelsFromMergeMessages(t *testing.T) {
	now := time.Now()
	root := makeAttributedCommit(Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now.Add(-5*time.Hour), "root")
	base := makeAttributedCommit(Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []Hash{root.ID}, now.Add(-4*time.Hour), "base")
	feature1 := makeAttributedCommit(Hash("cccccccccccccccccccccccccccccccccccccccc"), []Hash{base.ID}, now.Add(-3*time.Hour), "feature step 1")
	feature2 := makeAttributedCommit(Hash("dddddddddddddddddddddddddddddddddddddddd"), []Hash{feature1.ID}, now.Add(-2*time.Hour), "feature step 2")
	merge := makeAttributedCommit(
		Hash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
		[]Hash{base.ID, feature2.ID},
		now.Add(-time.Hour),
		"Merge branch 'feature/security' into dev",
	)

	repo := &Repository{
		headRef: "refs/heads/dev",
		head:    merge.ID,
		commits: []*Commit{root, base, feature1, feature2, merge},
		commitMap: map[Hash]*Commit{
			root.ID:     root,
			base.ID:     base,
			feature1.ID: feature1,
			feature2.ID: feature2,
			merge.ID:    merge,
		},
		refs: map[string]Hash{
			"refs/heads/dev": merge.ID,
		},
	}

	summary := repo.BuildGraphSummary()
	found := make(map[Hash]CommitSkeleton, len(summary.Skeleton))
	for _, entry := range summary.Skeleton {
		found[entry.Hash] = entry
	}

	if got := found[feature2.ID].BranchLabel; got != "feature/security" {
		t.Fatalf("feature2 branchLabel = %q, want %q", got, "feature/security")
	}
	if got := found[feature2.ID].BranchLabelSource; got != branchLabelSourceMergeMessage {
		t.Fatalf("feature2 branchLabelSource = %q, want %q", got, branchLabelSourceMergeMessage)
	}
	if got := found[feature1.ID].BranchLabel; got != "feature/security" {
		t.Fatalf("feature1 branchLabel = %q, want %q", got, "feature/security")
	}
	if got := found[base.ID].BranchLabel; got == "feature/security" {
		t.Fatalf("base branchLabel = %q, did not expect feature label on merge base", got)
	}
}

func TestGetCommits_PreservesBranchLabels(t *testing.T) {
	now := time.Now()
	commit := makeAttributedCommit(Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), nil, now, "tip")

	repo := &Repository{
		headRef: "refs/heads/main",
		head:    commit.ID,
		commits: []*Commit{commit},
		commitMap: map[Hash]*Commit{
			commit.ID: commit,
		},
		refs: map[string]Hash{
			"refs/heads/main": commit.ID,
		},
	}

	result := repo.GetCommits([]Hash{commit.ID})
	if len(result) != 1 {
		t.Fatalf("GetCommits() returned %d commits, want 1", len(result))
	}
	if got := result[0].BranchLabel; got != "main" {
		t.Fatalf("GetCommits()[0].BranchLabel = %q, want %q", got, "main")
	}
	if got := result[0].BranchLabelSource; got != branchLabelSourceHeadRef {
		t.Fatalf("GetCommits()[0].BranchLabelSource = %q, want %q", got, branchLabelSourceHeadRef)
	}
}
