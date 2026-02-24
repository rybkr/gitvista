package server

import (
	"encoding/json"
	"net/http"
	"time"
)

type addRepoRequest struct {
	URL string `json:"url"`
}

type repoResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// handleAddRepo accepts a JSON body with a URL and enqueues a clone via the
// RepoManager. Returns 201 with the repo ID and initial state.
func (s *Server) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	if s.repoManager == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req addRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "Missing 'url' field", http.StatusBadRequest)
		return
	}

	id, err := s.repoManager.AddRepo(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state, errMsg, _ := s.repoManager.Status(id)

	resp := repoResponse{
		ID:    id,
		URL:   req.URL,
		State: state.String(),
		Error: errMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode add-repo response", "err", err)
	}
}

// handleListRepos returns a JSON array of all managed repos with their state.
func (s *Server) handleListRepos(w http.ResponseWriter, _ *http.Request) {
	if s.repoManager == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	infos := s.repoManager.List()

	repos := make([]repoResponse, len(infos))
	for i, info := range infos {
		repos[i] = repoResponse{
			ID:        info.ID,
			URL:       info.URL,
			State:     info.State.String(),
			Error:     info.Error,
			CreatedAt: info.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(repos); err != nil {
		s.logger.Error("Failed to encode list-repos response", "err", err)
	}
}

// handleRepoStatus returns the state and error for a single repo.
func (s *Server) handleRepoStatus(w http.ResponseWriter, _ *http.Request, id string) {
	if s.repoManager == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	state, errMsg, err := s.repoManager.Status(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	resp := repoResponse{
		ID:    id,
		State: state.String(),
		Error: errMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode repo-status response", "err", err)
	}
}

// handleRemoveRepo tears down the session and removes the repo from the
// RepoManager. Returns 204 on success.
func (s *Server) handleRemoveRepo(w http.ResponseWriter, _ *http.Request, id string) {
	if s.repoManager == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Tear down the session first (if one exists)
	s.removeSession(id)

	if err := s.repoManager.Remove(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
