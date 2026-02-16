package server

import (
	"encoding/json"
	"fmt"
	"github.com/rybkr/gitvista/internal/gitcore"
	"net/http"
	"strings"
)

// handleRepository serves repository metadata via REST API.
// Used for initial page load and debugging.
func (s *Server) handleRepository(w http.ResponseWriter, r *http.Request) {
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/tree/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing tree hash in path", http.StatusBadRequest)
		return
	}
	path = strings.TrimPrefix(path, "/")

	treeHash, err := gitcore.NewHash(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid hash format: %v", err), http.StatusBadRequest)
		return
	}

	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/blob/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing blob hash in path", http.StatusBadRequest)
		return
	}
	path = strings.TrimPrefix(path, "/")

	blobHash, err := gitcore.NewHash(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid hash format: %v", err), http.StatusBadRequest)
		return
	}

	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse commit hash from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/tree/blame/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing commit hash in path", http.StatusBadRequest)
		return
	}
	path = strings.TrimPrefix(path, "/")

	commitHash, err := gitcore.NewHash(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid commit hash format: %v", err), http.StatusBadRequest)
		return
	}

	// Parse directory path from query parameter (default to empty string for root)
	dirPath := r.URL.Query().Get("path")

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
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

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
