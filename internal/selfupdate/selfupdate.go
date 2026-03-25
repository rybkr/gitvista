// Package selfupdate provides lightweight self-update functionality for GitVista binaries using only Go stdlib. It
// queries GitHub releases for the latest version, downloads the archive and checksums, verifies integrity, and performs
// an atomic binary replacement.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var replaceBinaryFunc = replaceBinary

// Release represents the minimal fields from a GitHub release API response.
type Release struct {
	TagName string `json:"tag_name"`
}

// CheckLatest queries the GitHub releases API for the latest release tag.
func CheckLatest(repo string) (string, error) {
	return checkLatestFrom(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
}

func checkLatestFrom(url string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- URL derived from trusted repo name
	if err != nil {
		return "", fmt.Errorf("checking latest version: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("decoding release: %w", err)
	}

	if rel.TagName == "" {
		return "", fmt.Errorf("empty tag in release response")
	}

	return rel.TagName, nil
}

// NeedsUpdate reports whether the current version should be updated.
func NeedsUpdate(current, latest string) bool {
	if current == "dev" || current == "" {
		return false
	}
	return strings.TrimPrefix(current, "v") != strings.TrimPrefix(latest, "v")
}

// ArchiveName returns the platform-specific archive filename for a release (e.g. gitvista_1.2.3_darwin_arm64.tar.gz).
func ArchiveName(project, version string) string {
	v := strings.TrimPrefix(version, "v")
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", project, v, runtime.GOOS, runtime.GOARCH, ext)
}

// Update downloads the release archive from GitHub, verifies its SHA-256 checksum against the checksums file, extracts
// the named binary, and atomically replaces the currently running executable.
func Update(repo, project, version string) error {
	return updateFrom(
		fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, version),
		project, version,
	)
}

func updateFrom(baseURL, project, version string) error {
	archive := ArchiveName(project, version)
	archiveURL := baseURL + "/" + archive
	checksumsURL := baseURL + "/checksums.txt"

	archiveData, err := httpGetBytes(archiveURL)
	if err != nil {
		return fmt.Errorf("downloading archive: %w", err)
	}

	checksumsData, err := httpGetBytes(checksumsURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	verifyErr := verifyChecksum(archiveData, checksumsData, archive)
	if verifyErr != nil {
		return verifyErr
	}

	binaryData, err := extractBinary(archiveData, archive, project)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	return replaceBinaryFunc(binaryData)
}

func replaceBinary(binaryData []byte) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	return replaceBinaryAtPath(execPath, binaryData)
}

func replaceBinaryAtPath(execPath string, binaryData []byte) error {
	execInfo, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}

	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".gitvista-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if _, writeErr := tmp.Write(binaryData); writeErr != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing temp file: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		cleanup()
		return fmt.Errorf("closing temp file: %w", closeErr)
	}
	if renameErr := os.Rename(tmpPath, execPath); renameErr != nil { // #nosec G703 -- execPath is resolved from os.Executable
		cleanup()
		return fmt.Errorf("replacing binary: %w", renameErr)
	}
	if chmodErr := os.Chmod(execPath, execInfo.Mode().Perm()); chmodErr != nil { // #nosec G302,G703 -- replacement binary must preserve the existing executable mode after atomic rename
		return fmt.Errorf("restoring permissions: %w", chmodErr)
	}

	return nil
}

func httpGetBytes(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- URL derived from trusted base URL
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

// verifyChecksum checks that the SHA-256 hash of data matches the entry for
// filename in the checksums file content.
func verifyChecksum(data, checksums []byte, filename string) error {
	expected, err := findChecksum(checksums, filename)
	if err != nil {
		return err
	}

	h := sha256.Sum256(data)
	actual := hex.EncodeToString(h[:])

	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s:\n  expected: %s\n  actual:   %s", filename, expected, actual)
	}
	return nil
}

func findChecksum(checksums []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %s", filename)
}

// extractBinary pulls the named binary out of a tar.gz or zip archive held entirely in memory.
func extractBinary(archiveData []byte, archiveName, binaryName string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractFromZip(archiveData, binaryName)
	}
	return extractFromTarGz(archiveData, binaryName)
}

func extractFromTarGz(data []byte, binaryName string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer func() {
		_ = gr.Close()
	}()

	tr := tar.NewReader(gr)
	for {
		hdr, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("tar reader: %w", nextErr)
		}
		if filepath.Base(hdr.Name) == binaryName && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binaryName)
}

func extractFromZip(data []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip reader: %w", err)
	}
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base == binaryName+".exe" || base == binaryName {
			rc, openErr := f.Open()
			if openErr != nil {
				return nil, openErr
			}
			content, readErr := io.ReadAll(rc)
			_ = rc.Close()
			return content, readErr
		}
	}
	return nil, fmt.Errorf("binary %q not found in zip archive", binaryName)
}
