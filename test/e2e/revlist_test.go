//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRevListMatchesGit(t *testing.T) {
	dir := setupRevListRepo(t)
	featureHash := strings.TrimSpace(git(t, dir, "rev-parse", "feature"))

	tests := []struct {
		name string
		args []string
	}{
		{name: "head", args: []string{"rev-list", "HEAD"}},
		{name: "count", args: []string{"rev-list", "--count", "HEAD"}},
		{name: "no-merges", args: []string{"rev-list", "--no-merges", "HEAD"}},
		{name: "topo-order", args: []string{"rev-list", "--topo-order", "HEAD"}},
		{name: "date-order", args: []string{"rev-list", "--date-order", "HEAD"}},
		{name: "specific hash", args: []string{"rev-list", featureHash}},
		{name: "all", args: []string{"rev-list", "--all"}},
		{name: "all topo-order", args: []string{"rev-list", "--all", "--topo-order"}},
		{name: "all count", args: []string{"rev-list", "--all", "--count"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runCLI(t, dir, tt.args...)
			want := git(t, dir, tt.args...)
			compareOutput(t, tt.name, got, want)
		})
	}
}

func TestRevListRejectsUnsupportedArgs(t *testing.T) {
	dir := setupRevListRepo(t)

	_, stderr := runCLIExpectExit(t, dir, 1, "rev-list", "--reverse", "HEAD")
	if stderr != "gitvista-cli rev-list: unsupported argument \"--reverse\"\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRevListRequiresRevision(t *testing.T) {
	dir := setupRevListRepo(t)

	_, stderr := runCLIExpectExit(t, dir, 1, "rev-list", "--count")
	if stderr != "gitvista-cli rev-list: missing revision (expected <commit> or --all)\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRevListRejectsMultipleRevisions(t *testing.T) {
	dir := setupRevListRepo(t)

	_, stderr := runCLIExpectExit(t, dir, 1, "rev-list", "HEAD", "main")
	if stderr != "gitvista-cli rev-list: accepts at most one revision argument\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func setupRevListRepo(t *testing.T) string {
	t.Helper()

	dir := setupTestRepo(t)

	addCommit(t, dir, "base.txt", "base\n", "base", "2024-01-01T00:00:00Z")
	git(t, dir, "branch", "feature")

	addCommit(t, dir, "main.txt", "main\n", "main", "2024-01-01T01:00:00Z")

	git(t, dir, "checkout", "feature")
	addCommit(t, dir, "feature.txt", "feature\n", "feature", "2024-01-01T02:00:00Z")

	git(t, dir, "checkout", "main")
	gitWithEnv(t, dir, []string{
		"GIT_AUTHOR_DATE=2024-01-01T03:00:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T03:00:00Z",
	}, "merge", "--no-ff", "-m", "merge feature", "feature")

	return dir
}
