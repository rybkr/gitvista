package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestParseLsTreeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantRev  string
		wantCode int
		wantErr  string
	}{
		{name: "revision", args: []string{"HEAD"}, wantRev: "HEAD"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli ls-tree"},
		{name: "too many args", args: []string{"HEAD", "extra"}, wantCode: 1, wantErr: "usage: gitvista-cli ls-tree"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseLsTreeArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseLsTreeArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.revision != tt.wantRev {
				t.Fatalf("parseLsTreeArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestRunLsTree(t *testing.T) {
	repo := newCLIRepo(t)
	blobID := mustCLIHash(t, "1111111111111111111111111111111111111111")
	treeID := mustCLIHash(t, "2222222222222222222222222222222222222222")
	subtreeID := mustCLIHash(t, "3333333333333333333333333333333333333333")
	commitID := mustCLIHash(t, "4444444444444444444444444444444444444444")

	treeRaw := treeBody(
		treeEntryBytes("100644", "README.md", blobID),
		treeEntryBytes("040000", "docs", subtreeID),
	)
	commitRaw := []byte("tree " + string(treeID) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nmsg\n")

	writeCLILooseObject(t, repo, blobID, "blob", []byte("hello\n"))
	writeCLILooseObject(t, repo, subtreeID, "tree", treeBody())
	writeCLILooseObject(t, repo, treeID, "tree", treeRaw)
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
		return runLsTree(repoCtx, []string{"HEAD"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runLsTree(HEAD) = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	want := strings.Join([]string{
		"100644 blob 1111111111111111111111111111111111111111\tREADME.md",
		"040000 tree 3333333333333333333333333333333333333333\tdocs",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("runLsTree(HEAD) stdout = %q, want %q", stdout, want)
	}

	_, stderr, code = captureCLIOutput(t, func() int {
		return runLsTree(repoCtx, []string{"missing"})
	})
	if code != 128 || !strings.Contains(stderr, "ambiguous argument") {
		t.Fatalf("runLsTree(missing) = code %d stderr %q", code, stderr)
	}

	_, stderr, code = captureCLIOutput(t, func() int {
		return runLsTree(repoCtx, nil)
	})
	if code != 1 || !strings.Contains(stderr, "usage: gitvista-cli ls-tree") {
		t.Fatalf("runLsTree(no args) = code %d stderr %q", code, stderr)
	}
}
