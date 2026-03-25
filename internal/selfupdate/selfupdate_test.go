package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckLatest(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		wantTag    string
		wantErr    bool
	}{
		{
			name:       "valid release",
			body:       `{"tag_name": "v1.2.3"}`,
			statusCode: 200,
			wantTag:    "v1.2.3",
		},
		{
			name:       "empty tag",
			body:       `{"tag_name": ""}`,
			statusCode: 200,
			wantErr:    true,
		},
		{
			name:       "not found",
			body:       `{"message": "Not Found"}`,
			statusCode: 404,
			wantErr:    true,
		},
		{
			name:       "invalid json",
			body:       `{invalid`,
			statusCode: 200,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
				return newTestResponse(req, tt.statusCode, tt.body), nil
			})

			tag, err := checkLatestFrom("https://example.test/releases/latest")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tag != tt.wantTag {
				t.Errorf("got tag %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

func TestCheckLatest_TransportError(t *testing.T) {
	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})

	_, err := checkLatestFrom("https://example.test/releases/latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "checking latest version") {
		t.Fatalf("expected wrapped transport error, got %v", err)
	}
}

func TestCheckLatest_Wrapper(t *testing.T) {
	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://api.github.com/repos/rybkr/gitvista/releases/latest" {
			t.Fatalf("unexpected URL %q", req.URL.String())
		}
		return newTestResponse(req, http.StatusOK, `{"tag_name": "v1.2.3"}`), nil
	})

	tag, err := CheckLatest("rybkr/gitvista")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("got tag %q, want %q", tag, "v1.2.3")
	}
}

func TestNeedsUpdate(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"dev", "v1.0.0", false},
		{"", "v1.0.0", false},
		{"v1.0.0", "v1.0.0", false},
		{"1.0.0", "v1.0.0", false},
		{"v1.0.0", "1.0.0", false},
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "v2.0.0", true},
		{"0.9.0", "v1.0.0", true},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_vs_%s", tt.current, tt.latest)
		t.Run(name, func(t *testing.T) {
			got := NeedsUpdate(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("NeedsUpdate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestArchiveName(t *testing.T) {
	name := ArchiveName("gitvista", "v1.2.3")

	if !strings.Contains(name, "gitvista_1.2.3_") {
		t.Errorf("expected name to contain 'gitvista_1.2.3_', got %q", name)
	}
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("expected name to contain %q, got %q", runtime.GOOS, name)
	}
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("expected name to contain %q, got %q", runtime.GOARCH, name)
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello world")
	h := sha256.Sum256(data)
	goodHash := hex.EncodeToString(h[:])

	checksums := fmt.Sprintf("%s  test.tar.gz\nbadbadbad  other.tar.gz\n", goodHash)

	t.Run("valid checksum", func(t *testing.T) {
		err := verifyChecksum(data, []byte(checksums), "test.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		err := verifyChecksum([]byte("different data"), []byte(checksums), "test.tar.gz")
		if err == nil {
			t.Fatal("expected checksum mismatch error")
		}
		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("expected 'checksum mismatch' in error, got: %v", err)
		}
	})

	t.Run("file not in checksums", func(t *testing.T) {
		err := verifyChecksum(data, []byte(checksums), "missing.tar.gz")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", err)
		}
	})
}

func TestExtractFromTarGz(t *testing.T) {
	binaryContent := []byte("#!/bin/fake-binary")
	archive := makeTarGz(t, "gitvista", binaryContent)

	got, err := extractFromTarGz(archive, "gitvista")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Errorf("extracted content mismatch: got %q, want %q", got, binaryContent)
	}
}

func TestExtractFromTarGz_NotFound(t *testing.T) {
	archive := makeTarGz(t, "other-binary", []byte("data"))

	_, err := extractFromTarGz(archive, "gitvista")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestExtractFromTarGz_InvalidGzip(t *testing.T) {
	_, err := extractFromTarGz([]byte("not-gzip"), "gitvista")
	if err == nil {
		t.Fatal("expected error for invalid gzip data")
	}
	if !strings.Contains(err.Error(), "gzip reader") {
		t.Fatalf("expected gzip reader error, got %v", err)
	}
}

func TestExtractFromZip(t *testing.T) {
	binaryContent := []byte("windows-binary")
	archive := makeZip(t, "gitvista.exe", binaryContent)

	got, err := extractFromZip(archive, "gitvista")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Fatalf("got %q, want %q", got, binaryContent)
	}
}

func TestExtractFromZip_NotFound(t *testing.T) {
	archive := makeZip(t, "other.exe", []byte("data"))

	_, err := extractFromZip(archive, "gitvista")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	binaryContent := []byte("windows-binary")
	archive := makeZip(t, "nested/gitvista.exe", binaryContent)

	got, err := extractBinary(archive, "gitvista_1.0.0_windows_amd64.zip", "gitvista")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Fatalf("got %q, want %q", got, binaryContent)
	}
}

func TestHTTPGetBytes(t *testing.T) {
	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, "payload"), nil
	})

	got, err := httpGetBytes("https://example.test/archive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("got %q, want %q", got, "payload")
	}
}

