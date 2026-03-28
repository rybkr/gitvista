package gitcore

import (
	"strings"
	"testing"
	"time"
)

func TestResolveRevision(t *testing.T) {
	head := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	other := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	tagObject := Hash("cccccccccccccccccccccccccccccccccccccccc")
	remote := Hash("dddddddddddddddddddddddddddddddddddddddd")
	grandparent := Hash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")

	repo := &Repository{
		head: head,
		refs: map[string]Hash{
			"refs/heads/main":          head,
			"refs/remotes/origin/main": remote,
			"refs/tags/v1.0.0":         tagObject,
		},
		commitMap: map[Hash]*Commit{
			head:        {ID: head, Parents: []Hash{other}},
			other:       {ID: other, Parents: []Hash{grandparent}},
			remote:      {ID: remote},
			grandparent: {ID: grandparent},
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
		{name: "head tilde zero", revision: "HEAD~0", want: head},
		{name: "head tilde one", revision: "HEAD~1", want: other},
		{name: "head tilde two", revision: "HEAD~2", want: grandparent},
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

func TestResolveRevisionHeadTildeErrors(t *testing.T) {
	head := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	parent := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	repo := &Repository{
		head: head,
		commitMap: map[Hash]*Commit{
			head:   {ID: head, Parents: []Hash{parent}},
			parent: {ID: parent},
		},
	}

	for _, revision := range []string{"HEAD~x", "HEAD~-1", "HEAD~2"} {
		t.Run(revision, func(t *testing.T) {
			if _, err := repo.ResolveRevision(revision); err == nil {
				t.Fatalf("ResolveRevision(%q) error = nil, want error", revision)
			}
		})
	}
}

func TestResolveRevisionRejectsMissingHead(t *testing.T) {
	repo := &Repository{
		commitMap: map[Hash]*Commit{},
	}

	for _, revision := range []string{"HEAD", "HEAD~1"} {
		t.Run(revision, func(t *testing.T) {
			if _, err := repo.ResolveRevision(revision); err == nil || !strings.Contains(err.Error(), "ambiguous argument") {
				t.Fatalf("ResolveRevision(%q) error = %v, want ambiguous argument", revision, err)
			}
		})
	}
}

func TestRevListErrorAndEmptyStartBranches(t *testing.T) {
	repo := &Repository{
		commitMap: map[Hash]*Commit{},
	}

	if _, err := repo.RevList(RevListOptions{Revision: "missing"}); err == nil || !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("RevList(missing) error = %v, want ambiguous argument", err)
	}

	commits, err := repo.RevList(RevListOptions{})
	if err != nil {
		t.Fatalf("RevList(empty) error = %v", err)
	}
	if commits != nil {
		t.Fatalf("RevList(empty) = %#v, want nil", commits)
	}
}

func TestChronologicalRevListCommitsSkipsMissingAndDuplicateStarts(t *testing.T) {
	root := Hash("1111111111111111111111111111111111111111")
	child := Hash("2222222222222222222222222222222222222222")
	now := time.Unix(1_700_000_000, 0)

	commits := map[Hash]*Commit{
		root:  {ID: root, Committer: Signature{When: now.Add(-time.Hour)}},
		child: {ID: child, Parents: []Hash{root}, Committer: Signature{When: now}},
	}

	got := chronologicalRevListCommits(commits, []Hash{child, child, Hash("missing")})
	if len(got) != 2 {
		t.Fatalf("chronologicalRevListCommits len = %d, want 2", len(got))
	}
	if got[0].ID != child || got[1].ID != root {
		t.Fatalf("chronologicalRevListCommits order = [%s %s], want [%s %s]", got[0].ID, got[1].ID, child, root)
	}
}

func TestRevListStartPointsSkipsEmptyHashesAndChronologicalSkipsMissingParents(t *testing.T) {
	startRepo := &Repository{
		refs: map[string]Hash{
			"refs/heads/main":          "",
			"refs/remotes/origin/main": Hash("1111111111111111111111111111111111111111"),
			"refs/tags/v1.0.0":         "",
		},
	}
	starts, err := startRepo.revListStartPoints(RevListOptions{All: true})
	if err != nil {
		t.Fatalf("revListStartPoints(All) error = %v", err)
	}
	if len(starts) != 1 || starts[0] != Hash("1111111111111111111111111111111111111111") {
		t.Fatalf("revListStartPoints(All) = %#v, want single non-empty start", starts)
	}

	root := Hash("2222222222222222222222222222222222222222")
	missingParent := Hash("3333333333333333333333333333333333333333")
	now := time.Unix(1_700_000_000, 0)
	ordered := chronologicalRevListCommits(map[Hash]*Commit{
		root: {ID: root, Parents: []Hash{missingParent}, Committer: Signature{When: now}},
	}, []Hash{root})
	if len(ordered) != 1 || ordered[0].ID != root {
		t.Fatalf("chronologicalRevListCommits(missing parent) = %#v", ordered)
	}
}

func TestTopoOrderRevListCommitsHandlesEmptyAndExternalParents(t *testing.T) {
	if got := topoOrderRevListCommits(nil, false); got != nil {
		t.Fatalf("topoOrderRevListCommits(nil) = %#v, want nil", got)
	}

	root := Hash("1111111111111111111111111111111111111111")
	child := Hash("2222222222222222222222222222222222222222")
	external := Hash("3333333333333333333333333333333333333333")
	now := time.Unix(1_700_000_000, 0)

	commits := []*Commit{
		{ID: child, Parents: []Hash{root, external}, Committer: Signature{When: now}},
		{ID: root, Committer: Signature{When: now.Add(-time.Hour)}},
	}

	for _, useDate := range []bool{false, true} {
		got := topoOrderRevListCommits(commits, useDate)
		if len(got) != 2 {
			t.Fatalf("topoOrderRevListCommits(useDate=%v) len = %d, want 2", useDate, len(got))
		}
		if got[0].ID != child || got[1].ID != root {
			t.Fatalf("topoOrderRevListCommits(useDate=%v) order = [%s %s], want [%s %s]", useDate, got[0].ID, got[1].ID, child, root)
		}
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
		if pos[merge] >= pos[left] || pos[merge] >= pos[right] || pos[left] >= pos[root] || pos[right] >= pos[root] {
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

func TestRevListTopoOrderKeepsSingleParentChainTogether(t *testing.T) {
	root := Hash("1111111111111111111111111111111111111111")
	left := Hash("2222222222222222222222222222222222222222")
	leftParent := Hash("3333333333333333333333333333333333333333")
	right := Hash("4444444444444444444444444444444444444444")
	now := time.Unix(1_700_000_000, 0)

	commits := map[Hash]*Commit{
		root:       {ID: root, Committer: Signature{When: now.Add(-3 * time.Hour)}},
		left:       {ID: left, Parents: []Hash{leftParent}, Committer: Signature{When: now}},
		leftParent: {ID: leftParent, Parents: []Hash{root}, Committer: Signature{When: now.Add(-2 * time.Hour)}},
		right:      {ID: right, Parents: []Hash{root}, Committer: Signature{When: now.Add(-1 * time.Hour)}},
	}

	got := orderRevListCommits(commits, []Hash{left, right}, RevListOrderTopo)
	want := []Hash{left, leftParent, right, root}
	if len(got) != len(want) {
		t.Fatalf("orderRevListCommits len = %d, want %d", len(got), len(want))
	}
	for i, commit := range got {
		if commit.ID != want[i] {
			t.Fatalf("orderRevListCommits[%d] = %s, want %s", i, commit.ID, want[i])
		}
	}
}

func TestRevListDateOrderUsesCommitDateWhenParentsBecomeReady(t *testing.T) {
	root := Hash("1111111111111111111111111111111111111111")
	left := Hash("2222222222222222222222222222222222222222")
	leftParent := Hash("3333333333333333333333333333333333333333")
	right := Hash("4444444444444444444444444444444444444444")
	now := time.Unix(1_700_000_000, 0)

	commits := map[Hash]*Commit{
		root:       {ID: root, Committer: Signature{When: now.Add(-4 * time.Hour)}},
		left:       {ID: left, Parents: []Hash{leftParent}, Committer: Signature{When: now}},
		leftParent: {ID: leftParent, Parents: []Hash{root}, Committer: Signature{When: now.Add(-3 * time.Hour)}},
		right:      {ID: right, Parents: []Hash{root}, Committer: Signature{When: now.Add(-1 * time.Hour)}},
	}

	got := orderRevListCommits(commits, []Hash{left, right}, RevListOrderDate)
	want := []Hash{left, right, leftParent, root}
	if len(got) != len(want) {
		t.Fatalf("orderRevListCommits len = %d, want %d", len(got), len(want))
	}
	for i, commit := range got {
		if commit.ID != want[i] {
			t.Fatalf("orderRevListCommits[%d] = %s, want %s", i, commit.ID, want[i])
		}
	}
}
