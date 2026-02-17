package server

import (
	"encoding/json"
	"net/http"
)

// HealthStatus represents the server health check response.
type HealthStatus struct {
	Status string `json:"status"`
	Repo   string `json:"repo"`
}

// handleHealth returns a health check response for load balancers and monitoring.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := HealthStatus{
		Status: "ok",
		Repo:   s.repo.GitDir(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
