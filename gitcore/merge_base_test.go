package gitcore

import "testing"

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
