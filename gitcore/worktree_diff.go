package gitcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errBlobNotFound = errors.New("blob not found in tree")

func resolveBlobAtPath(repo *Repository, treeHash Hash, filePath string) (Hash, error) {
	filePath = strings.Trim(filePath, "/")
	if filePath == "" {
		return "", fmt.Errorf("resolveBlobAtPath: empty file path")
	}

	components := strings.Split(filePath, "/")
	currentTreeHash := treeHash

	for _, component := range components[:len(components)-1] {
		tree, err := repo.GetTree(currentTreeHash)
		if err != nil {
			return "", fmt.Errorf("resolveBlobAtPath: failed to read tree %s: %w", currentTreeHash, err)
		}

		found := false
		for _, entry := range tree.Entries {
			if entry.Name == component {
				if !isTreeEntry(entry) {
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

	leafName := components[len(components)-1]
	tree, err := repo.GetTree(currentTreeHash)
	if err != nil {
		return "", fmt.Errorf("resolveBlobAtPath: failed to read leaf tree %s: %w", currentTreeHash, err)
	}

	for _, entry := range tree.Entries {
		if entry.Name == leafName {
			if isTreeEntry(entry) {
				return "", errBlobNotFound
			}
			return entry.ID, nil
		}
	}

	return "", errBlobNotFound
}

// ComputeWorkingTreeFileDiff diffs the on-disk content of filePath against HEAD.
func ComputeWorkingTreeFileDiff(repo *Repository, filePath string, contextLines int) (*FileDiff, error) {
	result := &FileDiff{
		Path:  filePath,
		Hunks: make([]DiffHunk, 0),
	}

	headHash := repo.Head()
	var headContent []byte

	if headHash != "" {
		commits := repo.Commits()
		headCommit, exists := commits[headHash]
		if exists {
			blobHash, err := resolveBlobAtPath(repo, headCommit.Tree, filePath)
			if err != nil && !errors.Is(err, errBlobNotFound) {
				return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: resolving HEAD blob: %w", err)
			}

			if err == nil {
				result.OldHash = blobHash
				content, readErr := repo.GetBlob(blobHash)
				if readErr != nil {
					return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: reading HEAD blob: %w", readErr)
				}
				headContent = content
			}
		}
	}

	diskPath := filepath.Join(repo.WorkDir(), filePath)
	diskContent, err := os.ReadFile(diskPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: reading on-disk file: %w", err)
		}
		diskContent = nil
	}

	if headContent == nil && diskContent == nil {
		return result, nil
	}
	if len(headContent) > maxBlobSize || len(diskContent) > maxBlobSize {
		result.Truncated = true
		return result, nil
	}
	if IsBinaryContent(headContent) || IsBinaryContent(diskContent) {
		result.IsBinary = true
		return result, nil
	}

	oldLines := splitLines(headContent)
	newLines := splitLines(diskContent)
	result.Hunks = myersDiff(oldLines, newLines, contextLines)

	return result, nil
}
