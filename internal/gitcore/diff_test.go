package gitcore

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo is a helper to create a test repository with synthetic objects.
func setupTestRepo(t *testing.T) *Repository {
	t.Helper()

	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")

	// Create .git structure
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		t.Fatalf("failed to create objects dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		t.Fatalf("failed to create refs dir: %v", err)
	}

	// Create HEAD
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("failed to create HEAD: %v", err)
	}

	repo := &Repository{
		gitDir:      gitDir,
		workDir:     tmpDir,
		packIndices: make([]*PackIndex, 0),
		refs:        make(map[string]Hash),
		commits:     make([]*Commit, 0),
	}

	return repo
}

// createBlob is a helper to create a loose blob object.
func createBlob(t *testing.T, repo *Repository, content []byte) Hash {
	t.Helper()

	// Create blob object
	header := fmt.Sprintf("blob %d\x00", len(content))
	data := append([]byte(header), content...)

	// Compress with zlib
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("failed to compress blob: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zlib writer: %v", err)
	}

	// Compute hash (simplified - just use first 40 hex chars of content hash)
	hash := fmt.Sprintf("%040x", sha1Sum(data))
	hashObj := Hash(hash)

	// Write to objects directory
	objDir := filepath.Join(repo.gitDir, "objects", hash[:2])
	if err := os.MkdirAll(objDir, 0755); err != nil {
		t.Fatalf("failed to create object dir: %v", err)
	}

	objPath := filepath.Join(objDir, hash[2:])
	if err := os.WriteFile(objPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write blob object: %v", err)
	}

	return hashObj
}

// createTree is a helper to create a tree object.
func createTree(t *testing.T, repo *Repository, entries []TreeEntry) Hash {
	t.Helper()

	// Build tree content
	var buf bytes.Buffer
	for _, entry := range entries {
		// mode name\0hash(20 bytes)
		buf.WriteString(entry.Mode)
		buf.WriteByte(' ')
		buf.WriteString(entry.Name)
		buf.WriteByte(0)

		// Write hash bytes
		hashBytes := make([]byte, 20)
		for i := 0; i < 20; i++ {
			fmt.Sscanf(string(entry.ID[i*2:i*2+2]), "%02x", &hashBytes[i])
		}
		buf.Write(hashBytes)
	}

	content := buf.Bytes()

	// Create tree object
	header := fmt.Sprintf("tree %d\x00", len(content))
	data := append([]byte(header), content...)

	// Compress with zlib
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("failed to compress tree: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zlib writer: %v", err)
	}

	// Compute hash
	hash := fmt.Sprintf("%040x", sha1Sum(data))
	hashObj := Hash(hash)

	// Write to objects directory
	objDir := filepath.Join(repo.gitDir, "objects", hash[:2])
	if err := os.MkdirAll(objDir, 0755); err != nil {
		t.Fatalf("failed to create object dir: %v", err)
	}

	objPath := filepath.Join(objDir, hash[2:])
	if err := os.WriteFile(objPath, compressed.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write tree object: %v", err)
	}

	return hashObj
}

// sha1Sum returns a simple SHA-1 sum for testing (not cryptographically secure, just for unique hashes).
func sha1Sum(data []byte) []byte {
	// Simplified hash - mix all bytes to create unique hash
	hash := make([]byte, 20)

	// Initialize with data length
	for i := 0; i < 20; i++ {
		hash[i] = byte((len(data) >> (i % 8)) & 0xFF)
	}

	// Mix in all data bytes
	for i, b := range data {
		hash[i%20] ^= b
		hash[(i+1)%20] ^= byte(i)
		hash[(i+7)%20] = (hash[(i+7)%20] + b) & 0xFF
	}

	return hash
}

