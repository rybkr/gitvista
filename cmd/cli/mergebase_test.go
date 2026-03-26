package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestParseMergeBaseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantOurs   string
		wantTheirs string
		wantCode   int
		wantErr    string
	}{
		{name: "two revisions", args: []string{"HEAD", "main"}, wantOurs: "HEAD", wantTheirs: "main"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli merge-base"},
		{name: "one arg", args: []string{"HEAD"}, wantCode: 1, wantErr: "usage: gitvista-cli merge-base"},
		{name: "too many args", args: []string{"HEAD", "main", "extra"}, wantCode: 1, wantErr: "usage: gitvista-cli merge-base"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseMergeBaseArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseMergeBaseArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.ours != tt.wantOurs || opts.theirs != tt.wantTheirs {
				t.Fatalf("parseMergeBaseArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestRunMergeBase(t *testing.T) {
	repo := newCLIRepo(t)

	baseTreeID := mustCLIHash(t, "1111111111111111111111111111111111111111")
	oursTreeID := mustCLIHash(t, "2222222222222222222222222222222222222222")
	theirsTreeID := mustCLIHash(t, "3333333333333333333333333333333333333333")
	baseCommitID := mustCLIHash(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	oursCommitID := mustCLIHash(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	theirsCommitID := mustCLIHash(t, "cccccccccccccccccccccccccccccccccccccccc")
	blobID := mustCLIHash(t, "dddddddddddddddddddddddddddddddddddddddd")

	writeCLILooseObject(t, repo, blobID, "blob", []byte("hello\n"))
	writeCLILooseObject(t, repo, baseTreeID, "tree", treeBody(treeEntryBytes("100644", "file.txt", blobID)))
	writeCLILooseObject(t, repo, oursTreeID, "tree", treeBody(treeEntryBytes("100644", "file.txt", blobID)))
	writeCLILooseObject(t, repo, theirsTreeID, "tree", treeBody(treeEntryBytes("100644", "file.txt", blobID)))

	baseRaw := []byte("tree " + string(baseTreeID) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nbase\n")
	oursRaw := []byte("tree " + string(oursTreeID) + "\nparent " + string(baseCommitID) + "\nauthor Jane Doe <jane@example.com> 1700000600 +0000\ncommitter Jane Doe <jane@example.com> 1700000600 +0000\n\nours\n")
	theirsRaw := []byte("tree " + string(theirsTreeID) + "\nparent " + string(baseCommitID) + "\nauthor Jane Doe <jane@example.com> 1700001200 +0000\ncommitter Jane Doe <jane@example.com> 1700001200 +0000\n\ntheirs\n")

	writeCLILooseObject(t, repo, baseCommitID, "commit", baseRaw)
	writeCLILooseObject(t, repo, oursCommitID, "commit", oursRaw)
	writeCLILooseObject(t, repo, theirsCommitID, "commit", theirsRaw)

	writeCLITextFile(t, filepath.Join(repo, "HEAD"), "ref: refs/heads/main\n")
	writeCLITextFile(t, filepath.Join(repo, "refs", "heads", "main"), string(oursCommitID)+"\n")
	writeCLITextFile(t, filepath.Join(repo, "refs", "heads", "feature"), string(theirsCommitID)+"\n")

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
		return runMergeBase(repoCtx, []string{"main", "feature"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runMergeBase(main, feature) = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	if stdout != string(baseCommitID)+"\n" {
		t.Fatalf("runMergeBase(main, feature) stdout = %q, want %q", stdout, string(baseCommitID)+"\n")
	}

	_, stderr, code = captureCLIOutput(t, func() int {
		return runMergeBase(repoCtx, []string{"missing", "feature"})
	})
	if code != 128 || !strings.Contains(stderr, "ambiguous argument") {
		t.Fatalf("runMergeBase(missing, feature) = code %d stderr %q", code, stderr)
	}

	_, stderr, code = captureCLIOutput(t, func() int {
		return runMergeBase(repoCtx, []string{"main"})
	})
	if code != 1 || !strings.Contains(stderr, "usage: gitvista-cli merge-base") {
		t.Fatalf("runMergeBase(main) = code %d stderr %q", code, stderr)
	}
}
