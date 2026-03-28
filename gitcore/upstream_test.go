package gitcore

import (
	"encoding/json"
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
		wantStatus   UpstreamStatus
		wantReason   UpstreamReason
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

func TestUpstreamTypesJSON(t *testing.T) {
	statusJSON, err := json.Marshal(UpstreamStatusBehind)
	if err != nil {
		t.Fatalf("json.Marshal(UpstreamStatusBehind) error = %v", err)
	}
	if got := string(statusJSON); got != `"behind"` {
		t.Fatalf("json.Marshal(UpstreamStatusBehind) = %s, want %q", statusJSON, `"behind"`)
	}

	var status UpstreamStatus
	if err := json.Unmarshal([]byte(`"diverged"`), &status); err != nil {
		t.Fatalf("json.Unmarshal(UpstreamStatus) error = %v", err)
	}
	if status != UpstreamStatusDiverged {
		t.Fatalf("json.Unmarshal(UpstreamStatus) = %v, want %v", status, UpstreamStatusDiverged)
	}

	reasonJSON, err := json.Marshal(UpstreamReasonMissingRef)
	if err != nil {
		t.Fatalf("json.Marshal(UpstreamReasonMissingRef) error = %v", err)
	}
	if got := string(reasonJSON); got != `"missing_remote_ref"` {
		t.Fatalf("json.Marshal(UpstreamReasonMissingRef) = %s, want %q", reasonJSON, `"missing_remote_ref"`)
	}

	var reason UpstreamReason
	if err := json.Unmarshal([]byte(`"detached_head"`), &reason); err != nil {
		t.Fatalf("json.Unmarshal(UpstreamReason) error = %v", err)
	}
	if reason != UpstreamReasonDetachedHead {
		t.Fatalf("json.Unmarshal(UpstreamReason) = %v, want %v", reason, UpstreamReasonDetachedHead)
	}
}

func TestUpstreamTypesJSON_InvalidValues(t *testing.T) {
	if got := UpstreamStatus(-1).String(); got != "unknown" {
		t.Fatalf("UpstreamStatus(-1).String() = %q, want %q", got, "unknown")
	}
	if got := UpstreamReason(-1).String(); got != "unknown" {
		t.Fatalf("UpstreamReason(-1).String() = %q, want %q", got, "unknown")
	}

	if _, err := json.Marshal(UpstreamStatus(-1)); err == nil {
		t.Fatal("json.Marshal(UpstreamStatus(-1)) error = nil, want error")
	}
	if _, err := json.Marshal(UpstreamReason(-1)); err == nil {
		t.Fatal("json.Marshal(UpstreamReason(-1)) error = nil, want error")
	}

	var status UpstreamStatus
	if err := json.Unmarshal([]byte(`"not_a_status"`), &status); err == nil {
		t.Fatal("json.Unmarshal(invalid UpstreamStatus) error = nil, want error")
	}
	if err := json.Unmarshal([]byte(`1`), &status); err == nil {
		t.Fatal("json.Unmarshal(non-string UpstreamStatus) error = nil, want error")
	}

	var reason UpstreamReason
	if err := json.Unmarshal([]byte(`"not_a_reason"`), &reason); err == nil {
		t.Fatal("json.Unmarshal(invalid UpstreamReason) error = nil, want error")
	}
	if err := json.Unmarshal([]byte(`1`), &reason); err == nil {
		t.Fatal("json.Unmarshal(non-string UpstreamReason) error = nil, want error")
	}
}

func TestRepositoryCurrentBranchUpstream_GuardCases(t *testing.T) {
	base := &Commit{ID: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}
	remote := &Commit{ID: Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), Parents: []Hash{base.ID}}

	tests := []struct {
		name       string
		head       Hash
		headRef    string
		createConf bool
		config     string
		refs       map[string]Hash
		wantStatus UpstreamStatus
		wantReason UpstreamReason
		wantBranch string
		wantRef    string
	}{
		{
			name:       "empty head ref",
			head:       remote.ID,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonNoCurrentBranch,
		},
		{
			name:       "non branch head ref",
			head:       remote.ID,
			headRef:    "refs/tags/v1.0.0",
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonNoCurrentBranch,
		},
		{
			name:       "missing config file",
			head:       remote.ID,
			headRef:    "refs/heads/main",
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonNoUpstream,
		},
		{
			name:       "invalid merge ref format",
			head:       remote.ID,
			headRef:    "refs/heads/main",
			createConf: true,
			config: `[branch "main"]
	remote = origin
	merge = main`,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonNoUpstream,
		},
		{
			name:       "empty upstream hash treated as missing ref",
			head:       remote.ID,
			headRef:    "refs/heads/main",
			createConf: true,
			config: `[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			refs: map[string]Hash{
				"refs/remotes/origin/main": "",
			},
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonMissingRef,
			wantBranch: "origin/main",
			wantRef:    "refs/remotes/origin/main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir := t.TempDir()
			if tt.createConf {
				if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(tt.config), 0o644); err != nil {
					t.Fatalf("WriteFile(config): %v", err)
				}
			}

			repo := &Repository{
				gitDir:       gitDir,
				workDir:      gitDir,
				head:         tt.head,
				headRef:      tt.headRef,
				refs:         tt.refs,
				commits:      []*Commit{base, remote},
				commitMap:    map[Hash]*Commit{base.ID: base, remote.ID: remote},
				headDetached: false,
			}

			got := repo.CurrentBranchUpstream()
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
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

func TestClassifyUpstreamRelation(t *testing.T) {
	base := &Commit{ID: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}
	localAhead := &Commit{ID: Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), Parents: []Hash{base.ID}}
	upstreamAhead := &Commit{ID: Hash("cccccccccccccccccccccccccccccccccccccccc"), Parents: []Hash{base.ID}}
	localMerge := &Commit{ID: Hash("dddddddddddddddddddddddddddddddddddddddd"), Parents: []Hash{localAhead.ID, upstreamAhead.ID}}
	orphanLocal := &Commit{ID: Hash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")}
	orphanRemote := &Commit{ID: Hash("ffffffffffffffffffffffffffffffffffffffff")}

	repo := &Repository{
		commitMap: map[Hash]*Commit{
			base.ID:          base,
			localAhead.ID:    localAhead,
			upstreamAhead.ID: upstreamAhead,
			localMerge.ID:    localMerge,
			orphanLocal.ID:   orphanLocal,
			orphanRemote.ID:  orphanRemote,
		},
	}

	tests := []struct {
		name       string
		local      Hash
		upstream   Hash
		wantStatus UpstreamStatus
		wantAhead  int
		wantBehind int
		wantReason UpstreamReason
	}{
		{
			name:       "missing local hash",
			upstream:   upstreamAhead.ID,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonMissingRef,
		},
		{
			name:       "equal hashes",
			local:      base.ID,
			upstream:   base.ID,
			wantStatus: UpstreamStatusUpToDate,
		},
		{
			name:       "ahead",
			local:      localAhead.ID,
			upstream:   base.ID,
			wantStatus: UpstreamStatusAhead,
			wantAhead:  1,
		},
		{
			name:       "behind",
			local:      base.ID,
			upstream:   upstreamAhead.ID,
			wantStatus: UpstreamStatusBehind,
			wantBehind: 1,
		},
		{
			name:       "diverged",
			local:      localAhead.ID,
			upstream:   upstreamAhead.ID,
			wantStatus: UpstreamStatusDiverged,
			wantAhead:  1,
			wantBehind: 1,
		},
		{
			name:       "no common base",
			local:      orphanLocal.ID,
			upstream:   orphanRemote.ID,
			wantStatus: UpstreamStatusUnavailable,
			wantReason: UpstreamReasonNoCommonBase,
		},
		{
			name:       "merge commit ahead after integrating upstream",
			local:      localMerge.ID,
			upstream:   upstreamAhead.ID,
			wantStatus: UpstreamStatusAhead,
			wantAhead:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, ahead, behind, reason := repo.classifyUpstreamRelation(tt.local, tt.upstream)
			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
			if ahead != tt.wantAhead {
				t.Fatalf("ahead = %d, want %d", ahead, tt.wantAhead)
			}
			if behind != tt.wantBehind {
				t.Fatalf("behind = %d, want %d", behind, tt.wantBehind)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestCountExclusiveCommits(t *testing.T) {
	base := &Commit{ID: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}
	left1 := &Commit{ID: Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), Parents: []Hash{base.ID}}
	left2 := &Commit{ID: Hash("cccccccccccccccccccccccccccccccccccccccc"), Parents: []Hash{left1.ID}}
	right1 := &Commit{ID: Hash("dddddddddddddddddddddddddddddddddddddddd"), Parents: []Hash{base.ID}}
	merge := &Commit{ID: Hash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"), Parents: []Hash{left2.ID, right1.ID}}
	missingParent := &Commit{ID: Hash("ffffffffffffffffffffffffffffffffffffffff"), Parents: []Hash{Hash("9999999999999999999999999999999999999999")}}

	repo := &Repository{
		commitMap: map[Hash]*Commit{
			base.ID:          base,
			left1.ID:         left1,
			left2.ID:         left2,
			right1.ID:        right1,
			merge.ID:         merge,
			missingParent.ID: missingParent,
		},
	}

	tests := []struct {
		name    string
		include Hash
		exclude Hash
		want    int
	}{
		{name: "empty include", exclude: base.ID, want: 0},
		{name: "empty exclude", include: left2.ID, want: 0},
		{name: "include commit missing from map", include: Hash("9999999999999999999999999999999999999999"), exclude: base.ID, want: 0},
		{name: "exclude commit missing from map", include: left2.ID, exclude: Hash("9999999999999999999999999999999999999999"), want: 3},
		{name: "linear exclusive commits", include: left2.ID, exclude: base.ID, want: 2},
		{name: "merge counts unique commits only", include: merge.ID, exclude: right1.ID, want: 3},
		{name: "include merge deduplicates shared ancestors", include: merge.ID, exclude: Hash("9999999999999999999999999999999999999998"), want: 5},
		{name: "exclude merge deduplicates shared ancestors", include: left2.ID, exclude: merge.ID, want: 0},
		{name: "missing commit entry ignored", include: missingParent.ID, exclude: base.ID, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countExclusiveCommits(repo, tt.include, tt.exclude); got != tt.want {
				t.Fatalf("countExclusiveCommits(%q, %q) = %d, want %d", tt.include, tt.exclude, got, tt.want)
			}
		})
	}
}

func TestParseBranchTrackingFromConfig_IgnoresUnrelatedSectionsAndMalformedBranches(t *testing.T) {
	config := `[core]
	editor = vim

[branch "main"]
	remote = origin
	merge = refs/heads/main

[branch "feature"]
	remote = upstream

[remote "origin"]
	fetch = +refs/heads/*:refs/remotes/origin/*

[branch invalid]
	remote = should-not-apply

[branch "release"]
	merge = refs/heads/release
`

	got := parseBranchTrackingFromConfig(config)

	if got["main"].Remote != "origin" || got["main"].MergeRef != "refs/heads/main" {
		t.Fatalf("main tracking = %+v, want origin + refs/heads/main", got["main"])
	}
	if got["feature"].Remote != "upstream" || got["feature"].MergeRef != "" {
		t.Fatalf("feature tracking = %+v, want upstream + empty merge", got["feature"])
	}
	if got["release"].Remote != "" || got["release"].MergeRef != "refs/heads/release" {
		t.Fatalf("release tracking = %+v, want empty remote + refs/heads/release", got["release"])
	}
	if _, exists := got["invalid"]; exists {
		t.Fatalf("malformed branch header should be ignored: %+v", got["invalid"])
	}
}