func TestHTTPGetBytes_TransportError(t *testing.T) {
	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})

	_, err := httpGetBytes("https://example.test/archive")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestUpdateFlow(t *testing.T) {
	binaryContent := []byte("#!/bin/updated-binary")
	archive := makeTarGz(t, "gitvista", binaryContent)

	h := sha256.Sum256(archive)
	archiveName := ArchiveName("gitvista", "v1.0.0")
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(h[:]), archiveName)
	baseURL := "https://example.test/releases/download/v1.0.0"
	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.String() == baseURL+"/checksums.txt":
			return newTestResponse(req, http.StatusOK, checksums), nil
		case req.URL.String() == baseURL+"/"+archiveName:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
				Request:    req,
			}, nil
		default:
			return newTestResponse(req, http.StatusNotFound, "not found"), nil
		}
	})

	originalReplaceBinary := replaceBinaryFunc
	t.Cleanup(func() {
		replaceBinaryFunc = originalReplaceBinary
	})

	var replaced []byte
	replaceBinaryFunc = func(data []byte) error {
		replaced = append([]byte(nil), data...)
		return nil
	}

	if err := updateFrom(baseURL, "gitvista", "v1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(replaced, binaryContent) {
		t.Fatalf("replaceBinary received %q, want %q", replaced, binaryContent)
	}
}

func TestUpdate_Wrapper(t *testing.T) {
	originalReplaceBinary := replaceBinaryFunc
	t.Cleanup(func() {
		replaceBinaryFunc = originalReplaceBinary
	})
	replaceBinaryFunc = func(data []byte) error {
		if len(data) == 0 {
			t.Fatal("expected binary data")
		}
		return nil
	}

	archiveName := ArchiveName("gitvista", "v1.0.0")
	archive := makeTarGz(t, "gitvista", []byte("updated"))
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)

	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://github.com/rybkr/gitvista/releases/download/v1.0.0/" + archiveName:
			return responseWithBytes(req, http.StatusOK, archive), nil
		case "https://github.com/rybkr/gitvista/releases/download/v1.0.0/checksums.txt":
			return newTestResponse(req, http.StatusOK, checksums), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	})

	if err := Update("rybkr/gitvista", "gitvista", "v1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateFrom_DownloadArchiveError(t *testing.T) {
	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusNotFound, "missing"), nil
	})

	err := updateFrom("https://example.test/releases/download/v1.0.0", "gitvista", "v1.0.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "downloading archive") {
		t.Fatalf("expected archive download error, got %v", err)
	}
}

func TestUpdateFrom_DownloadChecksumsError(t *testing.T) {
	archiveName := ArchiveName("gitvista", "v1.0.0")
	archive := makeTarGz(t, "gitvista", []byte("updated"))
	baseURL := "https://example.test/releases/download/v1.0.0"

	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case baseURL + "/" + archiveName:
			return responseWithBytes(req, http.StatusOK, archive), nil
		case baseURL + "/checksums.txt":
			return newTestResponse(req, http.StatusNotFound, "missing"), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	})

	err := updateFrom(baseURL, "gitvista", "v1.0.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "downloading checksums") {
		t.Fatalf("expected checksums download error, got %v", err)
	}
}

func TestUpdateFrom_ChecksumMismatch(t *testing.T) {
	archiveName := ArchiveName("gitvista", "v1.0.0")
	archive := makeTarGz(t, "gitvista", []byte("updated"))
	baseURL := "https://example.test/releases/download/v1.0.0"

	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case baseURL + "/" + archiveName:
			return responseWithBytes(req, http.StatusOK, archive), nil
		case baseURL + "/checksums.txt":
			return newTestResponse(req, http.StatusOK, "deadbeef  "+archiveName+"\n"), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	})

	err := updateFrom(baseURL, "gitvista", "v1.0.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestUpdateFrom_ExtractError(t *testing.T) {
	archiveName := ArchiveName("gitvista", "v1.0.0")
	archive := makeTarGz(t, "other-binary", []byte("updated"))
	sum := sha256.Sum256(archive)
	baseURL := "https://example.test/releases/download/v1.0.0"
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)

	withMockHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case baseURL + "/" + archiveName:
			return responseWithBytes(req, http.StatusOK, archive), nil
		case baseURL + "/checksums.txt":
			return newTestResponse(req, http.StatusOK, checksums), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	})

	err := updateFrom(baseURL, "gitvista", "v1.0.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "extracting binary") {
		t.Fatalf("expected extract error, got %v", err)
	}
}

func TestReplaceBinaryAtPath(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "gitvista")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinaryAtPath(execPath, []byte("new")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("got %q, want %q", got, "new")
	}

	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatalf("stat replaced binary: %v", err)
	}
	if info.Mode().Perm() != fs.FileMode(0o755) {
		t.Fatalf("got permissions %o, want 755", info.Mode().Perm())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withMockHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()

	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: fn}
	t.Cleanup(func() {
		http.DefaultClient = originalClient
	})
}

func newTestResponse(req *http.Request, statusCode int, body string) *http.Response {
	return responseWithBytes(req, statusCode, []byte(body))
}

func responseWithBytes(req *http.Request, statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

// makeTarGz creates a tar.gz archive in memory containing a single file.
func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     0o755,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}
