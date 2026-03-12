package gitcore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseRemotesFromConfigStripsHTTPAuthOnly(t *testing.T) {
	config := `[remote "origin"]
	url = ` + "https://" + "user" + ":" + "pass" + "@example.com/repo.git" + `
[remote "mirror"]
	url = ` + "http://" + "token" + "@example.com/mirror.git" + `
[remote "ssh"]
	url = git@example.com:repo.git
[branch "main"]
	remote = origin
[remote "empty"]
	fetch = +refs/heads/*:refs/remotes/empty/*
`

	got := parseRemotesFromConfig(config)
	want := map[string]string{
		"origin": "https://example.com/repo.git",
		"mirror": "http://example.com/mirror.git",
		"ssh":    "git@example.com:repo.git",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseRemotesFromConfig mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestStripCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "https auth", in: "https://" + "user" + ":" + "pass" + "@example.com/repo.git", want: "https://example.com/repo.git"},
		{name: "http auth", in: "http://" + "token" + "@example.com/repo.git", want: "http://example.com/repo.git"},
		{name: "ssh unchanged", in: "git@example.com:repo.git", want: "git@example.com:repo.git"},
		{name: "https without auth unchanged", in: "https://example.com/repo.git", want: "https://example.com/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCredentials(tt.in); got != tt.want {
				t.Fatalf("stripCredentials(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFindGitDirectoryFromNestedWorktreeAndGitFile(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "repo")
	gitDir := filepath.Join(root, "repo.git")
	nested := filepath.Join(worktree, "a", "b", "c")

	for _, dir := range []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs"),
		nested,
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeTextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeTextFile(t, filepath.Join(worktree, ".git"), "gitdir: ../repo.git\n")

	gotGit, gotWork, err := findGitDirectory(nested)
	if err != nil {
		t.Fatalf("findGitDirectory nested worktree: %v", err)
	}
	if gotGit != gitDir || gotWork != worktree {
		t.Fatalf("findGitDirectory mismatch: git=%q work=%q", gotGit, gotWork)
	}
}

func TestNewRepositoryLoadsConfigAndMetadata(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "repo")
	gitDir := filepath.Join(worktree, ".git")
	blobID := mustHash(t, testHash1)
	treeID := mustHash(t, testHash2)
	commitID := mustHash(t, testHash3)
	blobRaw := hashFromHex(testHash1)

	for _, dir := range []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs", "heads"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeTextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeTextFile(t, filepath.Join(gitDir, "refs", "heads", "main"), testHash3+"\n")
	writeTextFile(t, filepath.Join(gitDir, "config"), `[remote "origin"]
	url = `+"https://"+"user"+":"+"pass"+"@example.com/repo.git"+`
`)
	writeLooseObject(t, gitDir, blobID, "blob", []byte("hello"))
	writeLooseObject(t, gitDir, treeID, "tree", append(append([]byte("100644 README.md"), 0), blobRaw[:]...))
	writeLooseObject(t, gitDir, commitID, "commit", []byte("tree "+testHash2+"\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\ninitial commit\n"))

	repo, err := NewRepository(worktree)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	if repo.Name() != "repo" {
		t.Fatalf("Name() = %q, want repo", repo.Name())
	}
	if repo.GitDir() != gitDir || repo.WorkDir() != worktree {
		t.Fatalf("unexpected repository paths: git=%q work=%q", repo.GitDir(), repo.WorkDir())
	}
	if repo.IsBare() {
		t.Fatal("expected worktree repository to be non-bare")
	}

	remotes := repo.Remotes()
	if remotes["origin"] != "https://example.com/repo.git" {
		t.Fatalf("unexpected remotes: %#v", remotes)
	}
	if repo.head != commitID || repo.headRef != "refs/heads/main" || repo.headDetached {
		t.Fatalf("unexpected HEAD state: head=%s ref=%q detached=%v", repo.head, repo.headRef, repo.headDetached)
	}
}
