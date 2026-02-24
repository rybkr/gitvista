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

	err := cloneRepo(context.Background(), "https://invalid.invalid/no/such/repo.git", destPath, 10*time.Second)
	if err == nil {
		t.Fatal("cloneRepo() with invalid URL should return error")
	}

	// Verify cleanup happened
	if _, statErr := os.Stat(destPath); statErr == nil {
		t.Error("destPath should have been cleaned up after failed clone")
	}
}
