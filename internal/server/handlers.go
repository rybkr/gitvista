package server

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
	"net/http"
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

	// Build current branch name from HEAD ref
	currentBranch := ""
	headRef := repo.HeadRef()
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			currentBranch = name
		}
	}

	// Get branches and tags for counts
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

	// Parse context lines parameter; default to 3 when absent or invalid
	contextLines := gitcore.DefaultContextLines
	if raw := r.URL.Query().Get("context"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			contextLines = n
		}
	}

	// Build cache key; include context count so different depths are cached separately
	cacheKey := string(commitHash) + ":" + filePath + ":ctx" + strconv.Itoa(contextLines)

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
	fileDiff, err := gitcore.ComputeFileDiff(repo, targetEntry.OldHash, targetEntry.NewHash, filePath, contextLines)
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

// handleWorkingTreeDiff serves a unified diff of unstaged/staged working-tree changes
// for a single file relative to HEAD.
// Route: GET /api/working-tree/diff?path={filePath}
//
// Shells out to "git diff HEAD -- <path>" following the same pattern used in status.go.
// Returns a JSON object compatible with the FileDiff shape expected by diffContentViewer.js.
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

	// Validate the path to prevent directory traversal
	sanitized, err := sanitizePath(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	filePath = sanitized

	// Obtain the working directory from the cached repository
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	// Shell out to git diff HEAD, following the same pattern as status.go.
	// "git diff HEAD" captures both staged and unstaged changes relative to the last commit.
	cmd := exec.Command("git", "diff", "HEAD", "--", filePath)
	cmd.Dir = repo.WorkDir()
	out, err := cmd.Output()
	if err != nil {
		// Non-zero exit can mean the file is untracked (no history); treat as empty diff
		out = []byte{}
	}

	// If there is no output the file is either untracked or identical to HEAD
	if len(strings.TrimSpace(string(out))) == 0 {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{
			"path":      filePath,
			"status":    "untracked",
			"isBinary":  false,
			"truncated": false,
			"hunks":     []any{},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Parse the raw unified diff output into our FileDiff structure
	fileDiff := parseUnifiedDiff(string(out), filePath)

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{
		"path":      fileDiff.Path,
		"status":    "modified",
		"isBinary":  fileDiff.IsBinary,
		"truncated": fileDiff.Truncated,
		"hunks":     fileDiff.Hunks,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// parseUnifiedDiff converts the text output of "git diff" into a FileDiff struct.
// It handles the standard unified diff format produced by git:
//
//	diff --git a/file b/file
//	--- a/file
//	+++ b/file
//	@@ -oldStart,oldLines +newStart,newLines @@
//	 context line
//	-deleted line
//	+added line
func parseUnifiedDiff(raw, filePath string) *gitcore.FileDiff {
	result := &gitcore.FileDiff{
		Path:  filePath,
		Hunks: make([]gitcore.DiffHunk, 0),
	}

	// Check for binary marker in the diff header
	if strings.Contains(raw, "Binary files") || strings.Contains(raw, "GIT binary patch") {
		result.IsBinary = true
		return result
	}

	lines := strings.Split(raw, "\n")
	var currentHunk *gitcore.DiffHunk
	oldLine := 0
	newLine := 0

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@"):
			// Finalize any open hunk before starting a new one
			if currentHunk != nil {
				result.Hunks = append(result.Hunks, *currentHunk)
			}
			currentHunk = &gitcore.DiffHunk{
				Lines: make([]gitcore.DiffLine, 0),
			}
			// Parse "@@ -oldStart,oldLines +newStart,newLines @@"
			// We extract start positions to correctly track running line numbers
			var oStart, oCount, nStart, nCount int
			_, scanErr := fmt.Sscanf(line, "@@ -%d,%d +%d,%d @@", &oStart, &oCount, &nStart, &nCount)
			if scanErr != nil {
				// Try single-line hunk format "@@ -N +N @@"
				oCount = 1
				nCount = 1
				fmt.Sscanf(line, "@@ -%d +%d @@", &oStart, &nStart) //nolint:errcheck
			}
			currentHunk.OldStart = oStart
			currentHunk.NewStart = nStart
			oldLine = oStart
			newLine = nStart

		case currentHunk == nil:
			// Skip header lines before the first hunk (diff --git, ---, +++ lines)
			continue

		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			currentHunk.Lines = append(currentHunk.Lines, gitcore.DiffLine{
				Type:    "deletion",
				Content: line[1:],
				OldLine: oldLine,
				NewLine: 0,
			})
			currentHunk.OldLines++
			oldLine++

		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			currentHunk.Lines = append(currentHunk.Lines, gitcore.DiffLine{
				Type:    "addition",
				Content: line[1:],
				OldLine: 0,
				NewLine: newLine,
			})
			currentHunk.NewLines++
			newLine++

		case strings.HasPrefix(line, " "):
			// Context line (leading space)
			currentHunk.Lines = append(currentHunk.Lines, gitcore.DiffLine{
				Type:    "context",
				Content: line[1:],
				OldLine: oldLine,
				NewLine: newLine,
			})
			currentHunk.OldLines++
			currentHunk.NewLines++
			oldLine++
			newLine++
		}
	}

	// Finalize the last open hunk
	if currentHunk != nil {
		result.Hunks = append(result.Hunks, *currentHunk)
	}

	return result
}
