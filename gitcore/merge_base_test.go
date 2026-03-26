package gitcore

import (
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
