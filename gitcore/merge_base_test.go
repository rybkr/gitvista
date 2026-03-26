package gitcore

import (
	"slices"
	"testing"
	"time"
)

func TestMergeBase_LinearHistory(t *testing.T) {
	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashC := Hash("cccccccccccccccccccccccccccccccccccccccc")

	repo := NewEmptyRepository()
	repo.commits = []*Commit{
		{ID: hashA},
		{ID: hashB, Parents: []Hash{hashA}},
		{ID: hashC, Parents: []Hash{hashB}},
	}
	repo.commitMap = map[Hash]*Commit{
		hashA: repo.commits[0],
		hashB: repo.commits[1],
		hashC: repo.commits[2],
	}

	base, err := MergeBase(repo, hashB, hashC)
	if err != nil {
		t.Fatalf("MergeBase() error = %v", err)
	}
	if base != hashB {
		t.Fatalf("MergeBase() = %s, want %s", base, hashB)
	}
}

func TestMergeBase_DiamondHistory(t *testing.T) {
	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hashC := Hash("cccccccccccccccccccccccccccccccccccccccc")
	hashD := Hash("dddddddddddddddddddddddddddddddddddddddd")

	repo := NewEmptyRepository()
	repo.commits = []*Commit{
		{ID: hashA},
		{ID: hashB, Parents: []Hash{hashA}},
		{ID: hashC, Parents: []Hash{hashA}},
		{ID: hashD, Parents: []Hash{hashB, hashC}},
	}
	repo.commitMap = map[Hash]*Commit{
		hashA: repo.commits[0],
		hashB: repo.commits[1],
		hashC: repo.commits[2],
		hashD: repo.commits[3],
	}

	base, err := MergeBase(repo, hashB, hashC)
	if err != nil {
		t.Fatalf("MergeBase() error = %v", err)
	}
	if base != hashA {
		t.Fatalf("MergeBase() = %s, want %s", base, hashA)
	}
}

func TestMergeBase_NoCommonAncestor(t *testing.T) {
	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	repo := NewEmptyRepository()
	repo.commits = []*Commit{{ID: hashA}, {ID: hashB}}
	repo.commitMap = map[Hash]*Commit{
		hashA: repo.commits[0],
		hashB: repo.commits[1],
	}

	if _, err := MergeBase(repo, hashA, hashB); err == nil {
		t.Fatal("MergeBase() error = nil, want error")
	}
}

func TestMergeBase_PrefersBestAncestorOverNewerWorseAncestor(t *testing.T) {
	hashY := Hash("1111111111111111111111111111111111111111")
	hashX := Hash("2222222222222222222222222222222222222222")
	hashP := Hash("3333333333333333333333333333333333333333")
	hashQ := Hash("4444444444444444444444444444444444444444")
	hashC := Hash("5555555555555555555555555555555555555555")
	hashD := Hash("6666666666666666666666666666666666666666")
	hashO := Hash("7777777777777777777777777777777777777777")
	hashT := Hash("8888888888888888888888888888888888888888")

	now := time.Unix(1_700_000_000, 0)

	repo := NewEmptyRepository()
	repo.commits = []*Commit{
		{ID: hashY, Committer: Signature{When: now.Add(-8 * time.Hour)}},
		{ID: hashX, Parents: []Hash{hashY}, Committer: Signature{When: now.Add(-7 * time.Hour)}},
		{ID: hashP, Parents: []Hash{hashX}, Committer: Signature{When: now.Add(-6 * time.Hour)}},
		{ID: hashQ, Parents: []Hash{hashX}, Committer: Signature{When: now.Add(-5 * time.Hour)}},
		{ID: hashC, Parents: []Hash{hashY}, Committer: Signature{When: now.Add(-2 * time.Hour)}},
		{ID: hashD, Parents: []Hash{hashY}, Committer: Signature{When: now.Add(-1 * time.Hour)}},
		{ID: hashO, Parents: []Hash{hashP, hashC}, Committer: Signature{When: now}},
		{ID: hashT, Parents: []Hash{hashQ, hashD}, Committer: Signature{When: now.Add(-30 * time.Minute)}},
	}
	repo.commitMap = map[Hash]*Commit{
		hashY: repo.commits[0],
		hashX: repo.commits[1],
		hashP: repo.commits[2],
		hashQ: repo.commits[3],
		hashC: repo.commits[4],
		hashD: repo.commits[5],
		hashO: repo.commits[6],
		hashT: repo.commits[7],
	}

	base, err := MergeBase(repo, hashO, hashT)
	if err != nil {
		t.Fatalf("MergeBase() error = %v", err)
	}
	if base != hashX {
		t.Fatalf("MergeBase() = %s, want %s", base, hashX)
	}
}

func TestMergeBases_CrissCrossReturnsAllBestBases(t *testing.T) {
	left1 := Hash("1111111111111111111111111111111111111111")
	right1 := Hash("2222222222222222222222222222222222222222")
	leftMerge := Hash("3333333333333333333333333333333333333333")
	rightMerge := Hash("4444444444444444444444444444444444444444")

	repo := NewEmptyRepository()
	repo.commits = []*Commit{
		{ID: left1},
		{ID: right1},
		{ID: leftMerge, Parents: []Hash{left1, right1}},
		{ID: rightMerge, Parents: []Hash{right1, left1}},
	}
	repo.commitMap = map[Hash]*Commit{
		left1:      repo.commits[0],
		right1:     repo.commits[1],
		leftMerge:  repo.commits[2],
		rightMerge: repo.commits[3],
	}

	bases, err := MergeBases(repo, leftMerge, rightMerge)
	if err != nil {
		t.Fatalf("MergeBases() error = %v", err)
	}

	want := []Hash{left1, right1}
	if !slices.Equal(bases, want) {
		t.Fatalf("MergeBases() = %v, want %v", bases, want)
	}
}

func TestMergeBase_UsesPreferredBaseFromBestBaseSet(t *testing.T) {
	left1 := Hash("1111111111111111111111111111111111111111")
	right1 := Hash("2222222222222222222222222222222222222222")
	leftMerge := Hash("3333333333333333333333333333333333333333")
	rightMerge := Hash("4444444444444444444444444444444444444444")

	now := time.Unix(1_700_000_000, 0)
	repo := NewEmptyRepository()
	repo.commits = []*Commit{
		{ID: left1, Committer: Signature{When: now.Add(-2 * time.Hour)}},
		{ID: right1, Committer: Signature{When: now.Add(-time.Hour)}},
		{ID: leftMerge, Parents: []Hash{left1, right1}, Committer: Signature{When: now}},
		{ID: rightMerge, Parents: []Hash{right1, left1}, Committer: Signature{When: now}},
	}
	repo.commitMap = map[Hash]*Commit{
		left1:      repo.commits[0],
		right1:     repo.commits[1],
		leftMerge:  repo.commits[2],
		rightMerge: repo.commits[3],
	}

	base, err := MergeBase(repo, leftMerge, rightMerge)
	if err != nil {
		t.Fatalf("MergeBase() error = %v", err)
	}
	if base != right1 {
		t.Fatalf("MergeBase() = %s, want %s", base, right1)
	}
}