func TestTreeDiff_AddedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create blobs
	fileContent := []byte("Hello, World!")
	fileHash := createBlob(t, repo, fileContent)

	// Create old tree (empty)
	oldTree := createTree(t, repo, []TreeEntry{})

	// Create new tree with one file
	newTree := createTree(t, repo, []TreeEntry{
		{
			ID:   fileHash,
			Name: "hello.txt",
			Mode: "100644",
			Type: "blob",
		},
	})

	// Compute diff
	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Status != DiffStatusAdded {
		t.Errorf("expected status Added, got %s", entry.Status)
	}
	if entry.Path != "hello.txt" {
		t.Errorf("expected path hello.txt, got %s", entry.Path)
	}
	if entry.NewHash != fileHash {
		t.Errorf("expected hash %s, got %s", fileHash, entry.NewHash)
	}
}

func TestTreeDiff_DeletedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create blobs
	fileContent := []byte("Hello, World!")
	fileHash := createBlob(t, repo, fileContent)

	// Create old tree with one file
	oldTree := createTree(t, repo, []TreeEntry{
		{
			ID:   fileHash,
			Name: "hello.txt",
			Mode: "100644",
			Type: "blob",
		},
	})

	// Create new tree (empty)
	newTree := createTree(t, repo, []TreeEntry{})

	// Compute diff
	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Status != DiffStatusDeleted {
		t.Errorf("expected status Deleted, got %s", entry.Status)
	}
	if entry.Path != "hello.txt" {
		t.Errorf("expected path hello.txt, got %s", entry.Path)
	}
	if entry.OldHash != fileHash {
		t.Errorf("expected hash %s, got %s", fileHash, entry.OldHash)
	}
}

func TestTreeDiff_ModifiedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create blobs
	oldContent := []byte("Hello, World!")
	oldHash := createBlob(t, repo, oldContent)

	newContent := []byte("Hello, Universe!")
	newHash := createBlob(t, repo, newContent)

	// Create old tree
	oldTree := createTree(t, repo, []TreeEntry{
		{
			ID:   oldHash,
			Name: "hello.txt",
			Mode: "100644",
			Type: "blob",
		},
	})

	// Create new tree
	newTree := createTree(t, repo, []TreeEntry{
		{
			ID:   newHash,
			Name: "hello.txt",
			Mode: "100644",
			Type: "blob",
		},
	})

	// Compute diff
	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Status != DiffStatusModified {
		t.Errorf("expected status Modified, got %s", entry.Status)
	}
	if entry.Path != "hello.txt" {
		t.Errorf("expected path hello.txt, got %s", entry.Path)
	}
	if entry.OldHash != oldHash {
		t.Errorf("expected old hash %s, got %s", oldHash, entry.OldHash)
	}
	if entry.NewHash != newHash {
		t.Errorf("expected new hash %s, got %s", newHash, entry.NewHash)
	}
}

