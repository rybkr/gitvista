package server

import (
	"encoding/json"
	"net/http"
)

// handleRepository serves repository metadata via REST API.
// Used for initial page load and debugging.
func (s *Server) handleRepository(w http.ResponseWriter, r *http.Request) {
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	response := map[string]interface{}{
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
