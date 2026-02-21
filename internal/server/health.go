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
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := HealthStatus{
		Status: "ok",
	}

	// In local mode, include the repo path.
	if s.mode == ModeLocal && s.localSession != nil {
		if repo := s.localSession.Repo(); repo != nil {
			status.Repo = repo.GitDir()
		}
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		s.logger.Error("Failed to encode health status", "err", err)
	}
}
