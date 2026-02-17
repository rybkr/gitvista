package server

import (
	"encoding/json"
	"fmt"
	"github.com/rybkr/gitvista/internal/gitcore"
	"net/http"
	"strings"
)

// extractHashParam extracts and validates a hash parameter from the URL path.
// It performs method validation (GET only), path extraction, hash parsing, and repository retrieval.
// Returns the parsed hash, repository, and a boolean indicating success.
// If validation fails, appropriate HTTP errors are written to the ResponseWriter.
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

// handleRepository serves repository metadata via REST API.
// Used for initial page load and debugging.
func (s *Server) handleRepository(w http.ResponseWriter, _ *http.Request) {
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	response := map[string]any{
		"name":   repo.Name(),
		"gitDir": repo.GitDir(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleTree serves tree object data via REST API.
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

// handleBlob serves raw blob content via REST API.
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

	// Detect if content is binary by scanning for null bytes in first 8KB
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
		// Cap content at 512KB to prevent browser from choking on huge files
		maxSize := 512 * 1024
		text := string(content)
		if len(text) > maxSize {
			text = text[:maxSize]
			response["truncated"] = true
		}
		response["content"] = text
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// isBinaryContent checks if content appears to be binary by looking for null bytes
// in the first 8KB. This matches Git's heuristic for binary detection.
func isBinaryContent(content []byte) bool {
	checkSize := min(8192, len(content))
	for i := range checkSize {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

// handleTreeBlame serves per-file blame information for a directory at a given commit.
// Path format: /api/tree/blame/{commitHash}?path={dirPath}
// Returns a map of entry names to BlameEntry structs with last-modifying commit info.
func (s *Server) handleTreeBlame(w http.ResponseWriter, r *http.Request) {
	commitHash, repo, ok := s.extractHashParam(w, r, "/api/tree/blame/")
	if !ok {
		return
	}

	// Parse directory path from query parameter (default to empty string for root)
	dirPath := r.URL.Query().Get("path")

	// Validate and sanitize the path to prevent directory traversal
	sanitized, err := sanitizePath(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	dirPath = sanitized

	// Build cache key
	cacheKey := string(commitHash) + ":" + dirPath

	// Check cache first
	if cached, ok := s.blameCache.Load(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{
			"entries": cached,
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Cache miss, compute blame
	blame, err := repo.GetFileBlame(commitHash, dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute blame: %v", err), http.StatusNotFound)
		return
	}

	// Store in cache
	s.blameCache.Store(cacheKey, blame)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{
		"entries": blame,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleCommitDiff serves commit diff information via REST API.
// Routes:
//   - GET /api/commit/diff/{commitHash} - Returns list of changed files (CommitDiff)
//   - GET /api/commit/diff/{commitHash}/file?path={path} - Returns line-level diff for a specific file (FileDiff)
//
// The handler determines the type of request based on the URL path suffix.
func (s *Server) handleCommitDiff(w http.ResponseWriter, r *http.Request) {
	// Only GET method allowed
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the URL path to extract commit hash and determine request type
	path := strings.TrimPrefix(r.URL.Path, "/api/commit/diff/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing commit hash in path", http.StatusBadRequest)
		return
	}

	// Check if this is a file-level diff request
	isFileDiff := strings.Contains(path, "/file")

	// Extract commit hash (everything before "/file" if present)
	commitHashStr := path
	if isFileDiff {
		commitHashStr = strings.TrimSuffix(path, "/file")
	}
	commitHashStr = strings.TrimPrefix(commitHashStr, "/")

	// Validate commit hash format
	commitHash, err := gitcore.NewHash(commitHashStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid commit hash format: %v", err), http.StatusBadRequest)
		return
	}

	// Get repository
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	// Handle file-level diff request
	if isFileDiff {
		s.handleFileDiff(w, r, repo, commitHash)
		return
	}

	// Handle commit-level diff request (list of changed files)
	s.handleCommitDiffList(w, repo, commitHash)
}

// handleCommitDiffList handles GET /api/commit/diff/{commitHash}
// Returns a list of all files changed in the commit.
func (s *Server) handleCommitDiffList(w http.ResponseWriter, repo *gitcore.Repository, commitHash gitcore.Hash) {
	// Check cache first
	cacheKey := string(commitHash)
	if cached, ok := s.diffCache.Load(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Get the commit
	commits := repo.Commits()
	commit, exists := commits[commitHash]
	if !exists {
		http.Error(w, fmt.Sprintf("Commit not found: %s", commitHash), http.StatusNotFound)
		return
	}

	// Determine parent hash (empty for root commits)
	var parentTreeHash gitcore.Hash
	if len(commit.Parents) > 0 {
		// Use first parent for merge commits
		parentCommit, exists := commits[commit.Parents[0]]
		if !exists {
			http.Error(w, fmt.Sprintf("Parent commit not found: %s", commit.Parents[0]), http.StatusNotFound)
			return
		}
		parentTreeHash = parentCommit.Tree
	}

	// Compute tree diff
	entries, err := gitcore.TreeDiff(repo, parentTreeHash, commit.Tree, "")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute diff: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert DiffEntry slice to JSON-friendly format and compute stats
	jsonEntries := make([]map[string]any, len(entries))
	stats := map[string]int{
		"added":    0,
		"modified": 0,
		"deleted":  0,
	}

	for i, entry := range entries {
		jsonEntries[i] = map[string]any{
			"path":    entry.Path,
			"status":  entry.Status.String(),
			"oldHash": string(entry.OldHash),
			"newHash": string(entry.NewHash),
			"binary":  entry.IsBinary,
		}

		// Update stats
		switch entry.Status {
		case gitcore.DiffStatusAdded:
			stats["added"]++
		case gitcore.DiffStatusModified:
			stats["modified"]++
		case gitcore.DiffStatusDeleted:
			stats["deleted"]++
		}
	}

	// Build response
	response := map[string]any{
		"commitHash": string(commitHash),
		"parentHash": string(parentTreeHash),
		"entries":    jsonEntries,
		"stats": map[string]any{
			"added":        stats["added"],
			"modified":     stats["modified"],
			"deleted":      stats["deleted"],
			"filesChanged": len(entries),
		},
	}

	// Cache the response (commit hashes are immutable)
	s.diffCache.Store(cacheKey, response)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleFileDiff handles GET /api/commit/diff/{commitHash}/file?path={path}
// Returns line-level diff for a specific file in the commit.
func (s *Server) handleFileDiff(w http.ResponseWriter, r *http.Request, repo *gitcore.Repository, commitHash gitcore.Hash) {
	// Get file path from query parameter
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}

	// Validate and sanitize path
	sanitized, err := sanitizePath(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	filePath = sanitized

	// Build cache key
	cacheKey := string(commitHash) + ":" + filePath

	// Check cache first
	if cached, ok := s.diffCache.Load(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Get the commit
	commits := repo.Commits()
	commit, exists := commits[commitHash]
	if !exists {
		http.Error(w, fmt.Sprintf("Commit not found: %s", commitHash), http.StatusNotFound)
		return
	}

	// Determine parent tree hash
	var parentTreeHash gitcore.Hash
	if len(commit.Parents) > 0 {
		parentCommit, exists := commits[commit.Parents[0]]
		if !exists {
			http.Error(w, fmt.Sprintf("Parent commit not found: %s", commit.Parents[0]), http.StatusNotFound)
			return
		}
		parentTreeHash = parentCommit.Tree
	}

	// First, compute tree diff to find the file's blob hashes
	entries, err := gitcore.TreeDiff(repo, parentTreeHash, commit.Tree, "")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute diff: %v", err), http.StatusInternalServerError)
		return
	}

	// Find the specific file in the diff entries
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

	// Compute file-level diff
	fileDiff, err := gitcore.ComputeFileDiff(repo, targetEntry.OldHash, targetEntry.NewHash, filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute file diff: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to JSON-friendly format
	response := map[string]any{
		"path":      fileDiff.Path,
		"status":    targetEntry.Status.String(),
		"oldHash":   string(fileDiff.OldHash),
		"newHash":   string(fileDiff.NewHash),
		"isBinary":  fileDiff.IsBinary,
		"truncated": fileDiff.Truncated,
		"hunks":     fileDiff.Hunks,
	}

	// Cache the response
	s.diffCache.Store(cacheKey, response)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
