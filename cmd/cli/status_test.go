package main

import (
	"crypto/sha1" // #nosec G505 -- test helper for Git object ids
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/cli"
)

func TestParseStatusArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantShort bool
		wantCode  int
		wantErr   string
	}{
		{name: "default", args: nil},
		{name: "short", args: []string{"--short"}, wantShort: true},
		{name: "bad arg", args: []string{"--bad"}, wantCode: 1, wantErr: "unsupported argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseStatusArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseStatusArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}

			if err != nil || code != 0 || opts.short != tt.wantShort {
				t.Fatalf("parseStatusArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestPrintShortStatus(t *testing.T) {
	status := &gitcore.WorkingTreeStatus{
		Files: []gitcore.FileStatus{
			{Path: "alpha.txt", IndexStatus: gitcore.StatusAdded},
			{Path: "beta.txt", IndexStatus: gitcore.StatusModified, WorkStatus: gitcore.StatusModified},
			{Path: "gamma.txt", WorkStatus: gitcore.StatusDeleted},
			{Path: "untracked.txt", IsUntracked: true},
		},
	}

	stdout, stderr, code := captureCLIOutput(t, func() int {
		printShortStatus(status)
		return 0
	})
	if code != 0 || stderr != "" {
		t.Fatalf("printShortStatus() = code %d stderr %q", code, stderr)
	}

	want := strings.Join([]string{
		"A  alpha.txt",
		"MM beta.txt",
		" D gamma.txt",
		"?? untracked.txt",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("printShortStatus() stdout = %q, want %q", stdout, want)
	}
}

func TestPrintShortStatus_TypeChangeAndQuotedPaths(t *testing.T) {
	status := &gitcore.WorkingTreeStatus{
		Files: []gitcore.FileStatus{
			{Path: "mode-staged.sh", IndexStatus: gitcore.StatusTypeChanged},
			{Path: "mode-worktree.sh", WorkStatus: gitcore.StatusTypeChanged},
			{Path: "space name.txt", WorkStatus: gitcore.StatusModified},
			{Path: "nested/", IsUntracked: true},
		},
	}

	stdout, stderr, code := captureCLIOutput(t, func() int {
		printShortStatus(status)
		return 0
	})
	if code != 0 || stderr != "" {
		t.Fatalf("printShortStatus() = code %d stderr %q", code, stderr)
	}

	want := strings.Join([]string{
		"T  mode-staged.sh",
		" T mode-worktree.sh",
		" M \"space name.txt\"",
		"?? nested/",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("printShortStatus() stdout = %q, want %q", stdout, want)
	}
}

func TestPrintLongStatus(t *testing.T) {
	repo := newStatusCLIRepo(t)
	status := &gitcore.WorkingTreeStatus{
		Files: []gitcore.FileStatus{
			{Path: "staged.txt", IndexStatus: gitcore.StatusAdded},
			{Path: "tracked.txt", WorkStatus: gitcore.StatusModified},
			{Path: "untracked.txt", IsUntracked: true},
		},
	}
	cw := cli.NewWriter(os.Stdout, cli.ColorNever)

	stdout, stderr, code := captureCLIOutput(t, func() int {
		printLongStatus(repo, status, cw)
		return 0
	})
	if code != 0 || stderr != "" {
		t.Fatalf("printLongStatus() = code %d stderr %q", code, stderr)
	}

	want := strings.Join([]string{
		"On branch main",
		"",
		"Changes to be committed:",
		"  new file:  staged.txt",
		"",
		"Changes not staged for commit:",
		"  modified:  tracked.txt",
		"",
		"Untracked files:",
		"  untracked.txt",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("printLongStatus() stdout = %q, want %q", stdout, want)
	}
}

func TestRunStatusCleanWorktree(t *testing.T) {
	repo := newStatusCLIRepo(t)
	cw := cli.NewWriter(os.Stdout, cli.ColorNever)

	stdout, stderr, code := captureCLIOutput(t, func() int {
		printLongStatus(repo, &gitcore.WorkingTreeStatus{}, cw)
		return 0
	})
	if code != 0 || stderr != "" {
		t.Fatalf("printLongStatus(clean) = code %d stderr %q", code, stderr)
	}

	want := "On branch main\nnothing to commit, working tree clean\n"
	if stdout != want {
		t.Fatalf("printLongStatus(clean) stdout = %q, want %q", stdout, want)
	}
}

func newStatusCLIRepo(t *testing.T) *gitcore.Repository {
	t.Helper()
	repoDir, gitDir := newStatusCLIRepoDir(t)
	treeID := writeStatusTree(t, gitDir)
	commitID := writeStatusCommit(t, gitDir, treeID)
	writeCLITextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeCLITextFile(t, filepath.Join(gitDir, "refs", "heads", "main"), string(commitID)+"\n")

	repo, err := gitcore.NewRepository(repoDir)
	if err != nil {
		t.Fatalf("NewRepository() error: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	})
	return repo
}

func writeStatusTree(t *testing.T, gitDir string) gitcore.Hash {
	t.Helper()
	return writeStatusObject(t, gitDir, "tree", nil)
}

func writeStatusCommit(t *testing.T, gitDir string, treeID gitcore.Hash) gitcore.Hash {
	t.Helper()
	body := []byte("tree " + string(treeID) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\ninitial commit\n")
	return writeStatusObject(t, gitDir, "commit", body)
}

func writeStatusObject(t *testing.T, gitDir, objectType string, body []byte) gitcore.Hash {
	t.Helper()
	payload := append([]byte(fmt.Sprintf("%s %d", objectType, len(body))), 0)
	payload = append(payload, body...)
	sum := sha1.Sum(payload) // #nosec G401 -- test helper for Git object ids
	id := gitcore.Hash(fmt.Sprintf("%x", sum[:]))
	writeCLILooseObject(t, gitDir, id, objectType, body)
	return id
}

func newStatusCLIRepoDir(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	for _, dir := range []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "tags"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return root, gitDir
}
