package gitcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	errBlobNotFound        = errors.New("blob not found in tree")
	errInvalidWorktreePath = errors.New("invalid worktree path")
)

func normalizeWorktreeRelativePath(filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("%w: empty file path", errInvalidWorktreePath)
	}
	if filepath.IsAbs(filePath) {
		return "", fmt.Errorf("%w: absolute paths are not allowed", errInvalidWorktreePath)
	}

	normalized := filepath.ToSlash(filepath.Clean(filePath))
	if normalized == "." || normalized == "" {
		return "", fmt.Errorf("%w: empty file path", errInvalidWorktreePath)
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("%w: path escapes worktree", errInvalidWorktreePath)
	}

	return normalized, nil
}

func resolveWorktreePath(workDir, relativePath string) (string, error) {
	normalizedPath, err := normalizeWorktreeRelativePath(relativePath)
	if err != nil {
		return "", err
	}

	workDirAbs, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path: %w", err)
	}
	diskPath := filepath.Join(workDirAbs, filepath.FromSlash(normalizedPath))
	if err := ensurePathWithinBase(workDirAbs, diskPath); err != nil {
		return "", fmt.Errorf("%w: %s", errInvalidWorktreePath, normalizedPath)
	}
	return diskPath, nil
}

func readWorktreeFile(workDir, relativePath string) ([]byte, error) {
	diskPath, err := resolveWorktreePath(workDir, relativePath)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- path is normalized and constrained to the repository worktree above.
	return os.ReadFile(diskPath)
}

func resolveTreeEntryAtPath(repo *Repository, treeHash Hash, filePath string) (TreeEntry, error) {
	normalizedPath, err := normalizeWorktreeRelativePath(filePath)
	if err != nil {
		return TreeEntry{}, fmt.Errorf("resolveTreeEntryAtPath: %w", err)
	}

	components := strings.Split(normalizedPath, "/")
	currentTreeHash := treeHash

	for _, component := range components[:len(components)-1] {
		tree, treeErr := repo.GetTree(currentTreeHash)
		if treeErr != nil {
			return TreeEntry{}, fmt.Errorf("resolveTreeEntryAtPath: failed to read tree %s: %w", currentTreeHash, treeErr)
		}

		found := false
		for _, entry := range tree.Entries {
			if entry.Name == component {
				if !isTreeEntry(entry) {
					return TreeEntry{}, errBlobNotFound
				}
				currentTreeHash = entry.ID
				found = true
				break
			}
		}
		if !found {
			return TreeEntry{}, errBlobNotFound
		}
	}

	leafName := components[len(components)-1]
	tree, err := repo.GetTree(currentTreeHash)
	if err != nil {
		return TreeEntry{}, fmt.Errorf("resolveTreeEntryAtPath: failed to read leaf tree %s: %w", currentTreeHash, err)
	}

	for _, entry := range tree.Entries {
		if entry.Name == leafName {
			return entry, nil
		}
	}

	return TreeEntry{}, errBlobNotFound
}

func resolveBlobAtPath(repo *Repository, treeHash Hash, filePath string) (Hash, error) {
	entry, err := resolveTreeEntryAtPath(repo, treeHash, filePath)
	if err != nil {
		return "", err
	}
	if isTreeEntry(entry) || isSubmodule(entry) {
		return "", errBlobNotFound
	}
	return entry.ID, nil
}

// ComputeWorkingTreeFileDiff diffs the on-disk content of filePath against HEAD.
// Like ComputeFileDiff, the output is intended to feel close to Git's unified
// diff presentation, but it is produced by GitVista's in-process diff logic
// rather than Git's exact implementation.
func ComputeWorkingTreeFileDiff(repo *Repository, filePath string, contextLines int) (*FileDiff, error) {
	result := &FileDiff{
		Path:  filePath,
		Hunks: make([]DiffHunk, 0),
	}

	normalizedPath, err := normalizeWorktreeRelativePath(filePath)
	if err != nil {
		return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: %w", err)
	}
	result.Path = normalizedPath

	headHash := repo.Head()
	var headContent []byte

	if headHash != "" {
		commits := repo.Commits()
		headCommit, exists := commits[headHash]
		if exists {
			entry, resolveErr := resolveTreeEntryAtPath(repo, headCommit.Tree, normalizedPath)
			if resolveErr != nil && !errors.Is(resolveErr, errBlobNotFound) {
				return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: resolving HEAD entry: %w", resolveErr)
			}

			if resolveErr == nil {
				result.OldHash = entry.ID
				if isSubmodule(entry) {
					result.IsBinary = true
					return result, nil
				}
				if isTreeEntry(entry) {
					return result, nil
				}
				content, readErr := repo.GetBlob(entry.ID)
				if readErr != nil {
					return nil, fmt.Errorf("ComputeWorkingTreeFileDiff: reading HEAD blob: %w", readErr)
				}
				headContent = content
			}
		}
	}

	diskContent, err := readWorktreeFile(repo.WorkDir(), normalizedPath)
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
