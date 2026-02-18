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

// Helper to create a test repository with synthetic objects
func setupTestRepo(t *testing.T) (*Repository, string) {
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
		gitDir:       gitDir,
		workDir:      tmpDir,
		packIndices:  make([]*PackIndex, 0),
		refs:         make(map[string]Hash),
		commits:      make([]*Commit, 0),
	}

	return repo, gitDir
}

// Helper to create a loose blob object
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

// Helper to create a tree object
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

// Simple SHA-1 sum for testing (not cryptographically secure, just for unique hashes)
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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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

func TestTreeDiff_Submodule(t *testing.T) {
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
	repo, _ := setupTestRepo(t)

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
