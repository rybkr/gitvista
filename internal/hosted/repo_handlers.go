package hosted

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

type RepoResponse struct {
	AccountID   string    `json:"accountId,omitempty"`
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	DisplayName string    `json:"displayName,omitempty"`
	RepoAccess  string    `json:"-"`
	State       string    `json:"state"`
	Error       string    `json:"error,omitempty"`
	Phase       string    `json:"phase,omitempty"`
	Percent     int       `json:"percent,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (r RepoResponse) MarshalJSON() ([]byte, error) {
	payload := map[string]any{
		"id":        r.ID,
		"url":       r.URL,
		"state":     r.State,
		"createdAt": r.CreatedAt,
	}
	if r.AccountID != "" {
		payload["accountId"] = r.AccountID
	}
	if r.DisplayName != "" {
		payload["displayName"] = r.DisplayName
	}
	if r.RepoAccess != "" {
		payload["accessToken"] = r.RepoAccess
	}
	if r.Error != "" {
		payload["error"] = r.Error
	}
	if r.Phase != "" {
		payload["phase"] = r.Phase
	}
	if r.Percent != 0 {
		payload["percent"] = r.Percent
	}
	return json.Marshal(payload)
}

func (r *RepoResponse) UnmarshalJSON(data []byte) error {
	var decoded struct {
		AccountID   string    `json:"accountId,omitempty"`
		ID          string    `json:"id"`
		URL         string    `json:"url"`
		DisplayName string    `json:"displayName,omitempty"`
		State       string    `json:"state"`
		Error       string    `json:"error,omitempty"`
		Phase       string    `json:"phase,omitempty"`
		Percent     int       `json:"percent,omitempty"`
		CreatedAt   time.Time `json:"createdAt"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var tokenFields map[string]string
	if err := json.Unmarshal(data, &tokenFields); err != nil {
		return err
	}

	*r = RepoResponse{
		AccountID:   decoded.AccountID,
		ID:          decoded.ID,
		URL:         decoded.URL,
		DisplayName: decoded.DisplayName,
		RepoAccess:  tokenFields["accessToken"],
		State:       decoded.State,
		Error:       decoded.Error,
		Phase:       decoded.Phase,
		Percent:     decoded.Percent,
		CreatedAt:   decoded.CreatedAt,
	}
	return nil
}

type cloneProgressEvent struct {
	Phase   string `json:"phase"`
	Percent int    `json:"percent"`
	Done    bool   `json:"done"`
	State   string `json:"state"`
	Error   string `json:"error"`
}

// handleAddRepo accepts a JSON body with a URL and enqueues a clone via the
// RepoManager. Returns 201 with the repo ID and initial state.
func (h *Handler) HandleAddRepo(w http.ResponseWriter, r *http.Request, accountSlug string) {
	if h.RepoManager == nil || h.Store == nil {
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

	hostedRepo, err := h.Store.AddRepo(accountSlug, req.URL)
	if err != nil {
		h.logger().Warn("Failed to add repo", "account", accountSlug, "err", err)
		http.Error(w, "Invalid repository URL", http.StatusBadRequest)
		return
	}

	state, errMsg, progress, _ := h.RepoManager.Status(hostedRepo.ManagedRepoID)

	resp := RepoResponse{
		AccountID:   hostedRepo.AccountSlug,
		ID:          hostedRepo.ID,
		URL:         hostedRepo.URL,
		DisplayName: hostedRepo.DisplayName,
		RepoAccess:  hostedRepo.AccessToken,
		State:       state.String(),
		Error:       errMsg,
		Phase:       progress.Phase,
		Percent:     progress.Percent,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger().Error("Failed to encode add-repo response", "err", err)
	}
}

// handleListRepos intentionally avoids server-side hosted repo enumeration.
// Hosted repo capabilities are browser-held, so recent repos are restored from
// client storage rather than disclosed globally by the server.
func (h *Handler) HandleListRepos(w http.ResponseWriter, _ *http.Request, accountSlug string) {
	if h.RepoManager == nil || h.Store == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode([]RepoResponse{}); err != nil {
		h.logger().Error("Failed to encode list-repos response", "err", err)
	}
}

// handleRepoStatus returns the state and error for a single repo.
func (h *Handler) HandleRepoStatus(w http.ResponseWriter, _ *http.Request, hostedRepo HostedRepo) {
	if h.RepoManager == nil || h.Store == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	state, errMsg, progress, err := h.RepoManager.Status(hostedRepo.ManagedRepoID)
	if err != nil {
		h.logger().Error("Failed to get repo status", "account", hostedRepo.AccountSlug, "id", hostedRepo.ID, "err", err)
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	resp := RepoResponse{
		AccountID:   hostedRepo.AccountSlug,
		ID:          hostedRepo.ID,
		URL:         hostedRepo.URL,
		DisplayName: hostedRepo.DisplayName,
		State:       state.String(),
		Error:       errMsg,
		Phase:       progress.Phase,
		Percent:     progress.Percent,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger().Error("Failed to encode repo-status response", "err", err)
	}
}

// handleRemoveRepo tears down the session and removes the repo from the
// RepoManager. Returns 204 on success.
func (h *Handler) HandleRemoveRepo(w http.ResponseWriter, _ *http.Request, hostedRepo HostedRepo) {
	if h.RepoManager == nil || h.Store == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	h.removeSession(hostedSessionKey(hostedRepo.AccountSlug, hostedRepo.ID))

	if err := h.Store.RemoveRepo(hostedRepo.AccountSlug, hostedRepo.ID); err != nil {
		h.logger().Error("Failed to remove repo", "account", hostedRepo.AccountSlug, "id", hostedRepo.ID, "err", err)
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRepoProgress streams clone progress as Server-Sent Events.
// If the repo is already in a terminal state, it sends a single event and returns.
func (h *Handler) HandleRepoProgress(w http.ResponseWriter, r *http.Request, hostedRepo HostedRepo) {
	if h.RepoManager == nil || h.Store == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Guard: verify repo exists before setting up SSE.
	if _, _, _, err := h.RepoManager.Status(hostedRepo.ManagedRepoID); err != nil {
		h.logger().Error("Failed to get repo status for progress", "account", hostedRepo.AccountSlug, "id", hostedRepo.ID, "err", err)
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe BEFORE re-checking state so we never miss the final Done
	// event if the clone finishes between the state check and subscribe.
	ch, unsubscribe := h.RepoManager.SubscribeProgress(hostedRepo.ManagedRepoID)
	defer unsubscribe()

	// Re-check state after subscribing — if already terminal, send one event.
	state, errMsg, _, _ := h.RepoManager.Status(hostedRepo.ManagedRepoID)

	// Clear any write deadline set by the writeDeadline middleware —
	// SSE connections are long-lived like WebSockets.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeEvent := func(p repomanager.CloneProgress) {
		data, _ := json.Marshal(cloneProgressEvent{
			Phase:   p.Phase,
			Percent: p.Percent,
			Done:    p.Done,
			State:   p.State,
			Error:   p.Error,
		})
		fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
		flusher.Flush()
	}

	if state == repomanager.StateReady || state == repomanager.StateError {
		writeEvent(repomanager.CloneProgress{
			Done:  true,
			State: state.String(),
			Error: errMsg,
		})
		return
	}

	// Flush headers immediately so the browser establishes the SSE connection.
	flusher.Flush()

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