func TestTreeDiff_RootCommit(t *testing.T) {
	repo := setupTestRepo(t)

	// Create blob
	fileContent := []byte("Initial commit")
	fileHash := createBlob(t, repo, fileContent)

	// Create tree
	newTree := createTree(t, repo, []TreeEntry{
		{
			ID:   fileHash,
			Name: "README.md",
			Mode: "100644",
			Type: "blob",
		},
	})

	// Compute diff with empty old tree (root commit)
	entries, err := TreeDiff(repo, "", newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Status != DiffStatusAdded {
		t.Errorf("expected status Added, got %s", entry.Status)
	}
	if entry.Path != "README.md" {
		t.Errorf("expected path README.md, got %s", entry.Path)
	}
}

func TestTreeDiff_NestedDirectories(t *testing.T) {
	repo := setupTestRepo(t)

	// Create blobs
	file1 := createBlob(t, repo, []byte("file1"))
	file2 := createBlob(t, repo, []byte("file2"))

	// Create nested tree
	innerTree := createTree(t, repo, []TreeEntry{
		{
			ID:   file2,
			Name: "inner.txt",
			Mode: "100644",
			Type: "blob",
		},
	})

	// Create old tree (empty)
	oldTree := createTree(t, repo, []TreeEntry{})

	// Create new tree with nested structure
	newTree := createTree(t, repo, []TreeEntry{
		{
			ID:   file1,
			Name: "root.txt",
			Mode: "100644",
			Type: "blob",
		},
		{
			ID:   innerTree,
			Name: "subdir",
			Mode: "040000",
			Type: "tree",
		},
	})

	// Compute diff
	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Check both files were found
	paths := make(map[string]bool)
	for _, entry := range entries {
		paths[entry.Path] = true
		if entry.Status != DiffStatusAdded {
			t.Errorf("expected status Added for %s, got %s", entry.Path, entry.Status)
		}
	}

	if !paths["root.txt"] {
		t.Error("expected root.txt in diff")
	}
	if !paths["subdir/inner.txt"] {
		t.Error("expected subdir/inner.txt in diff")
	}
}

func TestTreeDiff_ExactRenameDetection(t *testing.T) {
	repo := setupTestRepo(t)

	// Same content moved to a new path => should be detected as rename
	content := []byte("package main\n\nfunc hello() {}\n")
	blobHash := createBlob(t, repo, content)

	oldTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "hello.go", Mode: "100644", Type: "blob"},
	})
	newTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "world.go", Mode: "100644", Type: "blob"},
	})

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (rename), got %d: %+v", len(entries), entries)
	}
	if entries[0].Status != DiffStatusRenamed {
		t.Errorf("expected Renamed, got %s", entries[0].Status)
	}
	if entries[0].Path != "world.go" {
		t.Errorf("expected new path 'world.go', got %q", entries[0].Path)
	}
	if entries[0].OldPath != "hello.go" {
		t.Errorf("expected old path 'hello.go', got %q", entries[0].OldPath)
	}
	if entries[0].OldHash != blobHash {
		t.Errorf("expected OldHash to match blob hash")
	}
}

func TestTreeDiff_ModifiedRenameNotDetected(t *testing.T) {
	repo := setupTestRepo(t)

	// Different content => different hashes => NOT detected as rename
	oldBlob := createBlob(t, repo, []byte("version 1\n"))
	newBlob := createBlob(t, repo, []byte("version 2\n"))

	oldTree := createTree(t, repo, []TreeEntry{
		{ID: oldBlob, Name: "old.txt", Mode: "100644", Type: "blob"},
	})
	newTree := createTree(t, repo, []TreeEntry{
		{ID: newBlob, Name: "new.txt", Mode: "100644", Type: "blob"},
	})

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (delete+add), got %d", len(entries))
	}
	statuses := map[string]bool{}
	for _, e := range entries {
		statuses[e.Status.String()] = true
	}
	if !statuses["added"] || !statuses["deleted"] {
		t.Errorf("expected added and deleted statuses, got %v", statuses)
	}
}

func TestDetectRenames_NoDeletedEntries(t *testing.T) {
	// When there are no deleted entries, entries should be returned unchanged.
	entries := []DiffEntry{
		{Path: "a.txt", Status: DiffStatusAdded, NewHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{Path: "b.txt", Status: DiffStatusModified, OldHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", NewHash: "cccccccccccccccccccccccccccccccccccccccc"},
	}
	result := detectRenames(entries)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries unchanged, got %d", len(result))
	}
	if result[0].Status != DiffStatusAdded || result[1].Status != DiffStatusModified {
		t.Errorf("statuses changed unexpectedly: %v, %v", result[0].Status, result[1].Status)
	}
}

func TestTreeDiff_Submodule(t *testing.T) {
	repo := setupTestRepo(t)

	// Create submodule entry (mode 160000)
	submoduleHash := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	// Create old tree (empty)
	oldTree := createTree(t, repo, []TreeEntry{})

	// Create new tree with submodule
	newTree := createTree(t, repo, []TreeEntry{
		{
			ID:   submoduleHash,
			Name: "submodule",
			Mode: "160000",
			Type: "commit",
		},
	})

	// Compute diff
	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if !entry.IsBinary {
		t.Error("expected submodule to be marked as binary")
	}
}

func TestComputeFileDiff_AddedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create new blob
	newContent := []byte("line 1\nline 2\nline 3\n")
	newHash := createBlob(t, repo, newContent)

	// Compute diff (no old file)
	diff, err := ComputeFileDiff(repo, "", newHash, "test.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff failed: %v", err)
	}

	if diff.Path != "test.txt" {
		t.Errorf("expected path test.txt, got %s", diff.Path)
	}
	if diff.NewHash != newHash {
		t.Errorf("expected hash %s, got %s", newHash, diff.NewHash)
	}

	// Check hunks
	if len(diff.Hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Count additions
	additions := 0
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == "addition" {
				additions++
			}
		}
	}

	if additions != 3 {
		t.Errorf("expected 3 additions, got %d", additions)
	}
}

