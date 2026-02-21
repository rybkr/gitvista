package gitcore

import (
	"crypto/sha1" //nolint:gosec // Git uses SHA-1 for blob hashing
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
)

// FileStatus represents the status of a single file in the working tree.
type FileStatus struct {
	// Path is the slash-separated path relative to the repository root.
	Path string

	// IndexStatus describes the change staged relative to HEAD:
	//   "added"    — new file added to the index
	//   "modified" — file exists in both HEAD and index with different content
	//   "deleted"  — file present in HEAD has been removed from the index
	//   ""         — no staged change (file matches HEAD exactly)
	IndexStatus string

	// WorkStatus describes the change on disk relative to the index:
	//   "modified" — file exists on disk but differs from index content
	//   "deleted"  — file is tracked in the index but absent from disk
	//   ""         — working tree matches index (or file is untracked)
	WorkStatus string

	// IsUntracked is true when the file exists on disk but is not recorded
	// in the index at all. IndexStatus and WorkStatus are empty in this case.
	IsUntracked bool
}

// WorkingTreeStatus is the full working tree status computed without shelling
// out to git. It contains one FileStatus per file that differs from HEAD,
// differs from the index, or is untracked.
type WorkingTreeStatus struct {
	Files []FileStatus
}

// ComputeWorkingTreeStatus computes the status of the working tree by comparing:
//  1. HEAD tree vs index — to identify staged additions, modifications, and deletions.
//  2. Index vs working directory — to identify unstaged modifications and deletions.
//  3. Working directory walk — to identify untracked files.
//
// .gitignore rules are intentionally not applied; untracked files will therefore
// include ignored files. This is acceptable for the current use case.
func ComputeWorkingTreeStatus(repo *Repository) (*WorkingTreeStatus, error) {
	// ------------------------------------------------------------------
	// Step 1: Build a flat map of all blob paths from the HEAD tree.
	// An empty HEAD (fresh repository) results in an empty map.
	// ------------------------------------------------------------------
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
		// If the HEAD commit is not found (e.g., shallow clone edge case),
		// treat it as an empty tree — the same as a fresh repository.
	}

	// ------------------------------------------------------------------
	// Step 2: Read the index (staging area).
	// ------------------------------------------------------------------
	index, err := ReadIndex(repo.GitDir())
	if err != nil {
		return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading index: %w", err)
	}

	// Build a set of all paths currently in the index (stage-0 only).
	// This is used later during the working-directory walk to detect untracked files.
	indexPaths := make(map[string]struct{}, len(index.ByPath))
	for path := range index.ByPath {
		indexPaths[path] = struct{}{}
	}

	// Accumulate results as a map keyed by path so we can update entries
	// when both a staged and an unstaged change apply to the same file.
	results := make(map[string]*FileStatus)

	// ------------------------------------------------------------------
	// Step 3: Compare HEAD tree vs index to detect staged changes.
	// ------------------------------------------------------------------
	for path, entry := range index.ByPath {
		headHash, inHead := headTree[path]

		var idxStatus string
		if !inHead {
			// Path is in the index but not in HEAD → staged addition.
			idxStatus = "added"
		} else if headHash != entry.Hash {
			// Path is in both but hashes differ → staged modification.
			idxStatus = "modified"
		}
		// If hashes match, the staged content is identical to HEAD (idxStatus stays "").

		if idxStatus != "" {
			results[path] = &FileStatus{
				Path:        path,
				IndexStatus: idxStatus,
			}
		}
	}

	// Find paths in HEAD that are no longer in the index → staged deletion.
	for path := range headTree {
		if _, inIndex := index.ByPath[path]; !inIndex {
			results[path] = &FileStatus{
				Path:        path,
				IndexStatus: "deleted",
			}
		}
	}

	// ------------------------------------------------------------------
	// Step 4: Compare index vs working directory to detect unstaged changes.
	// ------------------------------------------------------------------
	workDir := repo.WorkDir()
	for path, entry := range index.ByPath {
		diskPath := filepath.Join(workDir, filepath.FromSlash(path))

		info, statErr := os.Stat(diskPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				// File tracked in the index is gone from disk → unstaged deletion.
				fs, exists := results[path]
				if !exists {
					results[path] = &FileStatus{Path: path}
					fs = results[path]
				}
				fs.WorkStatus = "deleted"
			} else {
				// Unexpected stat error (permission denied, etc.) — surface it.
				return nil, fmt.Errorf("ComputeWorkingTreeStatus: stat %s: %w", diskPath, statErr)
			}
			continue
		}

		// Fast-path: if the file size on disk differs from what the index
		// recorded, skip hashing and mark as modified immediately.
		diskSize := info.Size()
		if uint32(diskSize) != entry.FileSize { //nolint:gosec // diskSize is always non-negative here
			fs, exists := results[path]
			if !exists {
				results[path] = &FileStatus{Path: path}
				fs = results[path]
			}
			fs.WorkStatus = "modified"
			continue
		}

		// Sizes match: compute the git blob hash of the on-disk content and
		// compare against the index hash. This is necessary because two files
		// can be the same size yet have different content.
		//
		//nolint:gosec // G304: path is relative to the repository working directory
		diskContent, readErr := os.ReadFile(diskPath)
		if readErr != nil {
			return nil, fmt.Errorf("ComputeWorkingTreeStatus: reading %s: %w", diskPath, readErr)
		}

		diskHash := hashBlobContent(diskContent)
		if diskHash != entry.Hash {
			fs, exists := results[path]
			if !exists {
				results[path] = &FileStatus{Path: path}
				fs = results[path]
			}
			fs.WorkStatus = "modified"
		}
	}

	// ------------------------------------------------------------------
	// Step 5: Walk the working directory to find untracked files.
	// Files that are in the index are skipped. Only regular files are
	// reported (directories are not listed as untracked entries, matching
	// the general convention from `git status`).
	// ------------------------------------------------------------------
	walkErr := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip directories we cannot read (e.g., permission denied).
            return nil //nolint:nilerr
		}

		// Skip the .git directory entirely — it is not part of the working tree.
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// We only report files, not directories.
		if d.IsDir() {
			return nil
		}

		// Compute the path relative to the working directory, using forward
		// slashes so it matches the index path format on all platforms.
		relPath, relErr := filepath.Rel(workDir, path)
		if relErr != nil {
			// Should never happen since WalkDir starts at workDir.
			return relErr
		}
		relPath = filepath.ToSlash(relPath)

		// If the path is already in the index, it is tracked — not untracked.
		if _, tracked := indexPaths[relPath]; tracked {
			return nil
		}

		// File is not in the index → untracked.
		results[relPath] = &FileStatus{
			Path:        relPath,
			IsUntracked: true,
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("ComputeWorkingTreeStatus: walking work dir: %w", walkErr)
	}

	// ------------------------------------------------------------------
	// Assemble the final result slice from the map.
	// ------------------------------------------------------------------
	status := &WorkingTreeStatus{
		Files: make([]FileStatus, 0, len(results)),
	}
	for _, fs := range results {
		status.Files = append(status.Files, *fs)
	}

	return status, nil
}

