package gitcore

import (
	"container/heap"
	"testing"
	"time"
)

func TestCommitLogHeapOrdersNewerCommitsFirst(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	older := &Commit{ID: mustHash(t, testHash1), Committer: Signature{When: now.Add(-time.Hour)}}
	newer := &Commit{ID: mustHash(t, testHash2), Committer: Signature{When: now}}

	h := &commitLogHeap{}
	heap.Init(h)
	heap.Push(h, older)
	heap.Push(h, newer)

	if !h.Less(0, 1) && !h.Less(1, 0) {
		t.Fatal("commitLogHeap.Less should prefer the newer commit")
	}

	got := heap.Pop(h).(*Commit) //nolint:forcetypeassert
	if got.ID != newer.ID {
		t.Fatalf("heap.Pop() = %s, want %s", got.ID, newer.ID)
	}
}

func TestCommitLogReturnsNilWhenHeadCommitIsMissing(t *testing.T) {
	repo := NewEmptyRepository()
	repo.head = mustHash(t, testHash1)

	if log := repo.CommitLog(0); log != nil {
		t.Fatalf("CommitLog() = %v, want nil", log)
	}
}

func TestCommitLogSkipsMissingParents(t *testing.T) {
	head := mustHash(t, testHash1)
	missingParent := mustHash(t, testHash2)
	now := time.Unix(1_700_000_000, 0)

	repo := NewEmptyRepository()
	repo.head = head
	repo.commitMap[head] = &Commit{
		ID:        head,
		Parents:   []Hash{missingParent},
		Committer: Signature{When: now},
		Message:   "head",
	}

	log := repo.CommitLog(0)
	if len(log) != 1 {
		t.Fatalf("CommitLog() returned %d commits, want 1", len(log))
	}
	if log[0].ID != head {
		t.Fatalf("CommitLog()[0] = %s, want %s", log[0].ID, head)
	}
}

func TestCommitLogVisitsSharedAncestorOnce(t *testing.T) {
	root := mustHash(t, testHash1)
	left := mustHash(t, testHash2)
	right := mustHash(t, testHash3)
	merge := mustHash(t, testHash4)
	now := time.Unix(1_700_000_000, 0)

	repo := NewEmptyRepository()
	repo.head = merge
	repo.commitMap[root] = &Commit{ID: root, Committer: Signature{When: now.Add(-3 * time.Hour)}}
	repo.commitMap[left] = &Commit{ID: left, Parents: []Hash{root}, Committer: Signature{When: now.Add(-2 * time.Hour)}}
	repo.commitMap[right] = &Commit{ID: right, Parents: []Hash{root}, Committer: Signature{When: now.Add(-time.Hour)}}
	repo.commitMap[merge] = &Commit{ID: merge, Parents: []Hash{left, right}, Committer: Signature{When: now}}

	log := repo.CommitLog(0)
	if len(log) != 4 {
		t.Fatalf("CommitLog() returned %d commits, want 4", len(log))
	}

	rootCount := 0
	for _, commit := range log {
		if commit.ID == root {
			rootCount++
		}
	}
	if rootCount != 1 {
		t.Fatalf("CommitLog() returned shared ancestor %d times, want 1", rootCount)
	}
}
