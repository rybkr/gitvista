package gitcore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindGitDirectory_BareRepo(t *testing.T) {
	bareDir := t.TempDir()

	// Create bare repo structure: objects/, refs/, HEAD
	for _, dir := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(bareDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bareDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitDir, workDir, err := findGitDirectory(bareDir)
	if err != nil {
		t.Fatalf("findGitDirectory() error: %v", err)
	}
	if gitDir != bareDir {
		t.Errorf("gitDir = %q, want %q", gitDir, bareDir)
	}
	if workDir != bareDir {
		t.Errorf("workDir = %q, want %q (bare repo: gitDir == workDir)", workDir, bareDir)
	}
}

func TestFindGitDirectory_NonBareNotMisidentified(t *testing.T) {
	workDir := t.TempDir()
	dotGit := filepath.Join(workDir, ".git")

	// Create normal repo structure with .git/
	for _, dir := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(dotGit, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dotGit, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitDir, gotWorkDir, err := findGitDirectory(workDir)
	if err != nil {
		t.Fatalf("findGitDirectory() error: %v", err)
	}
	if gitDir != dotGit {
		t.Errorf("gitDir = %q, want %q", gitDir, dotGit)
	}
	if gotWorkDir != workDir {
		t.Errorf("workDir = %q, want %q", gotWorkDir, workDir)
	}
}

func TestIsBareRepository_MissingComponent(t *testing.T) {
	// Create directory with objects/ and refs/ but no HEAD
	dir := t.TempDir()
	for _, sub := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if isBareRepository(dir) {
		t.Error("isBareRepository() = true, want false (HEAD is missing)")
	}
}

func TestRepositoryGraphBranches(t *testing.T) {
	repo := &Repository{
		refs: map[string]Hash{
			"refs/heads/main":            Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			"refs/remotes/origin/main":   Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			"refs/remotes/upstream/feat": Hash("cccccccccccccccccccccccccccccccccccccccc"),
			"refs/tags/v1.0":             Hash("dddddddddddddddddddddddddddddddddddddddd"),
		},
	}

	got := repo.GraphBranches()

	if len(got) != 3 {
		t.Fatalf("len(GraphBranches) = %d, want 3", len(got))
	}
	if got["refs/heads/main"] == "" {
		t.Error("GraphBranches missing refs/heads/main")
	}
	if got["refs/remotes/origin/main"] == "" {
		t.Error("GraphBranches missing refs/remotes/origin/main")
	}
	if got["refs/remotes/upstream/feat"] == "" {
		t.Error("GraphBranches missing refs/remotes/upstream/feat")
	}
	if _, ok := got["refs/tags/v1.0"]; ok {
		t.Error("GraphBranches unexpectedly included tag ref")
	}
}

func TestRepository_Head(t *testing.T) {
	repo := &Repository{
		head: Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	}

	got := repo.Head()
	want := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	if got != want {
		t.Errorf("Head() = %s, want %s", got, want)
	}
}

func TestRepository_HeadRef(t *testing.T) {
	tests := []struct {
		name    string
		headRef string
		want    string
	}{
		{
			name:    "branch HEAD",
			headRef: "refs/heads/main",
			want:    "refs/heads/main",
		},
		{
			name:    "detached HEAD",
			headRef: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{
				headRef: tt.headRef,
			}

			got := repo.HeadRef()
			if got != tt.want {
				t.Errorf("HeadRef() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRepository_HeadDetached(t *testing.T) {
	tests := []struct {
		name         string
		headDetached bool
		want         bool
	}{
		{
			name:         "detached HEAD",
			headDetached: true,
			want:         true,
		},
		{
			name:         "branch HEAD",
			headDetached: false,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{
				headDetached: tt.headDetached,
			}

			got := repo.HeadDetached()
			if got != tt.want {
				t.Errorf("HeadDetached() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRepository_TagNames(t *testing.T) {
	repo := &Repository{
		refs: map[string]Hash{
			"refs/heads/main":    Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			"refs/tags/v1.0.0":   Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			"refs/tags/v2.0.0":   Hash("cccccccccccccccccccccccccccccccccccccccc"),
			"refs/heads/develop": Hash("dddddddddddddddddddddddddddddddddddddddd"),
		},
	}

	got := repo.TagNames()

	if len(got) != 2 {
		t.Fatalf("TagNames() returned %d tags, want 2", len(got))
	}

	// Check that both tags are present (order may vary)
	foundV1 := false
	foundV2 := false
	for _, tag := range got {
		if tag == "v1.0.0" {
			foundV1 = true
		}
		if tag == "v2.0.0" {
			foundV2 = true
		}
	}

	if !foundV1 {
		t.Errorf("TagNames() missing v1.0.0")
	}
	if !foundV2 {
		t.Errorf("TagNames() missing v2.0.0")
	}
}

func TestParseRemotesFromConfig(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   map[string]string
	}{
		{
			name: "single remote",
			config: `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://github.com/user/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main`,
			want: map[string]string{
				"origin": "https://github.com/user/repo.git",
			},
		},
		{
			name: "multiple remotes",
			config: `[remote "origin"]
	url = https://github.com/user/repo.git
[remote "upstream"]
	url = git@github.com:upstream/repo.git`,
			want: map[string]string{
				"origin":   "https://github.com/user/repo.git",
				"upstream": "git@github.com:upstream/repo.git",
			},
		},
		{ //nolint:gosec // G101: Test data, not actual credentials
			name: "credentials stripped",
			config: `[remote "origin"]
	url = https://user:token@github.com/user/repo.git`,
			want: map[string]string{
				"origin": "https://github.com/user/repo.git",
			},
		},
		{
			name: "no remotes",
			config: `[core]
	repositoryformatversion = 0`,
			want: map[string]string{},
		},
		{
			name: "SSH URL preserved",
			config: `[remote "origin"]
	url = git@github.com:user/repo.git`,
			want: map[string]string{
				"origin": "git@github.com:user/repo.git",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemotesFromConfig(tt.config)

			if len(got) != len(tt.want) {
				t.Fatalf("parseRemotesFromConfig() returned %d remotes, want %d", len(got), len(tt.want))
			}

			for name, wantURL := range tt.want {
				gotURL, ok := got[name]
				if !ok {
					t.Errorf("parseRemotesFromConfig() missing remote %q", name)
					continue
				}
				if gotURL != wantURL {
					t.Errorf("parseRemotesFromConfig() remote %q = %q, want %q", name, gotURL, wantURL)
				}
			}
		})
	}
}

func TestStripCredentials(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{ //nolint:gosec // G101: Test data, not actual credentials
			name: "HTTPS with credentials",
			url:  "https://user:token@github.com/user/repo.git",
			want: "https://github.com/user/repo.git",
		},
		{
			name: "HTTPS without credentials",
			url:  "https://github.com/user/repo.git",
			want: "https://github.com/user/repo.git",
		},
		{
			name: "SSH URL",
			url:  "git@github.com:user/repo.git",
			want: "git@github.com:user/repo.git",
		},
		{ //nolint:gosec // G101: Test data, not actual credentials
			name: "HTTP with credentials",
			url:  "http://user:token@example.com/repo.git",
			want: "http://example.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCredentials(tt.url)
			if got != tt.want {
				t.Errorf("stripCredentials() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseBranchTrackingFromConfig(t *testing.T) {
	config := `[core]
	repositoryformatversion = 0
[branch "main"]
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

func TestNewSignature_Timezone(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		wantName       string
		wantTZ         string
		wantOffsetSecs int
	}{
		{
			name:           "positive offset",
			line:           "John Doe <john@example.com> 1234567890 +0530",
			wantName:       "John Doe",
			wantTZ:         "+0530",
			wantOffsetSecs: 5*3600 + 30*60,
		},
		{
			name:           "negative offset",
			line:           "Jane Doe <jane@example.com> 1234567890 -0800",
			wantName:       "Jane Doe",
			wantTZ:         "-0800",
			wantOffsetSecs: -8 * 3600,
		},
		{
			name:           "UTC offset",
			line:           "Test User <test@example.com> 1234567890 +0000",
			wantName:       "Test User",
			wantTZ:         "+0000",
			wantOffsetSecs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, err := NewSignature(tt.line)
			if err != nil {
				t.Fatalf("NewSignature() error: %v", err)
			}
			if sig.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", sig.Name, tt.wantName)
			}
			zoneName, offset := sig.When.Zone()
			if offset != tt.wantOffsetSecs {
				t.Errorf("timezone offset = %d, want %d", offset, tt.wantOffsetSecs)
			}
			if zoneName != tt.wantTZ {
				t.Errorf("timezone name = %q, want %q", zoneName, tt.wantTZ)
			}
		})
	}
}

func TestRepository_GetCommit(t *testing.T) {
	hash1 := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hash2 := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	c1 := &Commit{ID: hash1, Message: "first"}
	c2 := &Commit{ID: hash2, Message: "second"}
	repo := &Repository{
		commits:   []*Commit{c1, c2},
		commitMap: map[Hash]*Commit{hash1: c1, hash2: c2},
	}

	t.Run("found", func(t *testing.T) {
		c, err := repo.GetCommit(hash1)
		if err != nil {
			t.Fatalf("GetCommit() error: %v", err)
		}
		if c.Message != "first" {
			t.Errorf("Message = %q, want %q", c.Message, "first")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetCommit(Hash("cccccccccccccccccccccccccccccccccccccccc"))
		if err == nil {
			t.Fatal("GetCommit() expected error for missing commit")
		}
	})
}

func TestRepository_GetTag(t *testing.T) {
	hash1 := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	repo := &Repository{
		tags: []*Tag{
			{ID: hash1, Name: "v1.0"},
		},
	}

	t.Run("found", func(t *testing.T) {
		tag, err := repo.GetTag(hash1)
		if err != nil {
			t.Fatalf("GetTag() error: %v", err)
		}
		if tag.Name != "v1.0" {
			t.Errorf("Name = %q, want %q", tag.Name, "v1.0")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetTag(Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
		if err == nil {
			t.Fatal("GetTag() expected error for missing tag")
		}
	})
}

func TestGetCommits(t *testing.T) {
	hash1 := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hash2 := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hash3 := Hash("cccccccccccccccccccccccccccccccccccccccc")

	c1 := &Commit{ID: hash1, Message: "first"}
	c2 := &Commit{ID: hash2, Message: "second"}

	repo := &Repository{
		commits:   []*Commit{c1, c2},
		commitMap: map[Hash]*Commit{hash1: c1, hash2: c2},
	}

	t.Run("found all", func(t *testing.T) {
		result := repo.GetCommits([]Hash{hash1, hash2})
		if len(result) != 2 {
			t.Fatalf("GetCommits() returned %d, want 2", len(result))
		}
		if result[0] == c1 {
			t.Fatal("GetCommits() returned original commit pointer")
		}
	})

	t.Run("skips unknown", func(t *testing.T) {
		result := repo.GetCommits([]Hash{hash1, hash3})
		if len(result) != 1 {
			t.Fatalf("GetCommits() returned %d, want 1", len(result))
		}
		if result[0].ID != hash1 {
			t.Errorf("result[0].ID = %s, want %s", result[0].ID, hash1)
		}
	})

	t.Run("all unknown", func(t *testing.T) {
		result := repo.GetCommits([]Hash{hash3})
		if len(result) != 0 {
			t.Errorf("GetCommits() returned %d, want 0", len(result))
		}
	})
}

func TestGetCommits_Empty(t *testing.T) {
	hash1 := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	c1 := &Commit{ID: hash1, Message: "first"}
	repo := &Repository{
		commits:   []*Commit{c1},
		commitMap: map[Hash]*Commit{hash1: c1},
	}

	result := repo.GetCommits([]Hash{})
	if len(result) != 0 {
		t.Errorf("GetCommits(empty) returned %d, want 0", len(result))
	}
}

func TestRepository_CommitLog(t *testing.T) {
	now := time.Now()

	commit1 := &Commit{
		ID:        Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Parents:   []Hash{},
		Committer: Signature{When: now.Add(-2 * time.Hour)},
		Message:   "first",
	}
	commit2 := &Commit{
		ID:        Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Parents:   []Hash{commit1.ID},
		Committer: Signature{When: now.Add(-1 * time.Hour)},
		Message:   "second",
	}
	commit3 := &Commit{
		ID:        Hash("cccccccccccccccccccccccccccccccccccccccc"),
		Parents:   []Hash{commit2.ID},
		Committer: Signature{When: now},
		Message:   "third",
	}

	repo := &Repository{
		head:      commit3.ID,
		commits:   []*Commit{commit1, commit2, commit3},
		commitMap: map[Hash]*Commit{commit1.ID: commit1, commit2.ID: commit2, commit3.ID: commit3},
	}

	t.Run("all commits", func(t *testing.T) {
		log := repo.CommitLog(0)
		if len(log) != 3 {
			t.Fatalf("CommitLog(0) returned %d commits, want 3", len(log))
		}
		if log[0].ID != commit3.ID {
			t.Errorf("first commit = %s, want %s", log[0].ID, commit3.ID)
		}
		if log[1].ID != commit2.ID {
			t.Errorf("second commit = %s, want %s", log[1].ID, commit2.ID)
		}
		if log[2].ID != commit1.ID {
			t.Errorf("third commit = %s, want %s", log[2].ID, commit1.ID)
		}
	})

	t.Run("limited count", func(t *testing.T) {
		log := repo.CommitLog(2)
		if len(log) != 2 {
			t.Fatalf("CommitLog(2) returned %d commits, want 2", len(log))
		}
		if log[0].ID != commit3.ID {
			t.Errorf("first commit = %s, want %s", log[0].ID, commit3.ID)
		}
	})

	t.Run("empty head", func(t *testing.T) {
		emptyRepo := NewEmptyRepository()
		log := emptyRepo.CommitLog(0)
		if log != nil {
			t.Errorf("CommitLog() on empty repo = %v, want nil", log)
		}
	})
}
