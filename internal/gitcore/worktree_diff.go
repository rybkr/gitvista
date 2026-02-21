package gitcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// errBlobNotFound is a sentinel used internally when a path does not exist in
// the HEAD tree. The caller uses errors.Is to distinguish "file not tracked"
// from unexpected I/O errors.
var errBlobNotFound = errors.New("blob not found in tree")

// resolveBlobAtPath walks the tree rooted at treeHash to find the blob for the
// given filePath (e.g., "internal/gitcore/diff.go"). It splits the path into
// components, descends through nested tree objects for all but the final
// component, then returns the blob hash of the leaf entry.
//
// Returns errBlobNotFound when any component of the path does not exist in the
// tree, or when the final component refers to a tree rather than a blob.
func resolveBlobAtPath(repo *Repository, treeHash Hash, filePath string) (Hash, error) {
	// Normalise: strip leading/trailing slashes and collapse any empty segments.
	filePath = strings.Trim(filePath, "/")
	if filePath == "" {
		return "", fmt.Errorf("resolveBlobAtPath: empty file path")
	}

	components := strings.Split(filePath, "/")
	currentTreeHash := treeHash

	// Walk all directory components except the final filename.
	for _, component := range components[:len(components)-1] {
		tree, err := repo.GetTree(currentTreeHash)
		if err != nil {
			return "", fmt.Errorf("resolveBlobAtPath: failed to read tree %s: %w", currentTreeHash, err)
		}

		found := false
		for _, entry := range tree.Entries {
			if entry.Name == component {
				if !isTreeEntry(entry) {
					// Path component exists but is a blob, not a directory.
					return "", errBlobNotFound
				}
				currentTreeHash = entry.ID
				found = true
				break
			}
		}
		if !found {
			return "", errBlobNotFound
		}
	}

	// Read the tree that should contain the final filename.
	leafName := components[len(components)-1]
	tree, err := repo.GetTree(currentTreeHash)
	if err != nil {
		return "", fmt.Errorf("resolveBlobAtPath: failed to read leaf tree %s: %w", currentTreeHash, err)
	}

	for _, entry := range tree.Entries {
		if entry.Name == leafName {
			if isTreeEntry(entry) {
				// The path points to a directory, not a file.
				return "", errBlobNotFound
			}
			return entry.ID, nil
		}
	}

	return "", errBlobNotFound
}

// ComputeWorkingTreeFileDiff diffs the on-disk content of filePath against the
// version recorded in the HEAD commit, using the same Myers diff engine that
// ComputeFileDiff uses for commit-to-commit diffs. This replaces the previous
// "git diff HEAD -- <file>" shell-out in the server's working-tree diff handler.
//
// filePath must be relative to the repository root (e.g., "cmd/vista/main.go").
// contextLines controls how many unchanged lines to include around each hunk.
//
// Edge cases:
//   - HEAD is unset (empty repo): treated as new file — all on-disk lines are
//     additions.
//   - File not tracked by HEAD: treated as new file.
//   - File absent on disk but present in HEAD: treated as deleted — all HEAD
//     lines are deletions.
//   - Either side is binary: IsBinary is set and no hunks are returned.
//   - Either side exceeds maxBlobSize: Truncated is set and no hunks are
//     returned.
func ComputeWorkingTreeFileDiff(repo *Repository, filePath string, contextLines int) (*FileDiff, error) {
	result := &FileDiff{
		Path:  filePath,
		Hunks: make([]DiffHunk, 0),
	}

	headHash := repo.Head()
	var headContent []byte // nil means the file is not in HEAD

	if headHash != "" {
		// Look up the HEAD commit to obtain its tree hash.
		commits := repo.Commits()
		headCommit, exists := commits[headHash]
		if !exists {
			// HEAD points to a commit we haven't loaded — treat as new file.
			// This can happen in shallow clones or edge cases; we don't error
			// out because the on-disk diff is still meaningful.
			headContent = nil
		} else {
			blobHash, err := resolveBlobAtPath(repo, headCommit.Tree, filePath)
			if err != nil && !errors.Is(err, errBlobNotFound) {
				// Unexpected error (I/O, corrupt object, etc.) — surface it.
				return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: resolving HEAD blob: %w", err)
			}

			if err == nil {
				// File exists in HEAD — read its content.
				result.OldHash = blobHash
				content, readErr := repo.GetBlob(blobHash)
				if readErr != nil {
					return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: reading HEAD blob: %w", readErr)
				}
				headContent = content
			}
			// errBlobNotFound: file is not tracked in HEAD → headContent stays nil.
		}
	}

	diskPath := filepath.Join(repo.WorkDir(), filePath)
	//nolint:gosec // G304: path sanitized by the server handler before reaching here
	diskContent, err := os.ReadFile(diskPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: reading on-disk file: %w", err)
		}
		// File has been deleted from disk.
		diskContent = nil
	}

	// If neither side exists there is nothing to diff.
	if headContent == nil && diskContent == nil {
		return result, nil
	}

	if len(headContent) > maxBlobSize || len(diskContent) > maxBlobSize {
		result.Truncated = true
		return result, nil
	}

	if isBinaryContent(headContent) || isBinaryContent(diskContent) {
		result.IsBinary = true
		return result, nil
	}

	oldLines := splitLines(headContent)
	newLines := splitLines(diskContent)
	result.Hunks = myersDiff(oldLines, newLines, contextLines)

	return result, nil
}