// flattenTree recursively walks the tree object identified by treeHash and
// returns a map of every blob path (relative to the repository root, using
// forward slashes) to its blob hash. Subtrees are descended into; blobs and
// submodule gitlinks are recorded as leaves; symlinks are treated as blobs.
//
// prefix is the directory path accumulated during recursion and must start as
// an empty string at the top-level call. It is joined with each entry name
// using a "/" separator.
func flattenTree(repo *Repository, treeHash Hash, prefix string) (map[string]Hash, error) {
	result := make(map[string]Hash)

	tree, err := repo.GetTree(treeHash)
	if err != nil {
		return nil, fmt.Errorf("flattenTree: reading tree %s: %w", treeHash, err)
	}

	for _, entry := range tree.Entries {
		// Build the full slash-separated path for this entry.
		var fullPath string
		if prefix == "" {
			fullPath = entry.Name
		} else {
			fullPath = prefix + "/" + entry.Name
		}

		if isTreeEntry(entry) {
			// Recurse into sub-trees.
			sub, err := flattenTree(repo, entry.ID, fullPath)
			if err != nil {
				return nil, err
			}
			maps.Copy(result, sub)
		} else {
			// Blob, symlink (120000), or gitlink (160000) — record as a leaf.
			// We treat all non-tree entries uniformly: they have a blob hash
			// in the index that we can compare against HEAD.
			result[fullPath] = entry.ID
		}
	}

	return result, nil
}

// hashBlobContent computes the git blob hash for raw file content.
// Git blob objects are stored as "blob <size>\0<content>", and the SHA-1
// of that byte sequence is the canonical object identifier used in the index.
func hashBlobContent(content []byte) Hash {
	// Construct the git blob header: "blob <length>\0".
	header := fmt.Sprintf("blob %d\x00", len(content))

	h := sha1.New() //nolint:gosec // Git uses SHA-1 for blob hashing
	h.Write([]byte(header))
	h.Write(content)

	sum := h.Sum(nil)

	// Encode as a 40-character lowercase hex string, matching Git's convention.
	return Hash(fmt.Sprintf("%x", sum))
}