func TestComputeFileDiff_DeletedFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create old blob
	oldContent := []byte("line 1\nline 2\nline 3\n")
	oldHash := createBlob(t, repo, oldContent)

	// Compute diff (no new file)
	diff, err := ComputeFileDiff(repo, oldHash, "", "test.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff failed: %v", err)
	}

	if diff.Path != "test.txt" {
		t.Errorf("expected path test.txt, got %s", diff.Path)
	}
	if diff.OldHash != oldHash {
		t.Errorf("expected hash %s, got %s", oldHash, diff.OldHash)
	}

	// Count deletions
	deletions := 0
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == "deletion" {
				deletions++
			}
		}
	}

	if deletions != 3 {
		t.Errorf("expected 3 deletions, got %d", deletions)
	}
}

func TestComputeFileDiff_ModifiedLines(t *testing.T) {
	repo := setupTestRepo(t)

	// Create blobs
	oldContent := []byte("line 1\nline 2\nline 3\n")
	oldHash := createBlob(t, repo, oldContent)

	newContent := []byte("line 1\nmodified line 2\nline 3\n")
	newHash := createBlob(t, repo, newContent)

	// Compute diff
	diff, err := ComputeFileDiff(repo, oldHash, newHash, "test.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff failed: %v", err)
	}

	if len(diff.Hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Check for deletion and addition
	hasDelete := false
	hasAdd := false

	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == "deletion" && strings.Contains(line.Content, "line 2") {
				hasDelete = true
			}
			if line.Type == "addition" && strings.Contains(line.Content, "modified line 2") {
				hasAdd = true
			}
		}
	}

	if !hasDelete {
		t.Error("expected deletion of 'line 2'")
	}
	if !hasAdd {
		t.Error("expected addition of 'modified line 2'")
	}
}

func TestComputeFileDiff_BinaryFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create binary content (with null bytes)
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	binaryHash := createBlob(t, repo, binaryContent)

	// Compute diff
	diff, err := ComputeFileDiff(repo, "", binaryHash, "binary.bin", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff failed: %v", err)
	}

	if !diff.IsBinary {
		t.Error("expected file to be marked as binary")
	}

	if len(diff.Hunks) != 0 {
		t.Error("expected no hunks for binary file")
	}
}

func TestComputeFileDiff_LargeFile(t *testing.T) {
	repo := setupTestRepo(t)

	// Create large content (> 512KB)
	largeContent := bytes.Repeat([]byte("x"), 600*1024)
	largeHash := createBlob(t, repo, largeContent)

	// Compute diff
	diff, err := ComputeFileDiff(repo, "", largeHash, "large.txt", DefaultContextLines)
	if err != nil {
		t.Fatalf("ComputeFileDiff failed: %v", err)
	}

	if !diff.Truncated {
		t.Error("expected file to be marked as truncated")
	}
}

func TestMyersDiff_NoChanges(t *testing.T) {
	oldLines := []string{"line 1", "line 2", "line 3"}
	newLines := []string{"line 1", "line 2", "line 3"}

	hunks := myersDiff(oldLines, newLines, 3)

	if len(hunks) != 0 {
		t.Errorf("expected no hunks for identical files, got %d", len(hunks))
	}
}

