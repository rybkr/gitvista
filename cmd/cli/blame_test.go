package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestParseBlameArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantCode int
		wantErr  string
	}{
		{name: "path only", args: []string{"README.md"}, wantPath: "README.md"},
		{name: "explicit porcelain", args: []string{"-p", "cmd/cli"}, wantPath: "cmd/cli"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli blame"},
		{name: "missing path", args: []string{""}, wantCode: 1, wantErr: "missing path"},
		{name: "unsupported flag", args: []string{"--bad", "README.md"}, wantCode: 1, wantErr: "unsupported argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseBlameArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseBlameArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.path != tt.wantPath {
				t.Fatalf("parseBlameArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestRunBlameFileAndDirectory(t *testing.T) {
	repo := newCLIRepo(t)

	blobReadme := mustCLIHash(t, "1111111111111111111111111111111111111111")
	blobMain := mustCLIHash(t, "2222222222222222222222222222222222222222")
	blobUtil := mustCLIHash(t, "3333333333333333333333333333333333333333")
	treeCLI := mustCLIHash(t, "4444444444444444444444444444444444444444")
	treeCmd := mustCLIHash(t, "5555555555555555555555555555555555555555")
	treeRoot := mustCLIHash(t, "6666666666666666666666666666666666666666")
	commitID := mustCLIHash(t, "7777777777777777777777777777777777777777")

	writeCLILooseObject(t, repo, blobReadme, "blob", []byte("readme\n"))
	writeCLILooseObject(t, repo, blobMain, "blob", []byte("main\n"))
	writeCLILooseObject(t, repo, blobUtil, "blob", []byte("util\n"))
	writeCLILooseObject(t, repo, treeCLI, "tree", treeBody(
		treeEntryBytes("100644", "main.go", blobMain),
		treeEntryBytes("100644", "util.go", blobUtil),
	))
	writeCLILooseObject(t, repo, treeCmd, "tree", treeBody(
		treeEntryBytes("040000", "cli", treeCLI),
	))
	writeCLILooseObject(t, repo, treeRoot, "tree", treeBody(
		treeEntryBytes("100644", "README.md", blobReadme),
		treeEntryBytes("040000", "cmd", treeCmd),
	))

	commitRaw := []byte("tree " + string(treeRoot) + "\nauthor Jane Doe <jane@example.com> 1700000000 -0500\ncommitter Jane Doe <jane@example.com> 1700000000 -0500\n\ninitial tree\n")
	writeCLILooseObject(t, repo, commitID, "commit", commitRaw)
	writeCLITextFile(t, filepath.Join(repo, "HEAD"), "ref: refs/heads/main\n")
	writeCLITextFile(t, filepath.Join(repo, "refs", "heads", "main"), string(commitID)+"\n")

	repository, err := gitcore.NewRepository(filepath.Dir(repo))
	if err != nil {
		t.Fatalf("NewRepository() error: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	})

	repoCtx := &repositoryContext{repo: repository}

	stdout, stderr, code := captureCLIOutput(t, func() int {
		return runBlame(repoCtx, []string{"README.md"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runBlame(file) = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	wantFile := strings.Join([]string{
		"7777777777777777777777777777777777777777 1 1 1",
		"author Jane Doe",
		"author-mail <jane@example.com>",
		"author-time 1700000000",
		"author-tz -0500",
		"summary initial tree",
		"filename README.md",
		"",
	}, "\n")
	if stdout != wantFile {
		t.Fatalf("runBlame(file) stdout = %q, want %q", stdout, wantFile)
	}

	stdout, stderr, code = captureCLIOutput(t, func() int {
		return runBlame(repoCtx, []string{"-p", "cmd/cli"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runBlame(dir) = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	wantDir := strings.Join([]string{
		"7777777777777777777777777777777777777777 1 1 1",
		"author Jane Doe",
		"author-mail <jane@example.com>",
		"author-time 1700000000",
		"author-tz -0500",
		"summary initial tree",
		"filename cmd/cli/main.go",
		"7777777777777777777777777777777777777777 1 1 1",
		"author Jane Doe",
		"author-mail <jane@example.com>",
		"author-time 1700000000",
		"author-tz -0500",
		"summary initial tree",
		"filename cmd/cli/util.go",
		"",
	}, "\n")
	if stdout != wantDir {
		t.Fatalf("runBlame(dir) stdout = %q, want %q", stdout, wantDir)
	}
}

func TestRunBlameMissingPath(t *testing.T) {
	repo := newCLIRepo(t)

	treeRoot := mustCLIHash(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	commitID := mustCLIHash(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	writeCLILooseObject(t, repo, treeRoot, "tree", treeBody())
	commitRaw := []byte("tree " + string(treeRoot) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nempty\n")
	writeCLILooseObject(t, repo, commitID, "commit", commitRaw)
	writeCLITextFile(t, filepath.Join(repo, "HEAD"), "ref: refs/heads/main\n")
	writeCLITextFile(t, filepath.Join(repo, "refs", "heads", "main"), string(commitID)+"\n")

	repository, err := gitcore.NewRepository(filepath.Dir(repo))
	if err != nil {
		t.Fatalf("NewRepository() error: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	})

	repoCtx := &repositoryContext{repo: repository}

	_, stderr, code := captureCLIOutput(t, func() int {
		return runBlame(repoCtx, []string{"missing.txt"})
	})
	if code != 128 || !strings.Contains(stderr, `resolve path "missing.txt": path component "missing.txt" not found`) {
		t.Fatalf("runBlame(missing) = code %d stderr %q", code, stderr)
	}
}
