package gitcore

import (
	"crypto/sha1" // #nosec G505 -- Git requires SHA-1 for blob hashing
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ChangeType represents the kind of change applied to a file.
type ChangeType int

// nolint:revive // See: https://git-scm.com/docs/git-status
const (
	ChangeTypeUntracked ChangeType = iota
	ChangeTypeAdded
	ChangeTypeModified
	ChangeTypeDeleted
	ChangeTypeRenamed
	ChangeTypeCopied
	ChangeTypeTypeChanged
)

var changeTypeNames = map[ChangeType]string{
	ChangeTypeUntracked:   "untracked",
	ChangeTypeAdded:       "added",
	ChangeTypeModified:    "modified",
	ChangeTypeDeleted:     "deleted",
	ChangeTypeRenamed:     "renamed",
	ChangeTypeCopied:      "copied",
	ChangeTypeTypeChanged: "typechanged",
}

// String returns the string representation of a ChangeType.
func (c ChangeType) String() string {
	if name, ok := changeTypeNames[c]; ok {
		return name
	}
	return "unknown"
}

// FileState represents the state of a single file across the three Git trees.
type FileState struct {
	Path           string
	StagedChange   ChangeType
	UnstagedChange ChangeType
	IsUntracked    bool
	HeadHash       Hash
	StagedHash     Hash
	WorktreeHash   Hash
}

// WorkingTreeStatus is the full working tree status computed without shelling out to git.
type WorkingTreeStatus struct {
	Files []FileState
}

type treeFile struct {
	Hash Hash
	Mode string
}

var walkWorktree = filepath.WalkDir

func markWorktreeModified(results map[string]*FileState, path string, stagedHash Hash) *FileState {
	fileState, exists := results[path]
	if !exists {
		results[path] = &FileState{Path: path}
		fileState = results[path]
	}
	fileState.UnstagedChange = ChangeTypeModified
	fileState.StagedHash = stagedHash
	return fileState
}

func trackedGitlinkPaths(index *Index) map[string]struct{} {
	paths := make(map[string]struct{})
	for path, entry := range index.ByPath {
		if entryModeKind(indexModeString(entry.Mode)) == "gitlink" {
			paths[path] = struct{}{}
		}
	}
	return paths
}

// ComputeWorkingTreeStatus computes the status of the working tree.
// Ignore handling is implemented in-process for common Git semantics, including
// repository-local .gitignore files, .git/info/exclude, and core.excludesFile.
// It is intentionally not a complete reimplementation of Git's ignore engine,
// so callers should treat edge-case parity with Git as best-effort rather than
// exhaustive.
func ComputeWorkingTreeStatus(repo *Repository) (*WorkingTreeStatus, error) {
	headTree := make(map[string]treeFile)

	headHash := repo.Head()
	if headHash != "" {
		commits := repo.Commits()
		headCommit, ok := commits[headHash]
		if ok {
			var err error
			headTree, err = flattenTree(repo, headCommit.Tree, "")
			if err != nil {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: flattening HEAD tree: %w", err)
			}
		}
	}

	index, err := ReadIndex(repo.GitDir())
	if err != nil {
		return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading index: %w", err)
	}

	indexPaths := make(map[string]struct{}, len(index.ByPath))
	for path := range index.ByPath {
		indexPaths[path] = struct{}{}
	}
	gitlinkPaths := trackedGitlinkPaths(index)

	results := make(map[string]*FileState)
	for path, entry := range index.ByPath {
		headFile, inHead := headTree[path]

		var stagedChange ChangeType
		var hasStagedChange bool
		if !inHead {
			stagedChange = ChangeTypeAdded
			hasStagedChange = true
		} else {
			stagedChange, hasStagedChange = compareTreeAndIndex(headFile, *entry)
		}

		if hasStagedChange {
			results[path] = &FileState{
				Path:         path,
				StagedChange: stagedChange,
				HeadHash:     headFile.Hash,
				StagedHash:   entry.Hash,
			}
		}
	}

	for path, file := range headTree {
		if _, inIndex := index.ByPath[path]; !inIndex {
			results[path] = &FileState{
				Path:         path,
				StagedChange: ChangeTypeDeleted,
				HeadHash:     file.Hash,
			}
		}
	}

	workDir := repo.WorkDir()
	for path, entry := range index.ByPath {
		normalizedPath, err := normalizeWorktreeRelativePath(path)
		if err != nil {
			return nil, fmt.Errorf("ComputeWorkingTreeStatus: invalid index path %q: %w", path, err)
		}
		diskPath, err := resolveWorktreePath(workDir, normalizedPath)
		if err != nil {
			return nil, fmt.Errorf("ComputeWorkingTreeStatus: invalid worktree path %q: %w", normalizedPath, err)
		}
		entryMode := indexModeString(entry.Mode)
		isSymlink := entryModeKind(entryMode) == "symlink"
		isGitlink := entryModeKind(entryMode) == "gitlink"

		info, statErr := os.Lstat(diskPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				fileState, exists := results[path]
				if !exists {
					results[path] = &FileState{Path: path}
					fileState = results[path]
				}
				fileState.UnstagedChange = ChangeTypeDeleted
				fileState.StagedHash = entry.Hash
			} else {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: stat %s: %w", diskPath, statErr)
			}
			continue
		}

		if isGitlink {
			if !info.IsDir() {
				fileState := markWorktreeModified(results, path, entry.Hash)
				fileState.UnstagedChange = ChangeTypeTypeChanged
			}
			continue
		}

		diskMode := worktreeModeString(info)
		if worktreeTypeStatus(entryMode, diskMode) == ChangeTypeTypeChanged {
			fileState := markWorktreeModified(results, path, entry.Hash)
			fileState.UnstagedChange = ChangeTypeTypeChanged
			continue
		}
		if diskMode != entryMode {
			markWorktreeModified(results, path, entry.Hash)
		}

		if isSymlink {
			linkTarget, linkErr := os.Readlink(diskPath)
			if linkErr != nil {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: readlink %s: %w", diskPath, linkErr)
			}
			diskHash := hashBlobContent([]byte(linkTarget))
			if diskHash != entry.Hash {
				fileState := markWorktreeModified(results, path, entry.Hash)
				fileState.WorktreeHash = diskHash
			}
			continue
		}

		if !info.Mode().IsRegular() {
			markWorktreeModified(results, path, entry.Hash)
			continue
		}

		diskSize := info.Size()
		if diskSize < 0 || diskSize > math.MaxUint32 || uint32(diskSize) != entry.FileSize {
			sizeContent, sizeReadErr := readWorktreeFile(workDir, normalizedPath)
			if sizeReadErr != nil {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading %s: %w", diskPath, sizeReadErr)
			}
			fileState := markWorktreeModified(results, path, entry.Hash)
			fileState.WorktreeHash = hashBlobContent(sizeContent)
			continue
		}

		diskContent, readErr := readWorktreeFile(workDir, normalizedPath)
		if readErr != nil {
			return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading %s: %w", diskPath, readErr)
		}

		diskHash := hashBlobContent(diskContent)
		if diskHash != entry.Hash {
			fileState := markWorktreeModified(results, path, entry.Hash)
			fileState.WorktreeHash = diskHash
		}
	}

	ignore := loadIgnoreMatcher(workDir, repo.GitDir())
	walkErr := walkWorktree(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		relPath, relErr := filepath.Rel(workDir, path)
		if relErr != nil {
			return relErr
		}
		relPath = filepath.ToSlash(relPath)

		if d.IsDir() {
			if relPath != "." {
				if _, trackedGitlink := gitlinkPaths[relPath]; trackedGitlink {
					return filepath.SkipDir
				}
				hasTrackedDescendants := hasTrackedPathPrefix(indexPaths, relPath+"/")
				if ignore.isIgnored(relPath, true) && !hasTrackedDescendants {
					return filepath.SkipDir
				}
				if !hasTrackedDescendants {
					results[relPath+"/"] = &FileState{
						Path:        relPath + "/",
						IsUntracked: true,
					}
					return filepath.SkipDir
				}
				ignore.loadFile(workDir, relPath+"/")
			}
			return nil
		}

		if ignore.isIgnored(relPath, false) {
			return nil
		}
		if _, tracked := indexPaths[relPath]; tracked {
			return nil
		}

		results[relPath] = &FileState{
			Path:        relPath,
			IsUntracked: true,
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("ComputeWorkingTreeStatus: walking work dir: %w", walkErr)
	}

	status := &WorkingTreeStatus{
		Files: make([]FileState, 0, len(results)),
	}
	for _, fileState := range results {
		status.Files = append(status.Files, *fileState)
	}
	slices.SortFunc(status.Files, func(a, b FileState) int {
		switch {
		case a.IsUntracked && !b.IsUntracked:
			return 1
		case !a.IsUntracked && b.IsUntracked:
			return -1
		}
		return strings.Compare(a.Path, b.Path)
	})

	return status, nil
}

func flattenTree(repo *Repository, treeHash Hash, prefix string) (map[string]treeFile, error) {
	result := make(map[string]treeFile)

	tree, err := repo.GetTree(treeHash)
	if err != nil {
		return nil, fmt.Errorf("flattenTree: reading tree %s: %w", treeHash, err)
	}

	for _, entry := range tree.Entries {
		fullPath := entry.Name
		if prefix != "" {
			fullPath = prefix + "/" + entry.Name
		}

		if isTreeEntry(entry) {
			sub, err := flattenTree(repo, entry.ID, fullPath)
			if err != nil {
				return nil, err
			}
			maps.Copy(result, sub)
		} else {
			result[fullPath] = treeFile{Hash: entry.ID, Mode: normalizeTreeMode(entry.Mode)}
		}
	}

	return result, nil
}

func hashBlobContent(content []byte) Hash {
	header := fmt.Sprintf("blob %d\x00", len(content))

	h := sha1.New() // #nosec G401
	h.Write([]byte(header))
	h.Write(content)

	return Hash(fmt.Sprintf("%x", h.Sum(nil)))
}

func compareTreeAndIndex(headFile treeFile, entry IndexEntry) (ChangeType, bool) {
	indexMode := indexModeString(entry.Mode)
	if worktreeTypeStatus(headFile.Mode, indexMode) == ChangeTypeTypeChanged {
		return ChangeTypeTypeChanged, true
	}
	if headFile.Mode != indexMode || headFile.Hash != entry.Hash {
		return ChangeTypeModified, true
	}
	return 0, false
}

func worktreeTypeStatus(expectedMode, actualMode string) ChangeType {
	if entryModeKind(expectedMode) != entryModeKind(actualMode) {
		return ChangeTypeTypeChanged
	}
	return 0
}

func entryModeKind(mode string) string {
	switch normalizeTreeMode(mode) {
	case "100644", "100755":
		return "regular"
	case "120000":
		return "symlink"
	case "160000":
		return "gitlink"
	case "040000":
		return "tree"
	default:
		return mode
	}
}

func normalizeTreeMode(mode string) string {
	switch mode {
	case "40000":
		return "040000"
	default:
		return mode
	}
}

func indexModeString(mode uint32) string {
	switch mode & 0170000 {
	case 0100000:
		if mode&0o111 != 0 {
			return "100755"
		}
		return "100644"
	case 0120000:
		return "120000"
	case 0160000:
		return "160000"
	case 0040000:
		return "040000"
	default:
		return fmt.Sprintf("%06o", mode&0o177777)
	}
}

func worktreeModeString(info fs.FileInfo) string {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return "120000"
	case info.Mode().IsRegular():
		if info.Mode().Perm()&0o111 != 0 {
			return "100755"
		}
		return "100644"
	case info.IsDir():
		return "040000"
	default:
		return info.Mode().String()
	}
}

func hasTrackedPathPrefix(indexPaths map[string]struct{}, prefix string) bool {
	for path := range indexPaths {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
