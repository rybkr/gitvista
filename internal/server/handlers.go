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
		blame, err = repo.GetFileBlame(commitHash, dirPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to compute blame: %v", err), http.StatusNotFound)
			return
		}
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
		"commitHash": string(commitHash),
		"parentTreeHash": string(parentTreeHash),
		"entries":    jsonEntries,
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
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			contextLines = n
		}
	}

	// Include context count so different depths are cached separately.
	cacheKey := string(commitHash) + ":" + filePath + ":ctx" + strconv.Itoa(contextLines)
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

// handleWorkingTreeDiff shells out to "git diff HEAD" for a single file and
// returns a FileDiff-shaped JSON response for the frontend diff viewer.
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

	// "git diff HEAD" captures both staged and unstaged changes relative to the last commit.
	cmd := exec.Command("git", "diff", "HEAD", "--", filePath)
	cmd.Dir = repo.WorkDir()
	out, err := cmd.Output()
	if err != nil {
		out = []byte{}
	}

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

// parseUnifiedDiff converts raw "git diff" output into a FileDiff struct.
func parseUnifiedDiff(raw, filePath string) *gitcore.FileDiff {
	result := &gitcore.FileDiff{
		Path:  filePath,
		Hunks: make([]gitcore.DiffHunk, 0),
	}

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
			if currentHunk != nil {
				result.Hunks = append(result.Hunks, *currentHunk)
			}
			currentHunk = &gitcore.DiffHunk{
				Lines: make([]gitcore.DiffLine, 0),
			}
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

	if currentHunk != nil {
		result.Hunks = append(result.Hunks, *currentHunk)
	}

	return result
}
