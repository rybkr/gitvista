package gitcore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseMailmap(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected number of entries
	}{
		{
			name:    "empty",
			content: "",
			want:    0,
		},
		{
			name:    "comments and blank lines",
			content: "# This is a comment\n\n# Another comment\n",
			want:    0,
		},
		{
			name:    "form 1: name only",
			content: "Proper Name <commit@example.com>\n",
			want:    1,
		},
		{
			name:    "form 2: email only",
			content: "<proper@example.com> <commit@example.com>\n",
			want:    1,
		},
		{
			name:    "form 3: name and email, match on email",
			content: "Proper Name <proper@example.com> <commit@example.com>\n",
			want:    1,
		},
		{
			name:    "form 4: full replacement",
			content: "Proper Name <proper@example.com> Commit Name <commit@example.com>\n",
			want:    1,
		},
		{
			name: "multiple entries with comments",
			content: `# Mailmap
Proper Name <proper@example.com> <old@example.com>
<canonical@example.com> <other@example.com>
# end
`,
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := parseMailmap(tt.content)
			if len(m.entries) != tt.want {
				t.Errorf("parseMailmap() got %d entries, want %d", len(m.entries), tt.want)
			}
		})
	}
}

func TestParseMailmapLine_Forms(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantOK     bool
		wantProper string
		wantPEmail string
		wantCName  string
		wantCEmail string
	}{
		{
			name:       "form 1: name replacement",
			line:       "Joe Developer <joe@example.com>",
			wantOK:     true,
			wantProper: "Joe Developer",
			wantCEmail: "joe@example.com",
		},
		{
			name:       "form 2: email replacement",
			line:       "<proper@example.com> <old@example.com>",
			wantOK:     true,
			wantPEmail: "proper@example.com",
			wantCEmail: "old@example.com",
		},
		{
			name:       "form 3: name+email, match on email",
			line:       "Joe Developer <joe@proper.com> <joe@old.com>",
			wantOK:     true,
			wantProper: "Joe Developer",
			wantPEmail: "joe@proper.com",
			wantCEmail: "joe@old.com",
		},
		{
			name:       "form 4: full mapping",
			line:       "Joe Developer <joe@proper.com> Joseph Dev <joseph@old.com>",
			wantOK:     true,
			wantProper: "Joe Developer",
			wantPEmail: "joe@proper.com",
			wantCName:  "Joseph Dev",
			wantCEmail: "joseph@old.com",
		},
		{
			name:   "no email brackets",
			line:   "Just some text",
			wantOK: false,
		},
		{
			name:   "unclosed bracket",
			line:   "Name <email@example.com",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := parseMailmapLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("parseMailmapLine() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if entry.properName != tt.wantProper {
				t.Errorf("properName = %q, want %q", entry.properName, tt.wantProper)
			}
			if entry.properEmail != tt.wantPEmail {
				t.Errorf("properEmail = %q, want %q", entry.properEmail, tt.wantPEmail)
			}
			if entry.commitName != tt.wantCName {
				t.Errorf("commitName = %q, want %q", entry.commitName, tt.wantCName)
			}
			if entry.commitEmail != tt.wantCEmail {
				t.Errorf("commitEmail = %q, want %q", entry.commitEmail, tt.wantCEmail)
			}
		})
	}
}

