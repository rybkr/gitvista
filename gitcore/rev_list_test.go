package gitcore

import (
	"testing"
	"time"
)

func TestResolveRevision(t *testing.T) {
	head := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	other := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	tagObject := Hash("cccccccccccccccccccccccccccccccccccccccc")
	remote := Hash("dddddddddddddddddddddddddddddddddddddddd")

	repo := &Repository{
		head: head,
		refs: map[string]Hash{
			"refs/heads/main":          head,
			"refs/remotes/origin/main": remote,
			"refs/tags/v1.0.0":         tagObject,
		},
		commitMap: map[Hash]*Commit{
			head:   {ID: head},
			other:  {ID: other},
			remote: {ID: remote},
		},
		tags: []*Tag{{ID: tagObject, Object: other}},
	}

	tests := []struct {
		name     string
		revision string
		want     Hash
	}{
		{name: "head", revision: "HEAD", want: head},
		{name: "local branch", revision: "main", want: head},
		{name: "tag", revision: "v1.0.0", want: other},
		{name: "remote ref", revision: "refs/remotes/origin/main", want: remote},
		{name: "full hash", revision: string(other), want: other},
		{name: "short hash", revision: "bbbbbbb", want: other},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.ResolveRevision(tc.revision)
			if err != nil {
				t.Fatalf("ResolveRevision(%q) error = %v", tc.revision, err)
			}
			if got != tc.want {
				t.Fatalf("ResolveRevision(%q) = %s, want %s", tc.revision, got, tc.want)
			}
		})
	}
}

func TestResolveRevisionAmbiguousShortHash(t *testing.T) {
	first := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	second := Hash("aaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	repo := &Repository{
		commitMap: map[Hash]*Commit{
			first:  {ID: first},
			second: {ID: second},
		},
	}

	if _, err := repo.ResolveRevision("aaaaaaa"); err == nil {
		t.Fatal("ResolveRevision should fail for ambiguous short hash")
	}
}

func TestRevListAllDedupesRefsAndFiltersMerges(t *testing.T) {
	root := Hash("1111111111111111111111111111111111111111")
	side := Hash("2222222222222222222222222222222222222222")
	merge := Hash("3333333333333333333333333333333333333333")
	branchOnly := Hash("4444444444444444444444444444444444444444")
	tagObject := Hash("5555555555555555555555555555555555555555")

	now := time.Unix(1_700_000_000, 0)
	repo := &Repository{
		refs: map[string]Hash{
			"refs/heads/main":     merge,
			"refs/remotes/origin": merge,
			"refs/heads/feature":  branchOnly,
			"refs/tags/v1.0.0":    tagObject,
		},
		commitMap: map[Hash]*Commit{
			root:       {ID: root, Committer: Signature{When: now.Add(-3 * time.Hour)}},
			side:       {ID: side, Parents: []Hash{root}, Committer: Signature{When: now.Add(-2 * time.Hour)}},
			merge:      {ID: merge, Parents: []Hash{root, side}, Committer: Signature{When: now.Add(-1 * time.Hour)}},
			branchOnly: {ID: branchOnly, Parents: []Hash{root}, Committer: Signature{When: now}},
		},
		tags: []*Tag{{ID: tagObject, Object: merge}},
	}

	got, err := repo.RevList(RevListOptions{All: true, NoMerges: true})
	if err != nil {
		t.Fatalf("RevList(All) error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("RevList(All, NoMerges) len = %d, want 3", len(got))
	}
	for _, commit := range got {
		if commit.ID == merge {
			t.Fatal("RevList(All, NoMerges) should exclude merge commit")
		}
	}
}

func TestRevListTopoAndDateOrderPreserveTopology(t *testing.T) {
	root := Hash("1111111111111111111111111111111111111111")
	left := Hash("2222222222222222222222222222222222222222")
	right := Hash("3333333333333333333333333333333333333333")
	merge := Hash("4444444444444444444444444444444444444444")
	now := time.Unix(1_700_000_000, 0)

	repo := &Repository{
		commitMap: map[Hash]*Commit{
			root:  {ID: root, Committer: Signature{When: now.Add(-4 * time.Hour)}},
			left:  {ID: left, Parents: []Hash{root}, Committer: Signature{When: now.Add(-3 * time.Hour)}},
			right: {ID: right, Parents: []Hash{root}, Committer: Signature{When: now.Add(-2 * time.Hour)}},
			merge: {ID: merge, Parents: []Hash{left, right}, Committer: Signature{When: now.Add(-1 * time.Hour)}},
		},
	}

	for _, order := range []RevListOrder{RevListOrderTopo, RevListOrderDate} {
		commits, err := repo.RevList(RevListOptions{Revision: string(merge), Order: order})
		if err != nil {
			t.Fatalf("RevList(%v) error = %v", order, err)
		}
		pos := make(map[Hash]int, len(commits))
		for i, commit := range commits {
			pos[commit.ID] = i
		}
		if !(pos[merge] < pos[left] && pos[merge] < pos[right] && pos[left] < pos[root] && pos[right] < pos[root]) {
			t.Fatalf("RevList(%v) did not preserve topology: %#v", order, pos)
		}
	}
}

func TestRevListTopoOrderPreservesTraversalOrderForEqualDateTips(t *testing.T) {
	root := Hash("1111111111111111111111111111111111111111")
	left := Hash("2222222222222222222222222222222222222222")
	right := Hash("3333333333333333333333333333333333333333")
	now := time.Unix(1_700_000_000, 0)

	commits := map[Hash]*Commit{
		root:  {ID: root, Committer: Signature{When: now}},
		left:  {ID: left, Parents: []Hash{root}, Committer: Signature{When: now}},
		right: {ID: right, Parents: []Hash{root}, Committer: Signature{When: now}},
	}

	got := orderRevListCommits(commits, []Hash{left, right}, RevListOrderTopo)
	if len(got) != 3 {
		t.Fatalf("orderRevListCommits len = %d, want 3", len(got))
	}

	want := []Hash{left, right, root}
	for i, commit := range got {
		if commit.ID != want[i] {
			t.Fatalf("orderRevListCommits[%d] = %s, want %s", i, commit.ID, want[i])
		}
	}
}
