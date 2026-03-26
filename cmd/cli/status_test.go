package main

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- test helper for Git object ids
	"encoding/binary"
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

func TestRunStatus(t *testing.T) {
	repoDir, gitDir := newStatusCLIRepoDir(t)
	headBlob := writeStatusBlob(t, gitDir, []byte("tracked\n"))
	stagedBlob := writeStatusBlob(t, gitDir, []byte("staged\n"))
	treeID := writeStatusTree(t, gitDir, []statusTreeEntry{
		{name: "tracked.txt", id: headBlob, mode: "100644"},
	})
	commitID := writeStatusCommit(t, gitDir, treeID)

	writeCLITextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeCLITextFile(t, filepath.Join(gitDir, "refs", "heads", "main"), string(commitID)+"\n")
	writeStatusIndex(t, gitDir, []statusIndexEntry{
		{path: "tracked.txt", blobHash: headBlob, fileSize: uint32(len("tracked\n"))},
		{path: "staged.txt", blobHash: stagedBlob, fileSize: uint32(len("staged\n"))},
	})
	writeCLITextFile(t, filepath.Join(repoDir, "tracked.txt"), "modified\n")
	writeCLITextFile(t, filepath.Join(repoDir, "staged.txt"), "staged\n")
	writeCLITextFile(t, filepath.Join(repoDir, "untracked.txt"), "new\n")

	repo, err := gitcore.NewRepository(repoDir)
	if err != nil {
		t.Fatalf("NewRepository() error: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	})

	repoCtx := &repositoryContext{repo: repo}
	cw := cli.NewWriter(os.Stdout, cli.ColorNever)

	stdout, stderr, code := captureCLIOutput(t, func() int {
		return runStatus(repoCtx, nil, cw)
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runStatus() = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Changes to be committed:") || !strings.Contains(stdout, "new file:  staged.txt") {
		t.Fatalf("runStatus() stdout missing staged section: %q", stdout)
	}
	if !strings.Contains(stdout, "Changes not staged for commit:") || !strings.Contains(stdout, "modified:  tracked.txt") {
		t.Fatalf("runStatus() stdout missing modified section: %q", stdout)
	}
	if !strings.Contains(stdout, "Untracked files:") || !strings.Contains(stdout, "untracked.txt") {
		t.Fatalf("runStatus() stdout missing untracked section: %q", stdout)
	}

	stdout, stderr, code = captureCLIOutput(t, func() int {
		return runStatus(repoCtx, []string{"--short"}, cw)
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runStatus(--short) = code %d stdout %q stderr %q", code, stdout, stderr)
	}

	wantShort := strings.Join([]string{
		"A  staged.txt",
		" M tracked.txt",
		"?? untracked.txt",
		"",
	}, "\n")
	if stdout != wantShort {
		t.Fatalf("runStatus(--short) stdout = %q, want %q", stdout, wantShort)
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
	treeID := writeStatusTree(t, gitDir, nil)
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

type statusIndexEntry struct {
	path     string
	blobHash gitcore.Hash
	fileSize uint32
	mode     uint32
}

func writeStatusIndex(t *testing.T, gitDir string, entries []statusIndexEntry) {
	t.Helper()
	data := buildStatusIndexHeader(uint32(len(entries)))
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o100644
		}
		rawHash := statusHashFromHex(string(entry.blobHash))
		start := len(data)
		data = append(data, buildStatusIndexEntry(entry.path, rawHash, mode)...)
		binary.BigEndian.PutUint32(data[start+36:start+40], entry.fileSize)
	}
	writeStatusIndexFile(t, gitDir, data)
}

func buildStatusIndexHeader(numEntries uint32) []byte {
	buf := &bytes.Buffer{}
	buf.WriteString("DIRC")
	_ = binary.Write(buf, binary.BigEndian, uint32(2))
	_ = binary.Write(buf, binary.BigEndian, numEntries)
	return buf.Bytes()
}

func buildStatusIndexEntry(path string, hash [20]byte, mode uint32) []byte {
	buf := &bytes.Buffer{}
	fields := []uint32{
		0, 0,
		0, 0,
		0, 0, mode, 0, 0, uint32(len(path)),
	}
	for _, field := range fields {
		_ = binary.Write(buf, binary.BigEndian, field)
	}
	buf.Write(hash[:])
	_ = binary.Write(buf, binary.BigEndian, uint16(len(path)))
	buf.WriteString(path)
	buf.WriteByte(0)
	for buf.Len()%8 != 0 {
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

func writeStatusIndexFile(t *testing.T, gitDir string, data []byte) {
	t.Helper()
	sum := sha1.Sum(data) // #nosec G401 -- test helper for Git index checksum
	if err := os.WriteFile(filepath.Join(gitDir, "index"), append(data, sum[:]...), 0o644); err != nil {
		t.Fatalf("WriteFile(index): %v", err)
	}
}

type statusTreeEntry struct {
	name string
	id   gitcore.Hash
	mode string
}

func writeStatusBlob(t *testing.T, gitDir string, body []byte) gitcore.Hash {
	t.Helper()
	return writeStatusObject(t, gitDir, "blob", body)
}

func writeStatusTree(t *testing.T, gitDir string, entries []statusTreeEntry) gitcore.Hash {
	t.Helper()
	var body []byte
	for _, entry := range entries {
		body = append(body, []byte(entry.mode+" "+entry.name)...)
		body = append(body, 0)
		rawHash := statusHashFromHex(string(entry.id))
		body = append(body, rawHash[:]...)
	}
	return writeStatusObject(t, gitDir, "tree", body)
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

func statusHashFromHex(s string) [20]byte {
	var out [20]byte
	for i := 0; i < len(out); i++ {
		out[i] = byte((statusFromHex(s[i*2]) << 4) | statusFromHex(s[i*2+1]))
	}
	return out
}

func statusFromHex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	default:
		return int(b-'A') + 10
	}
}
