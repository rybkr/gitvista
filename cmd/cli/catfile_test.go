package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestParseCatFileArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantMode catFileMode
		wantRev  string
		wantCode int
		wantErr  string
	}{
		{name: "type", args: []string{"-t", "HEAD"}, wantMode: catFileModeType, wantRev: "HEAD"},
		{name: "size", args: []string{"-s", "abc123"}, wantMode: catFileModeSize, wantRev: "abc123"},
		{name: "pretty", args: []string{"-p", "main"}, wantMode: catFileModePretty, wantRev: "main"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli cat-file"},
		{name: "unsupported flag", args: []string{"--bad", "HEAD"}, wantCode: 1, wantErr: "unsupported argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseCatFileArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseCatFileArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.mode != tt.wantMode || opts.revision != tt.wantRev {
				t.Fatalf("parseCatFileArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestFormatCatFileOutput(t *testing.T) {
	blobData, err := formatCatFileOutput(&gitcore.CatFileResult{
		Type: gitcore.ObjectTypeBlob,
		Data: []byte("blob\n"),
	})
	if err != nil || string(blobData) != "blob\n" {
		t.Fatalf("formatCatFileOutput(blob) = %q, %v", string(blobData), err)
	}

	treeHash1 := mustCLIHash(t, "1111111111111111111111111111111111111111")
	treeHash2 := mustCLIHash(t, "2222222222222222222222222222222222222222")
	treeData := treeBody(
		treeEntryBytes("100644", "README.md", treeHash1),
		treeEntryBytes("120000", "link", treeHash2),
	)
	formatted, err := formatCatFileOutput(&gitcore.CatFileResult{
		Type: gitcore.ObjectTypeTree,
		Data: treeData,
	})
	if err != nil {
		t.Fatalf("formatCatFileOutput(tree) error: %v", err)
	}
	want := strings.Join([]string{
		"100644 blob 1111111111111111111111111111111111111111\tREADME.md",
		"120000 blob 2222222222222222222222222222222222222222\tlink",
		"",
	}, "\n")
	if string(formatted) != want {
		t.Fatalf("formatCatFileOutput(tree) = %q, want %q", string(formatted), want)
	}

	if _, err := formatCatFileOutput(&gitcore.CatFileResult{Type: gitcore.ObjectTypeInvalid, Data: []byte("x")}); err == nil {
		t.Fatal("expected invalid type error")
	}
	if _, err := formatTreeObject(append([]byte("100644 bad"), 0)); err == nil {
		t.Fatal("expected malformed tree error")
	}
}

func TestRunCatFile(t *testing.T) {
	repo := newCLIRepo(t)
	blobID := mustCLIHash(t, "1111111111111111111111111111111111111111")
	treeID := mustCLIHash(t, "2222222222222222222222222222222222222222")
	commitID := mustCLIHash(t, "3333333333333333333333333333333333333333")

	treeRaw := treeBody(treeEntryBytes("100644", "README.md", blobID))
	commitRaw := []byte("tree " + string(treeID) + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nmsg\n")

	writeCLILooseObject(t, repo, blobID, "blob", []byte("hello\n"))
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
		return runCatFile(repoCtx, []string{"-t", "HEAD"})
	})
	if code != 0 || stdout != "commit\n" || stderr != "" {
		t.Fatalf("runCatFile(-t HEAD) = code %d stdout %q stderr %q", code, stdout, stderr)
	}

	stdout, stderr, code = captureCLIOutput(t, func() int {
		return runCatFile(repoCtx, []string{"-s", string(blobID)})
	})
	if code != 0 || stdout != "6\n" || stderr != "" {
		t.Fatalf("runCatFile(-s blob) = code %d stdout %q stderr %q", code, stdout, stderr)
	}

	stdout, stderr, code = captureCLIOutput(t, func() int {
		return runCatFile(repoCtx, []string{"-p", string(treeID)})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("runCatFile(-p tree) = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	wantTree := "100644 blob 1111111111111111111111111111111111111111\tREADME.md\n"
	if stdout != wantTree {
		t.Fatalf("runCatFile(-p tree) stdout = %q, want %q", stdout, wantTree)
	}

	_, stderr, code = captureCLIOutput(t, func() int {
		return runCatFile(repoCtx, []string{"--bad", "HEAD"})
	})
	if code != 1 || !strings.Contains(stderr, "unsupported argument") {
		t.Fatalf("runCatFile(bad args) = code %d stderr %q", code, stderr)
	}
}

func newCLIRepo(t *testing.T) string {
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
	return gitDir
}

func writeCLITextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeCLILooseObject(t *testing.T, gitDir string, id gitcore.Hash, objectType string, body []byte) {
	t.Helper()
	dir := filepath.Join(gitDir, "objects", string(id)[:2])
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir loose object dir: %v", err)
	}
	payload := append([]byte(fmt.Sprintf("%s %d", objectType, len(body))), 0)
	payload = append(payload, body...)
	if err := os.WriteFile(filepath.Join(dir, string(id)[2:]), compressCLIBytes(t, payload), 0o600); err != nil {
		t.Fatalf("write loose object: %v", err)
	}
}

func compressCLIBytes(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(content); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

func treeEntryBytes(mode, name string, hash gitcore.Hash) []byte {
	body := append([]byte(mode+" "+name), 0)
	raw := hashFromHex(hash)
	return append(body, raw[:]...)
}

func treeBody(entries ...[]byte) []byte {
	var body []byte
	for _, entry := range entries {
		body = append(body, entry...)
	}
	return body
}

func mustCLIHash(t *testing.T, s string) gitcore.Hash {
	t.Helper()
	h, err := gitcore.NewHash(s)
	if err != nil {
		t.Fatalf("NewHash(%q): %v", s, err)
	}
	return h
}

func hashFromHex(h gitcore.Hash) [20]byte {
	var out [20]byte
	for i := 0; i < 20; i++ {
		out[i] = byte((fromHex(string(h)[i*2]) << 4) | fromHex(string(h)[i*2+1]))
	}
	return out
}

func fromHex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	default:
		return int(b-'A') + 10
	}
}

func captureCLIOutput(t *testing.T, fn func() int) (string, string, int) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	code := fn()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	return string(stdoutBytes), string(stderrBytes), code
}
