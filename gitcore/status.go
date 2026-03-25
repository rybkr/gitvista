package gitcore

import (
	"crypto/sha1" // #nosec G505 -- Git requires SHA-1 for blob hashing
	"fmt"
	"io/fs"
	"math"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// FileStatus represents the status of a single file in the working tree.
type FileStatus struct {
	Path        string
	IndexStatus string
	WorkStatus  string
	IsUntracked bool
	HeadHash    Hash
	IndexHash   Hash
	WorkHash    Hash
}

// WorkingTreeStatus is the full working tree status computed without shelling out to git.
type WorkingTreeStatus struct {
	Files []FileStatus
}

var walkWorktree = filepath.WalkDir

func markWorktreeModified(results map[string]*FileStatus, path string, indexHash Hash) *FileStatus {
	fileStatus, exists := results[path]
	if !exists {
		results[path] = &FileStatus{Path: path}
		fileStatus = results[path]
	}
	fileStatus.WorkStatus = StatusModified
	fileStatus.IndexHash = indexHash
	return fileStatus
}

// ComputeWorkingTreeStatus computes the status of the working tree.
func ComputeWorkingTreeStatus(repo *Repository) (*WorkingTreeStatus, error) {
	headTree := make(map[string]Hash)

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

	results := make(map[string]*FileStatus)
	for path, entry := range index.ByPath {
		headBlobHash, inHead := headTree[path]

		var idxStatus string
		if !inHead {
			idxStatus = StatusAdded
		} else if headBlobHash != entry.Hash {
			idxStatus = StatusModified
		}

		if idxStatus != "" {
			results[path] = &FileStatus{
				Path:        path,
				IndexStatus: idxStatus,
				HeadHash:    headBlobHash,
				IndexHash:   entry.Hash,
			}
		}
	}

	for path, blobHash := range headTree {
		if _, inIndex := index.ByPath[path]; !inIndex {
			results[path] = &FileStatus{
				Path:        path,
				IndexStatus: StatusDeleted,
				HeadHash:    blobHash,
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
		isSymlink := entry.Mode&0170000 == 0120000

		info, statErr := os.Lstat(diskPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				fs, exists := results[path]
				if !exists {
					results[path] = &FileStatus{Path: path}
					fs = results[path]
				}
				fs.WorkStatus = StatusDeleted
				fs.IndexHash = entry.Hash
			} else {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: stat %s: %w", diskPath, statErr)
			}
			continue
		}

		if isSymlink {
			if info.Mode()&os.ModeSymlink == 0 {
				markWorktreeModified(results, path, entry.Hash)
				continue
			}

			linkTarget, linkErr := os.Readlink(diskPath)
			if linkErr != nil {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: readlink %s: %w", diskPath, linkErr)
			}
			diskHash := hashBlobContent([]byte(linkTarget))
			if diskHash != entry.Hash {
				fs := markWorktreeModified(results, path, entry.Hash)
				fs.WorkHash = diskHash
			}
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			markWorktreeModified(results, path, entry.Hash)
			continue
		}

		diskSize := info.Size()
		if diskSize < 0 || diskSize > math.MaxUint32 || uint32(diskSize) != entry.FileSize {
			sizeContent, sizeReadErr := readWorktreeFile(workDir, normalizedPath)
			if sizeReadErr != nil {
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading %s: %w", diskPath, sizeReadErr)
			}
			fs := markWorktreeModified(results, path, entry.Hash)
			fs.WorkHash = hashBlobContent(sizeContent)
			continue
		}

		diskContent, readErr := readWorktreeFile(workDir, normalizedPath)
		if readErr != nil {
			return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading %s: %w", diskPath, readErr)
		}

		diskHash := hashBlobContent(diskContent)
		if diskHash != entry.Hash {
			fs := markWorktreeModified(results, path, entry.Hash)
			fs.WorkHash = diskHash
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
				if ignore.isIgnored(relPath, true) {
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

		results[relPath] = &FileStatus{
			Path:        relPath,
			IsUntracked: true,
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("ComputeWorkingTreeStatus: walking work dir: %w", walkErr)
	}

	status := &WorkingTreeStatus{
		Files: make([]FileStatus, 0, len(results)),
	}
	for _, fs := range results {
		status.Files = append(status.Files, *fs)
	}
	slices.SortFunc(status.Files, func(a, b FileStatus) int {
		return compareStrings(a.Path, b.Path)
	})

	return status, nil
}

func flattenTree(repo *Repository, treeHash Hash, prefix string) (map[string]Hash, error) {
	result := make(map[string]Hash)

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
			result[fullPath] = entry.ID
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
