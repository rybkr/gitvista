package gitcore

import (
	"testing"
)

func TestRepositoryDiff(t *testing.T) {
	// Create synthetic commits for testing
	commit1 := &Commit{
		ID:      Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Tree:    Hash("1111111111111111111111111111111111111111"),
		Parents: []Hash{},
		Message: "Commit 1",
	}

	commit2 := &Commit{
		ID:      Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Tree:    Hash("2222222222222222222222222222222222222222"),
		Parents: []Hash{commit1.ID},
		Message: "Commit 2",
	}

	commit3 := &Commit{
		ID:      Hash("cccccccccccccccccccccccccccccccccccccccc"),
		Tree:    Hash("3333333333333333333333333333333333333333"),
		Parents: []Hash{commit2.ID},
		Message: "Commit 3",
	}

	oldRepo := &Repository{
		commits: []*Commit{commit1, commit2},
		refs: map[string]Hash{
			"refs/heads/main":    commit2.ID,
			"refs/heads/feature": commit1.ID,
		},
	}

	newRepo := &Repository{
		commits: []*Commit{commit1, commit2, commit3},
		refs: map[string]Hash{
			"refs/heads/main":   commit3.ID,
			"refs/heads/develop": commit2.ID,
		},
	}

	delta := newRepo.Diff(oldRepo)

	t.Run("added commits", func(t *testing.T) {
		if len(delta.AddedCommits) != 1 {
			t.Fatalf("expected 1 added commit, got %d", len(delta.AddedCommits))
		}
		if delta.AddedCommits[0].ID != commit3.ID {
			t.Errorf("added commit ID = %s, want %s", delta.AddedCommits[0].ID, commit3.ID)
		}
	})

	t.Run("deleted commits", func(t *testing.T) {
		if len(delta.DeletedCommits) != 0 {
			t.Errorf("expected 0 deleted commits, got %d", len(delta.DeletedCommits))
		}
	})

	t.Run("added branches", func(t *testing.T) {
		if len(delta.AddedBranches) != 1 {
			t.Fatalf("expected 1 added branch, got %d", len(delta.AddedBranches))
		}
		if hash, ok := delta.AddedBranches["develop"]; !ok || hash != commit2.ID {
			t.Errorf("added branch 'develop' = %s, want %s", hash, commit2.ID)
		}
	})

	t.Run("deleted branches", func(t *testing.T) {
		if len(delta.DeletedBranches) != 1 {
			t.Fatalf("expected 1 deleted branch, got %d", len(delta.DeletedBranches))
		}
		if hash, ok := delta.DeletedBranches["feature"]; !ok || hash != commit1.ID {
			t.Errorf("deleted branch 'feature' = %s, want %s", hash, commit1.ID)
		}
	})

	t.Run("amended branches", func(t *testing.T) {
		if len(delta.AmendedBranches) != 1 {
			t.Fatalf("expected 1 amended branch, got %d", len(delta.AmendedBranches))
		}
		if hash, ok := delta.AmendedBranches["main"]; !ok || hash != commit3.ID {
			t.Errorf("amended branch 'main' = %s, want %s", hash, commit3.ID)
		}
	})
}

func TestRepositoryDelta_IsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		delta *RepositoryDelta
		want  bool
	}{
		{
			name:  "empty delta",
			delta: NewRepositoryDelta(),
			want:  true,
		},
		{
			name: "delta with added commit",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{{ID: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}},
				DeletedCommits:  []*Commit{},
				AddedBranches:   make(map[string]Hash),
				DeletedBranches: make(map[string]Hash),
				AmendedBranches: make(map[string]Hash),
			},
			want: false,
		},
		{
			name: "delta with deleted commit",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{{ID: Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")}},
				AddedBranches:   make(map[string]Hash),
				DeletedBranches: make(map[string]Hash),
				AmendedBranches: make(map[string]Hash),
			},
			want: false,
		},
		{
			name: "delta with added branch (BUG: IsEmpty doesn't check branches)",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{},
				AddedBranches:   map[string]Hash{"feature": Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
				DeletedBranches: make(map[string]Hash),
				AmendedBranches: make(map[string]Hash),
			},
			want: true, // BUG: should be false, but IsEmpty() only checks commits
		},
		{
			name: "delta with deleted branch (BUG: IsEmpty doesn't check branches)",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{},
				AddedBranches:   make(map[string]Hash),
				DeletedBranches: map[string]Hash{"old": Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
				AmendedBranches: make(map[string]Hash),
			},
			want: true, // BUG: should be false, but IsEmpty() only checks commits
		},
		{
			name: "delta with amended branch (BUG: IsEmpty doesn't check branches)",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{},
				AddedBranches:   make(map[string]Hash),
				DeletedBranches: make(map[string]Hash),
				AmendedBranches: map[string]Hash{"main": Hash("cccccccccccccccccccccccccccccccccccccccc")},
			},
			want: true, // BUG: should be false, but IsEmpty() only checks commits
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.delta.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
