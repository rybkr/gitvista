//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once before all tests
	tmpDir, err := os.MkdirTemp("", "gitvista-cli-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binaryPath = filepath.Join(tmpDir, "gitvista-cli")

	// Find the repo root (two levels up from test/e2e)
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find repo root: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/gitcli")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build gitvista-cli: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func findRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// setupTestRepo creates a temporary git repository with deterministic timestamps.
// Returns the repo path and a cleanup function.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize repo
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.name", "Test User")
	git(t, dir, "config", "user.email", "test@example.com")

	return dir
}

// addCommit creates a file and commits it with a deterministic timestamp.
func addCommit(t *testing.T, dir string, filename, content, message string, timestamp string) {
	t.Helper()
	filePath := filepath.Join(dir, filename)

	// Ensure parent directory exists
	if parentDir := filepath.Dir(filePath); parentDir != dir {
		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", parentDir, err)
		}
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", filename, err)
	}

	git(t, dir, "add", filename)
	gitWithEnv(t, dir, []string{
		"GIT_AUTHOR_DATE=" + timestamp,
		"GIT_COMMITTER_DATE=" + timestamp,
	}, "commit", "-m", message)
}

// git runs a git command in dir and returns stdout.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gitWithEnv(t, dir, nil, args...)
}

// gitWithEnv runs a git command with extra environment variables.
func gitWithEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

// runCLI runs the gitvista-cli binary with the given arguments.
func runCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "GIT_DIR="+filepath.Join(dir, ".git"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gitvista-cli %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

// compareOutput compares two outputs and fails the test if they differ.
func compareOutput(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s output mismatch:\n--- want ---\n%s\n--- got ---\n%s", label, want, got)
	}
}
