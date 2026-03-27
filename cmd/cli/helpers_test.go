package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

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