func TestMyersDiff_SimpleAddition(t *testing.T) {
	oldLines := []string{"line 1", "line 3"}
	newLines := []string{"line 1", "line 2", "line 3"}

	hunks := myersDiff(oldLines, newLines, 3)

	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Find the addition
	hasAddition := false
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			if line.Type == "addition" && line.Content == "line 2" {
				hasAddition = true
			}
		}
	}

	if !hasAddition {
		t.Error("expected addition of 'line 2'")
	}
}

func TestMyersDiff_SimpleDeletion(t *testing.T) {
	oldLines := []string{"line 1", "line 2", "line 3"}
	newLines := []string{"line 1", "line 3"}

	hunks := myersDiff(oldLines, newLines, 3)

	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	// Find the deletion
	hasDeletion := false
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			if line.Type == "deletion" && line.Content == "line 2" {
				hasDeletion = true
			}
		}
	}

	if !hasDeletion {
		t.Error("expected deletion of 'line 2'")
	}
}

func TestMyersDiff_ContextLines(t *testing.T) {
	oldLines := []string{"ctx1", "ctx2", "old line", "ctx3", "ctx4"}
	newLines := []string{"ctx1", "ctx2", "new line", "ctx3", "ctx4"}

	hunks := myersDiff(oldLines, newLines, 2)

	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	hunk := hunks[0]

	// Count context lines
	contextCount := 0
	for _, line := range hunk.Lines {
		if line.Type == "context" {
			contextCount++
		}
	}

	// Should have context before and after (up to 2 lines each)
	if contextCount < 2 {
		t.Errorf("expected at least 2 context lines, got %d", contextCount)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []string
	}{
		{
			name:     "empty",
			input:    []byte{},
			expected: []string{},
		},
		{
			name:     "single line no newline",
			input:    []byte("hello"),
			expected: []string{"hello"},
		},
		{
			name:     "single line with newline",
			input:    []byte("hello\n"),
			expected: []string{"hello"},
		},
		{
			name:     "multiple lines",
			input:    []byte("line1\nline2\nline3"),
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "multiple lines with trailing newline",
			input:    []byte("line1\nline2\nline3\n"),
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "empty lines",
			input:    []byte("line1\n\nline3"),
			expected: []string{"line1", "", "line3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d lines, got %d", len(tt.expected), len(result))
			}
			for i, line := range result {
				if line != tt.expected[i] {
					t.Errorf("line %d: expected %q, got %q", i, tt.expected[i], line)
				}
			}
		})
	}
}

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "text content",
			content:  []byte("Hello, World!"),
			expected: false,
		},
		{
			name:     "binary with null byte",
			content:  []byte{0x00, 0x01, 0x02},
			expected: true,
		},
		{
			name:     "text with null byte",
			content:  []byte("Hello\x00World"),
			expected: true,
		},
		{
			name:     "empty",
			content:  []byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinaryContent(tt.content)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDiffStatus_String(t *testing.T) {
	tests := []struct {
		status   DiffStatus
		expected string
	}{
		{DiffStatusAdded, "added"},
		{DiffStatusModified, "modified"},
		{DiffStatusDeleted, "deleted"},
		{DiffStatusRenamed, "renamed"},
		{DiffStatus(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.status.String()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// B2: rename detection — additional cases not covered by the basic tests

// TestDetectRenames_EmptyInput verifies that an empty entry slice is handled
// without panic and returns an empty (non-nil) slice.
func TestDetectRenames_EmptyInput(t *testing.T) {
	result := detectRenames([]DiffEntry{})
	if result == nil {
		t.Fatal("detectRenames returned nil for empty input; expected empty slice")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

// TestDetectRenames_MultipleRenames verifies that two independent renames in the
// same diff are both detected. This exercises the matched-set logic to ensure the
// first rename does not consume the deleted-entry index used by the second.
func TestDetectRenames_MultipleRenames(t *testing.T) {
	hashA := Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	entries := []DiffEntry{
		{Path: "old_a.go", Status: DiffStatusDeleted, OldHash: hashA, OldMode: "100644"},
		{Path: "old_b.go", Status: DiffStatusDeleted, OldHash: hashB, OldMode: "100644"},
		{Path: "new_a.go", Status: DiffStatusAdded, NewHash: hashA},
		{Path: "new_b.go", Status: DiffStatusAdded, NewHash: hashB},
	}

	result := detectRenames(entries)

	// Two deletes should be removed; two adds should be promoted to renames.
	if len(result) != 2 {
		t.Fatalf("expected 2 rename entries, got %d: %+v", len(result), result)
	}

	renames := map[string]string{} // newPath -> oldPath
	for _, e := range result {
		if e.Status != DiffStatusRenamed {
			t.Errorf("entry %q: expected Renamed, got %s", e.Path, e.Status)
		}
		renames[e.Path] = e.OldPath
	}

	if renames["new_a.go"] != "old_a.go" {
		t.Errorf("new_a.go should have OldPath old_a.go, got %q", renames["new_a.go"])
	}
	if renames["new_b.go"] != "old_b.go" {
		t.Errorf("new_b.go should have OldPath old_b.go, got %q", renames["new_b.go"])
	}
}

// TestDetectRenames_RenamePreservesOldMode verifies that the OldMode from the
// deleted entry is carried forward onto the promoted rename entry.
func TestDetectRenames_RenamePreservesOldMode(t *testing.T) {
	hash := Hash("cccccccccccccccccccccccccccccccccccccccc")

	entries := []DiffEntry{
		{Path: "script.sh", Status: DiffStatusDeleted, OldHash: hash, OldMode: "100755"},
		{Path: "run.sh", Status: DiffStatusAdded, NewHash: hash, NewMode: "100755"},
	}

	result := detectRenames(entries)

	if len(result) != 1 {
		t.Fatalf("expected 1 rename entry, got %d", len(result))
	}
	if result[0].OldMode != "100755" {
		t.Errorf("OldMode not preserved: got %q, want %q", result[0].OldMode, "100755")
	}
	if result[0].OldHash != hash {
		t.Errorf("OldHash not set on rename: got %q, want %q", result[0].OldHash, hash)
	}
}

// TestDetectRenames_OneRenameOneDeletion verifies the mixed case where one file
// is renamed (matching add exists) and another file is simply deleted (no matching
// add). The deleted-with-no-match must remain as a DiffStatusDeleted entry.
func TestDetectRenames_OneRenameOneDeletion(t *testing.T) {
	matchedHash := Hash("dddddddddddddddddddddddddddddddddddddddd")
	orphanHash := Hash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")

	entries := []DiffEntry{
		// This delete has a matching add — should become a rename.
		{Path: "old_name.go", Status: DiffStatusDeleted, OldHash: matchedHash, OldMode: "100644"},
		// This delete has NO matching add — must remain as deleted.
		{Path: "gone_forever.go", Status: DiffStatusDeleted, OldHash: orphanHash, OldMode: "100644"},
		// The matching add for old_name.go.
		{Path: "new_name.go", Status: DiffStatusAdded, NewHash: matchedHash},
	}

	result := detectRenames(entries)

	// Expect: 1 rename (new_name.go <- old_name.go) + 1 deletion (gone_forever.go).
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(result), result)
	}

	byPath := map[string]DiffEntry{}
	for _, e := range result {
		byPath[e.Path] = e
	}

	renamed, ok := byPath["new_name.go"]
	if !ok {
		t.Fatal("expected new_name.go in result")
	}
	if renamed.Status != DiffStatusRenamed {
		t.Errorf("new_name.go: expected Renamed, got %s", renamed.Status)
	}
	if renamed.OldPath != "old_name.go" {
		t.Errorf("new_name.go: expected OldPath old_name.go, got %q", renamed.OldPath)
	}

	deleted, ok := byPath["gone_forever.go"]
	if !ok {
		t.Fatal("expected gone_forever.go to remain in result as Deleted")
	}
	if deleted.Status != DiffStatusDeleted {
		t.Errorf("gone_forever.go: expected Deleted, got %s", deleted.Status)
	}
}

// TestDetectRenames_NoAddedEntries verifies that when only deletes are present
// (no adds to match against) the entries are returned unchanged.
func TestDetectRenames_NoAddedEntries(t *testing.T) {
	entries := []DiffEntry{
		{Path: "a.go", Status: DiffStatusDeleted, OldHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{Path: "b.go", Status: DiffStatusDeleted, OldHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	}

	result := detectRenames(entries)

	if len(result) != 2 {
		t.Fatalf("expected 2 entries unchanged, got %d", len(result))
	}
	for _, e := range result {
		if e.Status != DiffStatusDeleted {
			t.Errorf("entry %q: expected Deleted, got %s", e.Path, e.Status)
		}
	}
}

// TestDetectRenames_DuplicateHashNotDoubleClaimed verifies that when two added
// files share the same blob hash but only one deleted file matches that hash,
// only one rename is produced; the other add stays as DiffStatusAdded.
func TestDetectRenames_DuplicateHashNotDoubleClaimed(t *testing.T) {
	sharedHash := Hash("ffffffffffffffffffffffffffffffffffffffff")

	entries := []DiffEntry{
		// Only one deleted file with sharedHash.
		{Path: "original.go", Status: DiffStatusDeleted, OldHash: sharedHash, OldMode: "100644"},
		// Two added files with the same hash.
		{Path: "copy_one.go", Status: DiffStatusAdded, NewHash: sharedHash},
		{Path: "copy_two.go", Status: DiffStatusAdded, NewHash: sharedHash},
	}

	result := detectRenames(entries)

	// Exactly one rename and one remaining add; the delete must be consumed.
	renames := 0
	adds := 0
	for _, e := range result {
		switch e.Status {
		case DiffStatusRenamed:
			renames++
		case DiffStatusAdded:
			adds++
		default:
			t.Errorf("unexpected status %s for entry %q", e.Status, e.Path)
		}
	}

	if renames != 1 {
		t.Errorf("expected exactly 1 rename, got %d", renames)
	}
	if adds != 1 {
		t.Errorf("expected exactly 1 remaining add, got %d", adds)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 total entries, got %d: %+v", len(result), result)
	}
}

// TestTreeDiff_MultipleRenames exercises the full TreeDiff pipeline (not just
// detectRenames directly) with two files renamed in the same commit so we can
// confirm the integration path from tree comparison through rename detection.
func TestTreeDiff_MultipleRenames(t *testing.T) {
	repo := setupTestRepo(t)

	contentA := []byte("package api\n\nfunc HandlerA() {}\n")
	contentB := []byte("package api\n\nfunc HandlerB() {}\n")
	hashA := createBlob(t, repo, contentA)
	hashB := createBlob(t, repo, contentB)

	oldTree := createTree(t, repo, []TreeEntry{
		{ID: hashA, Name: "handler_a_old.go", Mode: "100644", Type: "blob"},
		{ID: hashB, Name: "handler_b_old.go", Mode: "100644", Type: "blob"},
	})
	newTree := createTree(t, repo, []TreeEntry{
		{ID: hashA, Name: "handler_a_new.go", Mode: "100644", Type: "blob"},
		{ID: hashB, Name: "handler_b_new.go", Mode: "100644", Type: "blob"},
	})

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 rename entries, got %d: %+v", len(entries), entries)
	}

	renames := map[string]string{} // newPath -> oldPath
	for _, e := range entries {
		if e.Status != DiffStatusRenamed {
			t.Errorf("entry %q: expected Renamed, got %s", e.Path, e.Status)
		}
		renames[e.Path] = e.OldPath
	}

	if renames["handler_a_new.go"] != "handler_a_old.go" {
		t.Errorf("handler_a_new.go: unexpected OldPath %q", renames["handler_a_new.go"])
	}
	if renames["handler_b_new.go"] != "handler_b_old.go" {
		t.Errorf("handler_b_new.go: unexpected OldPath %q", renames["handler_b_new.go"])
	}
}

// TestTreeDiff_CrossDirectoryRename verifies that rename detection works when
// a file moves between two different directories within the same commit. The
// old entry appears under "src/" and the new entry appears under "lib/", so
// the rename spans a directory boundary and can only be detected if
// detectRenames runs on the complete flat list — not per-directory.
func TestTreeDiff_CrossDirectoryRename(t *testing.T) {
	repo := setupTestRepo(t)

	content := []byte("package util\n\nfunc Helper() {}\n")
	blobHash := createBlob(t, repo, content)

	// Old tree: src/util.go
	srcTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "util.go", Mode: "100644", Type: "blob"},
	})
	oldRoot := createTree(t, repo, []TreeEntry{
		{ID: srcTree, Name: "src", Mode: "040000", Type: "tree"},
	})

	// New tree: lib/util.go (same blob, different directory)
	libTree := createTree(t, repo, []TreeEntry{
		{ID: blobHash, Name: "util.go", Mode: "100644", Type: "blob"},
	})
	newRoot := createTree(t, repo, []TreeEntry{
		{ID: libTree, Name: "lib", Mode: "040000", Type: "tree"},
	})

	entries, err := TreeDiff(repo, oldRoot, newRoot, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 rename entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Status != DiffStatusRenamed {
		t.Errorf("expected Renamed, got %s", entries[0].Status)
	}
	if entries[0].Path != "lib/util.go" {
		t.Errorf("expected new path 'lib/util.go', got %q", entries[0].Path)
	}
	if entries[0].OldPath != "src/util.go" {
		t.Errorf("expected old path 'src/util.go', got %q", entries[0].OldPath)
	}
}

// TestTreeDiff_RenameAndDeletion exercises the full TreeDiff pipeline for the
// mixed case: one file renamed, one file deleted with no matching add. This
// validates that the pipeline does not accidentally absorb unmatched deletes.
func TestTreeDiff_RenameAndDeletion(t *testing.T) {
	repo := setupTestRepo(t)

	renamedContent := []byte("// renamed file\n")
	deletedContent := []byte("// this file is gone\n")
	renamedHash := createBlob(t, repo, renamedContent)
	deletedHash := createBlob(t, repo, deletedContent)

	oldTree := createTree(t, repo, []TreeEntry{
		{ID: renamedHash, Name: "before.go", Mode: "100644", Type: "blob"},
		{ID: deletedHash, Name: "removed.go", Mode: "100644", Type: "blob"},
	})
	newTree := createTree(t, repo, []TreeEntry{
		{ID: renamedHash, Name: "after.go", Mode: "100644", Type: "blob"},
		// removed.go is gone; no matching add
	})

	entries, err := TreeDiff(repo, oldTree, newTree, "")
	if err != nil {
		t.Fatalf("TreeDiff failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (1 rename + 1 delete), got %d: %+v", len(entries), entries)
	}

	byPath := map[string]DiffEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}

	if r, ok := byPath["after.go"]; !ok {
		t.Error("expected after.go (rename) in result")
	} else if r.Status != DiffStatusRenamed {
		t.Errorf("after.go: expected Renamed, got %s", r.Status)
	} else if r.OldPath != "before.go" {
		t.Errorf("after.go: expected OldPath before.go, got %q", r.OldPath)
	}

	if d, ok := byPath["removed.go"]; !ok {
		t.Error("expected removed.go (delete) in result")
	} else if d.Status != DiffStatusDeleted {
		t.Errorf("removed.go: expected Deleted, got %s", d.Status)
	}
}
