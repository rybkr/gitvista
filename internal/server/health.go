package server

import (
	"encoding/json"
	"net/http"
)

// HealthStatus represents the server health check response.
type HealthStatus struct {
	Status string `json:"status"`
	Repo   string `json:"repo,omitempty"`
}

// handleHealth returns a health check response for load balancers and monitoring.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	status := HealthStatus{
		Status: "ok",
	}

	// When serving a single repository, include the human-readable repo name (not the filesystem path)
	// so operational monitoring knows which repo is being served without leaking internal directory
	// structure to unauthenticated callers.
	if s.session != nil {
		if repo := s.session.Repo(); repo != nil {
			status.Repo = repo.Name()
		}
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		s.logger.Error("Failed to encode health status", "err", err)
	}
}
