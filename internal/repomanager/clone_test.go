package repomanager

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "HTTPS basic",
			input: "https://github.com/user/repo.git",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "HTTPS uppercase host",
			input: "https://GitHub.COM/User/Repo.git",
			want:  "https://github.com/User/Repo",
		},
		{
			name:  "HTTPS trailing slash",
			input: "https://github.com/user/repo/",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "HTTPS no .git suffix",
			input: "https://github.com/user/repo",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "SSH shorthand",
			input: "git@github.com:user/repo.git",
			want:  "ssh://github.com/user/repo",
		},
		{
			name:  "SSH shorthand no .git",
			input: "git@github.com:user/repo",
			want:  "ssh://github.com/user/repo",
		},
		{
			name:  "SSH scheme URL",
			input: "ssh://git@github.com/user/repo.git",
			want:  "ssh://github.com/user/repo",
		},
		{ //nolint:gosec // G101: Test data, not actual credentials
			name:  "HTTPS with credentials stripped",
			input: "https://user:token@github.com/user/repo.git",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "HTTP scheme",
			input: "http://example.com/user/repo.git",
			want:  "http://example.com/user/repo",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "file scheme rejected",
			input:   "file:///path/to/repo",
			wantErr: true,
		},
		{
			name:    "git scheme rejected",
			input:   "git://github.com/user/repo.git",
			wantErr: true,
		},
		{
			name:    "option injection rejected",
			input:   "--upload-pack=malicious",
			wantErr: true,
		},
		{
			name:    "localhost rejected (SSRF)",
			input:   "https://localhost/repo",
			wantErr: true,
		},
		{
			name:    "loopback IP rejected (SSRF)",
			input:   "https://127.0.0.1/repo",
			wantErr: true,
		},
		{
			name:    "metadata endpoint rejected (SSRF)",
			input:   "https://metadata.google.internal/computeMetadata/v1/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("normalizeURL(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeURL(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeURL_Deduplication(t *testing.T) {
	// Verify that equivalent URLs normalize to the same string
	urls := []string{
		"https://github.com/user/repo.git",
		"https://GitHub.COM/user/repo.git",
		"https://github.com/user/repo",
		"https://github.com/user/repo/",
	}

	first, err := normalizeURL(urls[0])
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range urls[1:] {
		got, err := normalizeURL(u)
		if err != nil {
			t.Fatalf("normalizeURL(%q) error: %v", u, err)
		}
		if got != first {
			t.Errorf("normalizeURL(%q) = %q, want %q (same as first URL)", u, got, first)
		}
	}
}

func TestHashURL(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		h1 := hashURL("https://github.com/user/repo")
		h2 := hashURL("https://github.com/user/repo")
		if h1 != h2 {
			t.Errorf("hashURL not deterministic: %q != %q", h1, h2)
		}
	})

	t.Run("length", func(t *testing.T) {
		h := hashURL("https://github.com/user/repo")
		if len(h) != 16 {
			t.Errorf("hashURL length = %d, want 16", len(h))
		}
	})

	t.Run("unique for different inputs", func(t *testing.T) {
		h1 := hashURL("https://github.com/user/repo1")
		h2 := hashURL("https://github.com/user/repo2")
		if h1 == h2 {
			t.Errorf("hashURL returned same hash for different URLs: %q", h1)
		}
	})
}

func TestCloneRepo_InvalidURL(t *testing.T) {
	destPath := t.TempDir() + "/should-not-exist"

	err := cloneRepo(context.Background(), "https://invalid.invalid/no/such/repo.git", destPath, 10*time.Second, nil)
	if err == nil {
		t.Fatal("cloneRepo() with invalid URL should return error")
	}

	// Verify cleanup happened
	if _, statErr := os.Stat(destPath); statErr == nil {
		t.Error("destPath should have been cleaned up after failed clone")
	}
}

func TestParseProgressLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		phase   string
		percent int
	}{
		{
			name:    "receiving objects",
			input:   "Receiving objects:  45% (123/456)",
			wantOK:  true,
			phase:   "Receiving objects",
			percent: 45,
		},
		{
			name:    "resolving deltas",
			input:   "Resolving deltas: 100% (789/789), done.",
			wantOK:  true,
			phase:   "Resolving deltas",
			percent: 100,
		},
		{
			name:    "counting objects",
			input:   "Counting objects: 12% (5/42)",
			wantOK:  true,
			phase:   "Counting objects",
			percent: 12,
		},
		{
			name:   "no match plain text",
			input:  "Cloning into bare repository...",
			wantOK: false,
		},
		{
			name:   "empty string",
			input:  "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := parseProgressLine(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseProgressLine(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if p.Phase != tt.phase {
				t.Errorf("phase = %q, want %q", p.Phase, tt.phase)
			}
			if p.Percent != tt.percent {
				t.Errorf("percent = %d, want %d", p.Percent, tt.percent)
			}
		})
	}
}

func TestSplitProgressLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "carriage return separated",
			input: "Receiving objects:  10% (1/10)\rReceiving objects:  20% (2/10)",
			want:  []string{"Receiving objects:  10% (1/10)", "Receiving objects:  20% (2/10)"},
		},
		{
			name:  "newline separated",
			input: "line1\nline2",
			want:  []string{"line1", "line2"},
		},
		{
			name:  "mixed separators",
			input: "a\rb\nc\rd",
			want:  []string{"a", "b", "c", "d"},
		},
		{
			name:  "empty chunks filtered",
			input: "\r\n\r",
			want:  nil,
		},
		{
			name:  "single line",
			input: "hello",
			want:  []string{"hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitProgressLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitProgressLines(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitProgressLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
