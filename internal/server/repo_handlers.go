package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rybkr/gitvista/internal/repomanager"
)

type addRepoRequest struct {
	URL string `json:"url"`
}

type repoResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Percent   int       `json:"percent,omitempty"`
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

	state, errMsg, progress, _ := s.repoManager.Status(id)

	resp := repoResponse{
		ID:      id,
		URL:     req.URL,
		State:   state.String(),
		Error:   errMsg,
		Phase:   progress.Phase,
		Percent: progress.Percent,
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

	state, errMsg, progress, err := s.repoManager.Status(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	resp := repoResponse{
		ID:      id,
		State:   state.String(),
		Error:   errMsg,
		Phase:   progress.Phase,
		Percent: progress.Percent,
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

// handleRepoProgress streams clone progress as Server-Sent Events.
// If the repo is already in a terminal state, it sends a single event and returns.
func (s *Server) handleRepoProgress(w http.ResponseWriter, r *http.Request, id string) {
	if s.repoManager == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	state, errMsg, progress, err := s.repoManager.Status(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Clear any write deadline set by the writeDeadline middleware —
	// SSE connections are long-lived like WebSockets.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeEvent := func(p repomanager.CloneProgress) {
		data, _ := json.Marshal(map[string]interface{}{
			"phase":   p.Phase,
			"percent": p.Percent,
			"done":    p.Done,
			"state":   p.State,
			"error":   p.Error,
		})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// If already in a terminal state, send one event and close.
	if state == repomanager.StateReady || state == repomanager.StateError {
		writeEvent(repomanager.CloneProgress{
			Done:  true,
			State: state.String(),
			Error: errMsg,
		})
		return
	}

	// Send current progress snapshot immediately.
	writeEvent(progress)

	ch, unsubscribe := s.repoManager.SubscribeProgress(id)
	defer unsubscribe()

	for {
		select {
		case p, ok := <-ch:
			if !ok {
				return
			}
			writeEvent(p)
			if p.Done {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
