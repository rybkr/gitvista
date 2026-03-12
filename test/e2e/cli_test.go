//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpIncludesRepoFlag(t *testing.T) {
	_, stderr := runCLIExpectExit(t, t.TempDir(), 1)

	if !strings.Contains(stderr, "--repo <path>") {
		t.Fatalf("help output missing --repo flag:\n%s", stderr)
	}
	if !strings.Contains(stderr, "default: .") {
		t.Fatalf("help output missing repo default:\n%s", stderr)
	}
}

func TestRepoDefaultsToCurrentDirectory(t *testing.T) {
	dir := setupTestRepo(t)

	stdout := runCLI(t, dir, "repo")
	if !strings.Contains(stdout, "Repository  "+filepath.Base(dir)) {
		t.Fatalf("expected repository header for %q, got:\n%s", dir, stdout)
	}
	if !strings.Contains(stdout, "worktree  "+dir) {
		t.Fatalf("expected worktree path %q, got:\n%s", dir, stdout)
	}
	if !strings.Contains(stdout, "git dir   "+filepath.Join(dir, ".git")) {
		t.Fatalf("expected gitdir path %q, got:\n%s", filepath.Join(dir, ".git"), stdout)
	}
	if !strings.Contains(stdout, "Loaded in") || !strings.Contains(stdout, "Stats") {
		t.Fatalf("expected load timing and stats output, got:\n%s", stdout)
	}
}

func TestRepoFlagUsesExplicitPath(t *testing.T) {
	dir := setupTestRepo(t)
	child := filepath.Join(dir, "nested", "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout := runCLI(t, t.TempDir(), "--repo", child, "repo")
	if !strings.Contains(stdout, "worktree  "+dir) {
		t.Fatalf("expected resolved repo worktree %q, got:\n%s", dir, stdout)
	}
	if !strings.Contains(stdout, "git dir   "+filepath.Join(dir, ".git")) {
		t.Fatalf("expected resolved gitdir %q, got:\n%s", filepath.Join(dir, ".git"), stdout)
	}
}

func TestRepoFlagRejectsMissingRepository(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-repo")

	_, stderr := runCLIExpectExit(t, t.TempDir(), 128, "--repo", missing, "repo")
	if !strings.Contains(stderr, "fatal:") {
		t.Fatalf("expected fatal repository error, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "not a git repository") && !strings.Contains(stderr, "does not exist") {
		t.Fatalf("expected repository lookup failure, got:\n%s", stderr)
	}
}
