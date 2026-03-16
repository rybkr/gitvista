package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var cliPath string

func TestMain(m *testing.M) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find repo root: %v\n", err)
		os.Exit(1)
	}

	tmpDir, err := os.MkdirTemp("", "gitvista-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	cliPath = filepath.Join(tmpDir, "cli")

	build := exec.Command("go", "build", "-o", cliPath, "./cmd/cli")
	build.Dir = repoRoot
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build cli: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestRevListAllMatchesGitForPreparedRepos(t *testing.T) {
	repoRoot := repoRoot(t)
	repos := []string{
		filepath.Join(repoRoot, "testdata", "express"),
		filepath.Join(repoRoot, "testdata", "gitvista"),
		filepath.Join(repoRoot, "testdata", "cpython"),
	}

	for _, repoDir := range repos {
		repoDir := repoDir
		t.Run(filepath.Base(repoDir), func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
				t.Fatalf("prepared repository missing at %s; run scripts/prepare_test_repos.py first: %v", repoDir, err)
			}

			want := git(t, repoDir, "rev-list", "--all")
			got := runCLI(t, repoRoot, "--repo", repoDir, "rev-list", "--all")
			if got != want {
				t.Fatalf("rev-list --all mismatch for %s: %s", repoDir, summarizeDiff(want, got))
			}
		})
	}
}

func summarizeDiff(want, got string) string {
	wantLines := nonEmptyLines(want)
	gotLines := nonEmptyLines(got)
	limit := min(len(wantLines), len(gotLines))
	for i := 0; i < limit; i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf(
				"first difference at line %d: want %s, got %s (want lines=%d, got lines=%d)",
				i+1,
				wantLines[i],
				gotLines[i],
				len(wantLines),
				len(gotLines),
			)
		}
	}
	if len(wantLines) != len(gotLines) {
		return fmt.Sprintf("line count mismatch: want %d, got %d", len(wantLines), len(gotLines))
	}
	return "outputs differed but no differing line was found"
}

func nonEmptyLines(output string) []string {
	lines := strings.Split(output, "\n")
	return slices.DeleteFunc(lines, func(line string) bool {
		return line == ""
	})
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func findRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s failed in %s: %v\nstderr: %s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return stdout.String()
}

func runCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command(cliPath, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("cli %s failed in %s: %v\nstderr: %s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return stdout.String()
}
