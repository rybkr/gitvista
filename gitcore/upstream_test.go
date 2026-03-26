package gitcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBranchTrackingFromConfig(t *testing.T) {
	config := `[branch "main"]
	remote = origin
	merge = refs/heads/main

[branch "feature"]
	remote = upstream
	merge = refs/heads/feature/demo
`

	got := parseBranchTrackingFromConfig(config)
	if got["main"].Remote != "origin" || got["main"].MergeRef != "refs/heads/main" {
		t.Fatalf("main tracking = %+v, want origin + refs/heads/main", got["main"])
	}
	if got["feature"].Remote != "upstream" || got["feature"].MergeRef != "refs/heads/feature/demo" {
		t.Fatalf("feature tracking = %+v, want upstream + refs/heads/feature/demo", got["feature"])
	}
}

func TestRepositoryCurrentBranchUpstream(t *testing.T) {
	base := &Commit{ID: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}
	ahead := &Commit{ID: Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), Parents: []Hash{base.ID}}
	divergedLocal := &Commit{ID: Hash("cccccccccccccccccccccccccccccccccccccccc"), Parents: []Hash{base.ID}}

	tests := []struct {
		name         string
		head         Hash
		headRef      string
		headDetached bool
		refs         map[string]Hash
		config       string
		wantStatus   string
		wantReason   string
		wantAhead    int
		wantBehind   int
		wantBranch   string
		wantRef      string
	}{
		{
			name:    "up to date",
			head:    ahead.ID,
			headRef: "refs/heads/main",
			refs: map[string]Hash{
				"refs/heads/main":          ahead.ID,
				"refs/remotes/origin/main": ahead.ID,
			},
			config: `[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			wantStatus: UpstreamStatusUpToDate,
			wantBranch: "origin/main",
			wantRef:    "refs/remotes/origin/main",
		},
		{
			name:    "ahead",
			head:    ahead.ID,
			headRef: "refs/heads/main",
			refs: map[string]Hash{
				"refs/heads/main":          ahead.ID,
				"refs/remotes/origin/main": base.ID,
			},
			config: `[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			wantStatus: UpstreamStatusAhead,
			wantAhead:  1,
			wantBranch: "origin/main",
			wantRef:    "refs/remotes/origin/main",
		},
		{
			name:    "behind",
			head:    base.ID,
			headRef: "refs/heads/main",
			refs: map[string]Hash{
				"refs/heads/main":          base.ID,
				"refs/remotes/origin/main": ahead.ID,
			},
			config: `[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			wantStatus: UpstreamStatusBehind,
			wantBehind: 1,
			wantBranch: "origin/main",
			wantRef:    "refs/remotes/origin/main",
		},
		{
			name:    "diverged",
			head:    divergedLocal.ID,
			headRef: "refs/heads/main",
			refs: map[string]Hash{
				"refs/heads/main":          divergedLocal.ID,
				"refs/remotes/origin/main": ahead.ID,
			},
			config: `[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			wantStatus: UpstreamStatusDiverged,
			wantAhead:  1,
			wantBehind: 1,
			wantBranch: "origin/main",
			wantRef:    "refs/remotes/origin/main",
		},
		{
			name:    "no upstream config",
			head:    ahead.ID,
			headRef: "refs/heads/main",
			refs: map[string]Hash{
				"refs/heads/main": ahead.ID,
			},
			config:     `[core]`,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonNoUpstream,
		},
		{
			name:    "missing remote ref",
			head:    ahead.ID,
			headRef: "refs/heads/main",
			refs: map[string]Hash{
				"refs/heads/main": ahead.ID,
			},
			config: `[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonMissingRef,
			wantBranch: "origin/main",
			wantRef:    "refs/remotes/origin/main",
		},
		{
			name:         "detached head",
			head:         ahead.ID,
			headDetached: true,
			refs: map[string]Hash{
				"refs/remotes/origin/main": ahead.ID,
			},
			config:     `[branch "main"]`,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonDetachedHead,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(tt.config), 0o644); err != nil {
				t.Fatalf("WriteFile(config): %v", err)
			}

			repo := &Repository{
				gitDir:       gitDir,
				workDir:      gitDir,
				head:         tt.head,
				headRef:      tt.headRef,
				headDetached: tt.headDetached,
				refs:         tt.refs,
				commits:      []*Commit{base, ahead, divergedLocal},
				commitMap: map[Hash]*Commit{
					base.ID:          base,
					ahead.ID:         ahead,
					divergedLocal.ID: divergedLocal,
				},
			}

			got := repo.CurrentBranchUpstream()
			if got == nil {
				t.Fatal("CurrentBranchUpstream() = nil")
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.AheadCount != tt.wantAhead {
				t.Fatalf("AheadCount = %d, want %d", got.AheadCount, tt.wantAhead)
			}
			if got.BehindCount != tt.wantBehind {
				t.Fatalf("BehindCount = %d, want %d", got.BehindCount, tt.wantBehind)
			}
			if got.BranchName != tt.wantBranch {
				t.Fatalf("BranchName = %q, want %q", got.BranchName, tt.wantBranch)
			}
			if got.Ref != tt.wantRef {
				t.Fatalf("Ref = %q, want %q", got.Ref, tt.wantRef)
			}
		})
	}
}

func TestRepositoryCurrentBranchUpstream_MergedUpstreamAheadCount(t *testing.T) {
	base := &Commit{ID: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}
	feature := &Commit{ID: Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), Parents: []Hash{base.ID}}
	main := &Commit{ID: Hash("cccccccccccccccccccccccccccccccccccccccc"), Parents: []Hash{base.ID}}
	merge := &Commit{ID: Hash("dddddddddddddddddddddddddddddddddddddddd"), Parents: []Hash{feature.ID, main.ID}}

	gitDir := t.TempDir()
	config := `[branch "feature"]
	remote = origin
	merge = refs/heads/main`
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	repo := &Repository{
		gitDir:  gitDir,
		workDir: gitDir,
		head:    merge.ID,
		headRef: "refs/heads/feature",
		refs: map[string]Hash{
			"refs/heads/feature":       merge.ID,
			"refs/heads/main":          main.ID,
			"refs/remotes/origin/main": main.ID,
		},
		commits: []*Commit{base, feature, main, merge},
		commitMap: map[Hash]*Commit{
			base.ID:    base,
			feature.ID: feature,
			main.ID:    main,
			merge.ID:   merge,
		},
	}

	got := repo.CurrentBranchUpstream()
	if got == nil {
		t.Fatal("CurrentBranchUpstream() = nil")
	}
	if got.Status != UpstreamStatusAhead {
		t.Fatalf("Status = %q, want %q", got.Status, UpstreamStatusAhead)
	}
	if got.AheadCount != 2 || got.BehindCount != 0 {
		t.Fatalf("Ahead/Behind = %d/%d, want 2/0", got.AheadCount, got.BehindCount)
	}
}