func TestMailmap_Resolve(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		mailmap   string
		sig       Signature
		wantName  string
		wantEmail string
	}{
		{
			name:      "form 1: name replaced",
			mailmap:   "Proper Name <joe@example.com>",
			sig:       Signature{Name: "Old Name", Email: "joe@example.com", When: now},
			wantName:  "Proper Name",
			wantEmail: "joe@example.com",
		},
		{
			name:      "form 2: email replaced",
			mailmap:   "<proper@example.com> <old@example.com>",
			sig:       Signature{Name: "Joe", Email: "old@example.com", When: now},
			wantName:  "Joe",
			wantEmail: "proper@example.com",
		},
		{
			name:      "form 3: name and email replaced",
			mailmap:   "Joe Developer <joe@proper.com> <joe@old.com>",
			sig:       Signature{Name: "Joseph", Email: "joe@old.com", When: now},
			wantName:  "Joe Developer",
			wantEmail: "joe@proper.com",
		},
		{
			name:      "form 4: full mapping match",
			mailmap:   "Joe Developer <joe@proper.com> Joseph Dev <joseph@old.com>",
			sig:       Signature{Name: "Joseph Dev", Email: "joseph@old.com", When: now},
			wantName:  "Joe Developer",
			wantEmail: "joe@proper.com",
		},
		{
			name:      "form 4: name mismatch does not apply",
			mailmap:   "Joe Developer <joe@proper.com> Joseph Dev <joseph@old.com>",
			sig:       Signature{Name: "Wrong Name", Email: "joseph@old.com", When: now},
			wantName:  "Wrong Name",
			wantEmail: "joseph@old.com",
		},
		{
			name:      "case insensitive email match",
			mailmap:   "Proper <proper@example.com> <JOE@EXAMPLE.COM>",
			sig:       Signature{Name: "Joe", Email: "joe@example.com", When: now},
			wantName:  "Proper",
			wantEmail: "proper@example.com",
		},
		{
			name:      "case insensitive name match (form 4)",
			mailmap:   "Proper <proper@example.com> JOSEPH DEV <joe@old.com>",
			sig:       Signature{Name: "joseph dev", Email: "joe@old.com", When: now},
			wantName:  "Proper",
			wantEmail: "proper@example.com",
		},
		{
			name:      "unmatched passthrough",
			mailmap:   "Proper Name <proper@example.com> <other@example.com>",
			sig:       Signature{Name: "Unrelated", Email: "unrelated@example.com", When: now},
			wantName:  "Unrelated",
			wantEmail: "unrelated@example.com",
		},
		{
			name: "last match wins",
			mailmap: "First <first@example.com> <joe@old.com>\n" +
				"Last <last@example.com> <joe@old.com>",
			sig:       Signature{Name: "Joe", Email: "joe@old.com", When: now},
			wantName:  "Last",
			wantEmail: "last@example.com",
		},
		{
			name:      "nil mailmap",
			mailmap:   "",
			sig:       Signature{Name: "Joe", Email: "joe@example.com", When: now},
			wantName:  "Joe",
			wantEmail: "joe@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m *Mailmap
			if tt.mailmap != "" {
				m = parseMailmap(tt.mailmap)
			}
			sig := tt.sig
			m.resolve(&sig)
			if sig.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", sig.Name, tt.wantName)
			}
			if sig.Email != tt.wantEmail {
				t.Errorf("Email = %q, want %q", sig.Email, tt.wantEmail)
			}
		})
	}
}

func TestLoadMailmap_Integration(t *testing.T) {
	now := time.Now()
	workDir := t.TempDir()

	mailmapContent := "Canonical Name <canonical@example.com> <old@example.com>\n"
	if err := os.WriteFile(filepath.Join(workDir, ".mailmap"), []byte(mailmapContent), 0o644); err != nil {
		t.Fatal(err)
	}

	commit := &Commit{
		ID:        Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Author:    Signature{Name: "Old Author", Email: "old@example.com", When: now},
		Committer: Signature{Name: "Old Committer", Email: "old@example.com", When: now},
	}
	tag := &Tag{
		ID:     Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Tagger: Signature{Name: "Old Tagger", Email: "old@example.com", When: now},
	}

	repo := &Repository{
		workDir:   workDir,
		gitDir:    filepath.Join(workDir, ".git"),
		commits:   []*Commit{commit},
		commitMap: map[Hash]*Commit{commit.ID: commit},
		tags:      []*Tag{tag},
	}

	if err := repo.loadMailmap(); err != nil {
		t.Fatalf("loadMailmap() error: %v", err)
	}

	if commit.Author.Name != "Canonical Name" {
		t.Errorf("Author.Name = %q, want %q", commit.Author.Name, "Canonical Name")
	}
	if commit.Author.Email != "canonical@example.com" {
		t.Errorf("Author.Email = %q, want %q", commit.Author.Email, "canonical@example.com")
	}
	if commit.Committer.Name != "Canonical Name" {
		t.Errorf("Committer.Name = %q, want %q", commit.Committer.Name, "Canonical Name")
	}
	if commit.Committer.Email != "canonical@example.com" {
		t.Errorf("Committer.Email = %q, want %q", commit.Committer.Email, "canonical@example.com")
	}
	if tag.Tagger.Name != "Canonical Name" {
		t.Errorf("Tagger.Name = %q, want %q", tag.Tagger.Name, "Canonical Name")
	}
	if tag.Tagger.Email != "canonical@example.com" {
		t.Errorf("Tagger.Email = %q, want %q", tag.Tagger.Email, "canonical@example.com")
	}
}

func TestLoadMailmap_NoFile(t *testing.T) {
	workDir := t.TempDir()

	repo := &Repository{
		workDir:   workDir,
		gitDir:    filepath.Join(workDir, ".git"),
		commits:   []*Commit{},
		commitMap: make(map[Hash]*Commit),
		tags:      []*Tag{},
	}

	if err := repo.loadMailmap(); err != nil {
		t.Fatalf("loadMailmap() should be no-op without .mailmap, got error: %v", err)
	}

	if repo.mailmap != nil {
		t.Error("mailmap should be nil when .mailmap doesn't exist")
	}
}

func TestLoadMailmap_BareRepo(t *testing.T) {
	dir := t.TempDir()

	repo := &Repository{
		workDir: dir,
		gitDir:  dir, // bare: gitDir == workDir
	}

	if err := repo.loadMailmap(); err != nil {
		t.Fatalf("loadMailmap() should be no-op for bare repos, got error: %v", err)
	}

	if repo.mailmap != nil {
		t.Error("mailmap should be nil for bare repos")
	}
}
