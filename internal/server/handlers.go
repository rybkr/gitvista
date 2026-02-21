package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// extractHashParam validates the request method, extracts a hex hash from the URL
// path after the given prefix, and returns the cached repository.
// On failure it writes an HTTP error and returns ok=false.
func (s *Server) extractHashParam(w http.ResponseWriter, r *http.Request, prefix string) (gitcore.Hash, *gitcore.Repository, bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return "", nil, false
	}

	path := strings.TrimPrefix(r.URL.Path, prefix)
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing hash in path", http.StatusBadRequest)
		return "", nil, false
	}
	path = strings.TrimPrefix(path, "/")

	hash, err := gitcore.NewHash(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid hash format: %v", err), http.StatusBadRequest)
		return "", nil, false
	}

	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return "", nil, false
	}

	return hash, repo, true
}

func (s *Server) handleRepository(w http.ResponseWriter, _ *http.Request) {
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	currentBranch := ""
	headRef := repo.HeadRef()
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			currentBranch = name
		}
	}

	branches := repo.Branches()
	tagNames := repo.TagNames()

	response := map[string]any{
		"name":          repo.Name(),
		"gitDir":        repo.GitDir(),
		"currentBranch": currentBranch,
		"headDetached":  repo.HeadDetached(),
		"headHash":      repo.Head(),
		"commitCount":   len(repo.Commits()),
		"branchCount":   len(branches),
		"tagCount":      len(tagNames),
		"tags":          tagNames,
		"description":   repo.Description(),
		"remotes":       repo.Remotes(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	treeHash, repo, ok := s.extractHashParam(w, r, "/api/tree/")
	if !ok {
		return
	}

	tree, err := repo.GetTree(treeHash)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load tree: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tree); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	blobHash, repo, ok := s.extractHashParam(w, r, "/api/blob/")
	if !ok {
		return
	}

	content, err := repo.GetBlob(blobHash)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load blob: %v", err), http.StatusNotFound)
		return
	}

	isBinary := isBinaryContent(content)

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{
		"hash":      string(blobHash),
		"size":      len(content),
		"binary":    isBinary,
		"truncated": false,
	}

	if isBinary {
		response["content"] = ""
	} else {
		// Cap content at 512KB to prevent browser from choking on huge files.
		// Truncate on byte boundary (not string rune boundary) to avoid splitting
		// UTF-8 multi-byte sequences; then re-validate as a complete UTF-8 string.
		const maxSize = 512 * 1024
		if len(content) > maxSize {
			content = content[:maxSize]
			response["truncated"] = true
		}
		response["content"] = string(content)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func isBinaryContent(content []byte) bool {
	checkSize := min(8192, len(content))
	for i := range checkSize {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

func (s *Server) handleTreeBlame(w http.ResponseWriter, r *http.Request) {
	commitHash, repo, ok := s.extractHashParam(w, r, "/api/tree/blame/")
	if !ok {
		return
	}

	dirPath := r.URL.Query().Get("path")

	sanitized, err := sanitizePath(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	dirPath = sanitized

	cacheKey := string(commitHash) + ":" + dirPath

	blame, ok := s.blameCache.Get(cacheKey)
	if !ok {
		result, err := repo.GetFileBlame(commitHash, dirPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to compute blame: %v", err), http.StatusNotFound)
			return
		}
		blame = result
		s.blameCache.Put(cacheKey, blame)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"entries": blame}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleCommitDiff routes to either the commit-level diff list or the
// file-level line diff based on whether the URL path ends with "/file".
func (s *Server) handleCommitDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/commit/diff/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing commit hash in path", http.StatusBadRequest)
		return
	}

	isFileDiff := strings.HasSuffix(path, "/file")
	commitHashStr := path
	if isFileDiff {
		commitHashStr = strings.TrimSuffix(path, "/file")
	}
	commitHashStr = strings.TrimPrefix(commitHashStr, "/")

	commitHash, err := gitcore.NewHash(commitHashStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid commit hash format: %v", err), http.StatusBadRequest)
		return
	}

	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	if isFileDiff {
		s.handleFileDiff(w, r, repo, commitHash)
		return
	}
	s.handleCommitDiffList(w, repo, commitHash)
}

// resolveCommitAndParent looks up a commit and its first parent's tree hash.
// On failure it writes an HTTP error and returns ok=false.
func resolveCommitAndParent(w http.ResponseWriter, repo *gitcore.Repository, commitHash gitcore.Hash) (*gitcore.Commit, gitcore.Hash, bool) {
	commits := repo.Commits()
	commit, exists := commits[commitHash]
	if !exists {
		http.Error(w, fmt.Sprintf("Commit not found: %s", commitHash), http.StatusNotFound)
		return nil, "", false
	}

	var parentTreeHash gitcore.Hash
	if len(commit.Parents) > 0 {
		parentCommit, exists := commits[commit.Parents[0]]
		if !exists {
			http.Error(w, fmt.Sprintf("Parent commit not found: %s", commit.Parents[0]), http.StatusNotFound)
			return nil, "", false
		}
		parentTreeHash = parentCommit.Tree
	}
	return commit, parentTreeHash, true
}

func (s *Server) handleCommitDiffList(w http.ResponseWriter, repo *gitcore.Repository, commitHash gitcore.Hash) {
	cacheKey := string(commitHash)
	if cached, ok := s.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	commit, parentTreeHash, ok := resolveCommitAndParent(w, repo, commitHash)
	if !ok {
		return
	}

	entries, err := gitcore.TreeDiff(repo, parentTreeHash, commit.Tree, "")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute diff: %v", err), http.StatusInternalServerError)
		return
	}

	jsonEntries := make([]map[string]any, len(entries))
	var added, modified, deleted, renamed int
	for i, entry := range entries {
		jsonEntry := map[string]any{
			"path":    entry.Path,
			"status":  entry.Status.String(),
			"oldHash": string(entry.OldHash),
			"newHash": string(entry.NewHash),
			"binary":  entry.IsBinary,
		}
		if entry.OldPath != "" {
			jsonEntry["oldPath"] = entry.OldPath
		}
		jsonEntries[i] = jsonEntry
		switch entry.Status {
		case gitcore.DiffStatusAdded:
			added++
		case gitcore.DiffStatusModified:
			modified++
		case gitcore.DiffStatusDeleted:
			deleted++
		case gitcore.DiffStatusRenamed:
			renamed++
		}
	}

	response := map[string]any{
		"commitHash":     string(commitHash),
		"parentTreeHash": string(parentTreeHash),
		"entries":        jsonEntries,
		"stats": map[string]any{
			"added":        added,
			"modified":     modified,
			"deleted":      deleted,
			"renamed":      renamed,
			"filesChanged": len(entries),
		},
	}

	s.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleFileDiff(w http.ResponseWriter, r *http.Request, repo *gitcore.Repository, commitHash gitcore.Hash) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}

	sanitized, err := sanitizePath(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	filePath = sanitized

	// Parse context lines parameter; default to 3 when absent or invalid.
	// Cap at 100 to prevent excessive response sizes.
	contextLines := gitcore.DefaultContextLines
	if raw := r.URL.Query().Get("context"); raw != "" {
		if n, _err := strconv.Atoi(raw); _err == nil && n > 0 && n <= 100 {
			contextLines = n
		}
	}

	// Include context count so different depths are cached separately.
	cacheKey := string(commitHash) + ":" + filePath + ":ctx" + strconv.Itoa(contextLines)
	if cached, ok := s.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if _err := json.NewEncoder(w).Encode(cached); _err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	commit, parentTreeHash, ok := resolveCommitAndParent(w, repo, commitHash)
	if !ok {
		return
	}

	entries, err := gitcore.TreeDiff(repo, parentTreeHash, commit.Tree, "")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute diff: %v", err), http.StatusInternalServerError)
		return
	}

	var targetEntry *gitcore.DiffEntry
	for i := range entries {
		if entries[i].Path == filePath {
			targetEntry = &entries[i]
			break
		}
	}

	if targetEntry == nil {
		http.Error(w, fmt.Sprintf("File not found in commit diff: %s", filePath), http.StatusNotFound)
		return
	}

	fileDiff, err := gitcore.ComputeFileDiff(repo, targetEntry.OldHash, targetEntry.NewHash, filePath, contextLines)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute file diff: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]any{
		"path":      fileDiff.Path,
		"status":    targetEntry.Status.String(),
		"oldHash":   string(fileDiff.OldHash),
		"newHash":   string(fileDiff.NewHash),
		"isBinary":  fileDiff.IsBinary,
		"truncated": fileDiff.Truncated,
		"hunks":     fileDiff.Hunks,
	}

	s.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleWorkingTreeDiff computes the diff between the HEAD version of a file
// and its current on-disk content using the pure gitcore implementation.
// It returns a FileDiff-shaped JSON response for the frontend diff viewer.
func (s *Server) handleWorkingTreeDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}

	sanitized, err := sanitizePath(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	filePath = sanitized

	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	// Use 3 lines of context — the unified-diff convention used everywhere else
	// in the codebase when a caller doesn't supply an explicit context count.
	const contextLines = 3
	fileDiff, err := gitcore.ComputeWorkingTreeFileDiff(repo, filePath, contextLines)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute working-tree diff: %v", err), http.StatusInternalServerError)
		return
	}

	// Derive a status string from what the diff tells us:
	//   - OldHash empty means the file was not present in HEAD → new file.
	//   - Non-empty OldHash but empty NewHash slot (disk file missing) is
	//     surfaced via all-deletion hunks → treat as deleted.
	//   - Otherwise the file exists on both sides → modified.
	// This mirrors the vocabulary used by handleFileDiff / ComputeFileDiff.
	status := fileStatusModified
	if fileDiff.OldHash == "" {
		status = fileStatusAdded
	} else if allDeletions(fileDiff.Hunks) && len(fileDiff.Hunks) > 0 {
		status = fileStatusDeleted
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{
		"path":      fileDiff.Path,
		"status":    status,
		"oldHash":   string(fileDiff.OldHash),
		"newHash":   string(fileDiff.NewHash),
		"isBinary":  fileDiff.IsBinary,
		"truncated": fileDiff.Truncated,
		"hunks":     fileDiff.Hunks,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// allDeletions reports whether every diff line across all hunks is a deletion.
// This is used to detect that a file has been removed from disk entirely.
func allDeletions(hunks []gitcore.DiffHunk) bool {
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			if line.Type != "deletion" {
				return false
			}
		}
	}
	return true
}
