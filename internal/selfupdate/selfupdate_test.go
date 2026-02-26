package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
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
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			tag, err := checkLatestFrom(srv.URL)
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

func TestUpdateFlow(t *testing.T) {
	binaryContent := []byte("#!/bin/updated-binary")
	archive := makeTarGz(t, "gitvista", binaryContent)

	h := sha256.Sum256(archive)
	archiveName := ArchiveName("gitvista", "v1.0.0")
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(h[:]), archiveName)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			fmt.Fprint(w, checksums)
		case strings.HasSuffix(r.URL.Path, "/"+archiveName):
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Create a fake executable to be replaced.
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "gitvista")
	if err := os.WriteFile(fakeBin, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Override os.Executable by using the updateFrom function directly
	// and replacing the exec path resolution.
	// Since we can't easily mock os.Executable, test the download/verify/extract
	// portion by calling the internal pieces.
	archiveData, err := httpGetBytes(srv.URL + "/" + archiveName)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	checksumsData, err := httpGetBytes(srv.URL + "/checksums.txt")
	if err != nil {
		t.Fatalf("download checksums: %v", err)
	}

	verifyErr := verifyChecksum(archiveData, checksumsData, archiveName)
	if verifyErr != nil {
		t.Fatalf("verify checksum: %v", verifyErr)
	}

	extracted, err := extractBinary(archiveData, archiveName, "gitvista")
	if err != nil {
		t.Fatalf("extract binary: %v", err)
	}

	if !bytes.Equal(extracted, binaryContent) {
		t.Errorf("extracted binary mismatch: got %q, want %q", extracted, binaryContent)
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
