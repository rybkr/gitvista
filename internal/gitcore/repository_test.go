package gitcore

import (
	"testing"
	"time"
)

func TestRepositoryDiff(t *testing.T) {
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
			"refs/heads/main":    commit3.ID,
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
			name: "delta with added branch",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{},
				AddedBranches:   map[string]Hash{"feature": Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
				DeletedBranches: make(map[string]Hash),
				AmendedBranches: make(map[string]Hash),
			},
			want: false,
		},
		{
			name: "delta with deleted branch",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{},
				AddedBranches:   make(map[string]Hash),
				DeletedBranches: map[string]Hash{"old": Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
				AmendedBranches: make(map[string]Hash),
			},
			want: false,
		},
		{
			name: "delta with amended branch",
			delta: &RepositoryDelta{
				AddedCommits:    []*Commit{},
				DeletedCommits:  []*Commit{},
				AddedBranches:   make(map[string]Hash),
				DeletedBranches: make(map[string]Hash),
				AmendedBranches: map[string]Hash{"main": Hash("cccccccccccccccccccccccccccccccccccccccc")},
			},
			want: false,
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

	repo := &Repository{
		commits: []*Commit{
			{ID: hash1, Message: "first"},
			{ID: hash2, Message: "second"},
		},
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
		head:    commit3.ID,
		commits: []*Commit{commit1, commit2, commit3},
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
		emptyRepo := &Repository{}
		log := emptyRepo.CommitLog(0)
		if log != nil {
			t.Errorf("CommitLog() on empty repo = %v, want nil", log)
		}
	})
}
