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
		commits:   []*Commit{commit1, commit2},
		commitMap: map[Hash]*Commit{commit1.ID: commit1, commit2.ID: commit2},
		refs: map[string]Hash{
			"refs/heads/main":    commit2.ID,
			"refs/heads/feature": commit1.ID,
		},
	}

	newRepo := &Repository{
		commits:   []*Commit{commit1, commit2, commit3},
		commitMap: map[Hash]*Commit{commit1.ID: commit1, commit2.ID: commit2, commit3.ID: commit3},
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

func TestBuildGraphSummary(t *testing.T) {
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

	tagObj := &Tag{
		ID:     Hash("dddddddddddddddddddddddddddddddddddddddd"),
		Object: commit1.ID,
		Name:   "v1.0",
	}

	repo := &Repository{
		head:      commit3.ID,
		commits:   []*Commit{commit1, commit2, commit3},
		commitMap: map[Hash]*Commit{commit1.ID: commit1, commit2.ID: commit2, commit3.ID: commit3},
		refs: map[string]Hash{
			"refs/heads/main": commit3.ID,
			"refs/tags/v1.0":  tagObj.ID,
		},
		tags:    []*Tag{tagObj},
		stashes: []*StashEntry{{Hash: commit1.ID, Message: "WIP"}},
	}

	summary := repo.BuildGraphSummary()

	t.Run("total commits", func(t *testing.T) {
		if summary.TotalCommits != 3 {
			t.Errorf("TotalCommits = %d, want 3", summary.TotalCommits)
		}
	})

	t.Run("skeleton count matches", func(t *testing.T) {
		if len(summary.Skeleton) != 3 {
			t.Fatalf("len(Skeleton) = %d, want 3", len(summary.Skeleton))
		}
	})

	t.Run("skeleton data", func(t *testing.T) {
		found := make(map[Hash]CommitSkeleton)
		for _, s := range summary.Skeleton {
			found[s.Hash] = s
		}

		s1, ok := found[commit1.ID]
		if !ok {
			t.Fatal("skeleton missing commit1")
		}
		if len(s1.Parents) != 0 {
			t.Errorf("commit1 parents = %d, want 0", len(s1.Parents))
		}
		if s1.Timestamp != commit1.Committer.When.Unix() {
			t.Errorf("commit1 timestamp = %d, want %d", s1.Timestamp, commit1.Committer.When.Unix())
		}

		s2, ok := found[commit2.ID]
		if !ok {
			t.Fatal("skeleton missing commit2")
		}
		if len(s2.Parents) != 1 || s2.Parents[0] != commit1.ID {
			t.Errorf("commit2 parents = %v, want [%s]", s2.Parents, commit1.ID)
		}
	})

	t.Run("time range", func(t *testing.T) {
		if summary.OldestTimestamp != commit1.Committer.When.Unix() {
			t.Errorf("OldestTimestamp = %d, want %d", summary.OldestTimestamp, commit1.Committer.When.Unix())
		}
		if summary.NewestTimestamp != commit3.Committer.When.Unix() {
			t.Errorf("NewestTimestamp = %d, want %d", summary.NewestTimestamp, commit3.Committer.When.Unix())
		}
	})

	t.Run("branches", func(t *testing.T) {
		if len(summary.Branches) != 1 {
			t.Fatalf("len(Branches) = %d, want 1", len(summary.Branches))
		}
		if summary.Branches["main"] != commit3.ID {
			t.Errorf("Branches[main] = %s, want %s", summary.Branches["main"], commit3.ID)
		}
	})

	t.Run("tags", func(t *testing.T) {
		if len(summary.Tags) != 1 {
			t.Fatalf("len(Tags) = %d, want 1", len(summary.Tags))
		}
		// Annotated tag should be peeled to the commit
		if summary.Tags["v1.0"] != string(commit1.ID) {
			t.Errorf("Tags[v1.0] = %s, want %s", summary.Tags["v1.0"], commit1.ID)
		}
	})

	t.Run("head hash", func(t *testing.T) {
		if summary.HeadHash != string(commit3.ID) {
			t.Errorf("HeadHash = %s, want %s", summary.HeadHash, commit3.ID)
		}
	})

	t.Run("stashes", func(t *testing.T) {
		if len(summary.Stashes) != 1 {
			t.Fatalf("len(Stashes) = %d, want 1", len(summary.Stashes))
		}
		if summary.Stashes[0].Message != "WIP" {
			t.Errorf("Stashes[0].Message = %q, want %q", summary.Stashes[0].Message, "WIP")
		}
	})
}

func TestBuildGraphSummary_Empty(t *testing.T) {
	repo := NewEmptyRepository()
	summary := repo.BuildGraphSummary()

	if summary.TotalCommits != 0 {
		t.Errorf("TotalCommits = %d, want 0", summary.TotalCommits)
	}
	if len(summary.Skeleton) != 0 {
		t.Errorf("len(Skeleton) = %d, want 0", len(summary.Skeleton))
	}
	if summary.HeadHash != "" {
		t.Errorf("HeadHash = %q, want empty", summary.HeadHash)
	}
	if summary.OldestTimestamp != 0 {
		t.Errorf("OldestTimestamp = %d, want 0", summary.OldestTimestamp)
	}
	if summary.NewestTimestamp != 0 {
		t.Errorf("NewestTimestamp = %d, want 0", summary.NewestTimestamp)
	}
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
