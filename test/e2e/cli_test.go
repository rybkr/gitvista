//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// Fixed timestamps for deterministic output
	ts1 = "2025-01-15T10:00:00-0500"
	ts2 = "2025-01-15T11:00:00-0500"
	ts3 = "2025-01-15T12:00:00-0500"
)

func setupStandardRepo(t *testing.T) string {
	t.Helper()
	dir := setupTestRepo(t)
	addCommit(t, dir, "README.md", "# Hello\n", "Initial commit", ts1)
	addCommit(t, dir, "main.go", "package main\n", "Add main.go", ts2)
	addCommit(t, dir, "main.go", "package main\n\nfunc main() {}\n", "Update main.go", ts3)
	return dir
}

func TestLog(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "log")
	gitOut := git(t, dir, "log", "--decorate=short", "--no-color")

	compareOutput(t, "log", cliOut, gitOut)
}

func TestLogOneline(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "log", "--oneline")
	gitOut := git(t, dir, "log", "--oneline", "--decorate=short", "--no-color")

	compareOutput(t, "log --oneline", cliOut, gitOut)
}

func TestLogN(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "log", "-n2")
	gitOut := git(t, dir, "log", "-n2", "--decorate=short", "--no-color")

	compareOutput(t, "log -n2", cliOut, gitOut)
}

func TestCatFileType(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "cat-file", "-t", "HEAD")
	gitOut := git(t, dir, "cat-file", "-t", "HEAD")

	compareOutput(t, "cat-file -t", cliOut, gitOut)
}

func TestCatFileSize(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "cat-file", "-s", "HEAD")
	gitOut := git(t, dir, "cat-file", "-s", "HEAD")

	compareOutput(t, "cat-file -s", cliOut, gitOut)
}

func TestCatFilePrettyCommit(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "cat-file", "-p", "HEAD")
	gitOut := git(t, dir, "cat-file", "-p", "HEAD")

	compareOutput(t, "cat-file -p (commit)", cliOut, gitOut)
}

func TestCatFilePrettyTree(t *testing.T) {
	dir := setupStandardRepo(t)

	// Get tree hash from HEAD commit
	treeHash := strings.TrimSpace(git(t, dir, "rev-parse", "HEAD^{tree}"))

	cliOut := runCLI(t, dir, "cat-file", "-p", treeHash)
	gitOut := git(t, dir, "cat-file", "-p", treeHash)

	compareOutput(t, "cat-file -p (tree)", cliOut, gitOut)
}

func TestDiff(t *testing.T) {
	dir := setupStandardRepo(t)

	// Get the first two commit hashes
	logOut := git(t, dir, "log", "--format=%H")
	hashes := strings.Fields(strings.TrimSpace(logOut))
	if len(hashes) < 2 {
		t.Fatal("need at least 2 commits")
	}

	// Diff between second and first commit (newest to oldest in the log)
	commit1 := hashes[1] // second commit (older)
	commit2 := hashes[0] // first commit (newer)

	cliOut := runCLI(t, dir, "diff", commit1, commit2)
	// Verify our diff is non-empty and contains expected markers
	if !strings.Contains(cliOut, "diff --git") {
		t.Error("diff output missing 'diff --git' header")
	}
	if !strings.Contains(cliOut, "@@") {
		t.Error("diff output missing hunk headers")
	}
}

func TestStatusClean(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "status", "--porcelain")
	// In a clean repo with no .gitignore, there should be no output
	// (since there are no untracked, modified, or staged files)
	if strings.TrimSpace(cliOut) != "" {
		t.Errorf("expected empty porcelain output for clean repo, got:\n%s", cliOut)
	}
}

func TestStatusModified(t *testing.T) {
	dir := setupStandardRepo(t)

	// Modify a tracked file
	if err := writeFile(dir, "main.go", "package main\n\n// modified\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}

	cliOut := runCLI(t, dir, "status", "--porcelain")
	if !strings.Contains(cliOut, " M main.go") {
		t.Errorf("expected ' M main.go' in porcelain output, got:\n%s", cliOut)
	}
}

func TestMergeCommit(t *testing.T) {
	dir := setupTestRepo(t)

	// Create initial commit on main
	addCommit(t, dir, "README.md", "# Hello\n", "Initial commit", ts1)

	// Create a branch and add a commit
	git(t, dir, "checkout", "-b", "feature")
	addCommit(t, dir, "feature.go", "package feature\n", "Add feature", ts2)

	// Go back to main and add a different commit
	git(t, dir, "checkout", "main")
	addCommit(t, dir, "main.go", "package main\n", "Add main", ts2)

	// Merge the feature branch
	gitWithEnv(t, dir, []string{
		"GIT_AUTHOR_DATE=" + ts3,
		"GIT_COMMITTER_DATE=" + ts3,
	}, "merge", "feature", "--no-edit")

	// Verify our log shows the merge commit
	cliOut := runCLI(t, dir, "log", "-n1")
	if !strings.Contains(cliOut, "Merge:") {
		t.Errorf("expected merge commit to contain 'Merge:' line, got:\n%s", cliOut)
	}
}

func TestBranch(t *testing.T) {
	dir := setupStandardRepo(t)

	// Create an extra branch
	git(t, dir, "branch", "feature")

	cliOut := runCLI(t, dir, "branch")
	gitOut := git(t, dir, "branch", "--no-color")

	compareOutput(t, "branch", cliOut, gitOut)
}

func TestTag(t *testing.T) {
	dir := setupStandardRepo(t)

	// Create two lightweight tags
	git(t, dir, "tag", "v0.1.0")
	git(t, dir, "tag", "v0.2.0")

	cliOut := runCLI(t, dir, "tag")
	gitOut := git(t, dir, "tag")

	compareOutput(t, "tag", cliOut, gitOut)
}

func TestTagEmpty(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "tag")
	if strings.TrimSpace(cliOut) != "" {
		t.Errorf("expected empty tag output, got:\n%s", cliOut)
	}
}

func TestShow(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "show")

	if !strings.Contains(cliOut, "commit ") {
		t.Error("show output missing 'commit ' line")
	}
	if !strings.Contains(cliOut, "Author:") {
		t.Error("show output missing 'Author:' line")
	}
	if !strings.Contains(cliOut, "Date:") {
		t.Error("show output missing 'Date:' line")
	}
	if !strings.Contains(cliOut, "diff --git") {
		t.Error("show output missing 'diff --git' header")
	}
}

func TestShowStat(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "show", "--stat")

	if !strings.Contains(cliOut, "commit ") {
		t.Error("show --stat output missing 'commit ' line")
	}
	if !strings.Contains(cliOut, "file(s) changed") {
		t.Error("show --stat output missing 'file(s) changed' line")
	}
}

func TestStashListEmpty(t *testing.T) {
	dir := setupStandardRepo(t)

	cliOut := runCLI(t, dir, "stash", "list")
	if strings.TrimSpace(cliOut) != "" {
		t.Errorf("expected empty stash list output, got:\n%s", cliOut)
	}
}

func TestStashList(t *testing.T) {
	dir := setupStandardRepo(t)

	// Modify a file and stash it
	if err := writeFile(dir, "main.go", "package main\n\n// stashed\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "stash", "push", "-m", "test stash")

	cliOut := runCLI(t, dir, "stash", "list")
	gitOut := git(t, dir, "stash", "list")

	compareOutput(t, "stash list", cliOut, gitOut)
}

func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}
