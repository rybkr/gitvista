package hosted

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/repomanager"
)

func mustHostedRepo(t *testing.T, h *Handler, repoID string) HostedRepo {
	t.Helper()
	repo, err := h.Store.GetRepo(DefaultHostedAccountSlug, repoID)
	if err != nil {
		t.Fatalf("failed to resolve hosted repo %q: %v", repoID, err)
	}
	return repo
}

func addHostedRepoForTest(t *testing.T, h *Handler) RepoResponse {
	t.Helper()
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleAddRepo(w, req, DefaultHostedAccountSlug)
	if w.Code != http.StatusCreated {
		t.Fatalf("add repo status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp RepoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}
	return resp
}

func TestHandleAddRepo_Success(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAddRepo(w, req, DefaultHostedAccountSlug)

	if w.Code != http.StatusCreated {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp RepoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("response ID is empty")
	}
	if resp.State == "" {
		t.Error("response State is empty")
	}
	if resp.RepoAccess == "" {
		t.Error("response RepoAccess is empty")
	}
	if resp.DisplayName != "golang/example" {
		t.Errorf("response DisplayName = %q, want %q", resp.DisplayName, "golang/example")
	}
}

func TestHandleAddRepo_MissingURL(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAddRepo(w, req, DefaultHostedAccountSlug)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddRepo_InvalidURL(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	body := strings.NewReader(`{"url":"file:///tmp/repo"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAddRepo(w, req, DefaultHostedAccountSlug)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleAddRepo_StripsCredentialBearingURL(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	body := strings.NewReader(`{"url":"https://user:secret@github.com/golang/example"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAddRepo(w, req, DefaultHostedAccountSlug)

	if w.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp RepoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.URL != "https://github.com/golang/example" {
		t.Fatalf("URL = %q, want %q", resp.URL, "https://github.com/golang/example")
	}
}

func TestHandleListRepos(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	// Add a repo first
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	h.HandleAddRepo(addW, addReq, DefaultHostedAccountSlug)

	if addW.Code != http.StatusCreated {
		t.Fatalf("setup: add repo failed with status %d", addW.Code)
	}

	// List repos
	req := httptest.NewRequest("GET", "/api/repos", nil)
	w := httptest.NewRecorder()

	h.HandleListRepos(w, req, DefaultHostedAccountSlug)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var repos []RepoResponse
	if err := json.NewDecoder(w.Body).Decode(&repos); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("got %d repos, want 0", len(repos))
	}
}

func TestHandleRepoStatus_NotFound(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	req := httptest.NewRequest("GET", "/api/repos/nonexistent/status", nil)
	w := httptest.NewRecorder()

	h.HandleRepoStatus(w, req, HostedRepo{AccountSlug: DefaultHostedAccountSlug, ID: "nonexistent", ManagedRepoID: "nonexistent"})

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRepoStatus_Found(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	// Add a repo
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	h.HandleAddRepo(addW, addReq, DefaultHostedAccountSlug)

	var addResp RepoResponse
	if err := json.NewDecoder(addW.Body).Decode(&addResp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}

	// Check status
	req := httptest.NewRequest("GET", "/api/repos/"+addResp.ID+"/status", nil)
	w := httptest.NewRecorder()

	h.HandleRepoStatus(w, req, mustHostedRepo(t, h, addResp.ID))

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp RepoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != addResp.ID {
		t.Errorf("ID = %q, want %q", resp.ID, addResp.ID)
	}
	if resp.DisplayName != "golang/example" {
		t.Errorf("DisplayName = %q, want %q", resp.DisplayName, "golang/example")
	}
}

func TestHandleRemoveRepo_NotFound(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	req := httptest.NewRequest("DELETE", "/api/repos/nonexistent", nil)
	w := httptest.NewRecorder()

	h.HandleRemoveRepo(w, req, HostedRepo{AccountSlug: DefaultHostedAccountSlug, ID: "nonexistent", ManagedRepoID: "nonexistent"})

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRemoveRepo_Success(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	// Add a repo
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	h.HandleAddRepo(addW, addReq, DefaultHostedAccountSlug)

	var addResp RepoResponse
	if err := json.NewDecoder(addW.Body).Decode(&addResp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}

	// Remove it
	req := httptest.NewRequest("DELETE", "/api/repos/"+addResp.ID, nil)
	w := httptest.NewRecorder()

	h.HandleRemoveRepo(w, req, mustHostedRepo(t, h, addResp.ID))

	if w.Code != http.StatusNoContent {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify it's gone
	listReq := httptest.NewRequest("GET", "/api/repos", nil)
	listW := httptest.NewRecorder()
	h.HandleListRepos(listW, listReq, DefaultHostedAccountSlug)

	var repos []RepoResponse
	if err := json.NewDecoder(listW.Body).Decode(&repos); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("got %d repos after removal, want 0", len(repos))
	}
}

func TestHandleRepoProgress_AlreadyReady(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	// Add a repo
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	h.HandleAddRepo(addW, addReq, DefaultHostedAccountSlug)

	var addResp RepoResponse
	if err := json.NewDecoder(addW.Body).Decode(&addResp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}

	// Force the repo to ready state for testing
	hostedRepo, err := h.Store.GetRepo(DefaultHostedAccountSlug, addResp.ID)
	if err != nil {
		t.Fatalf("failed to resolve hosted repo: %v", err)
	}
	h.RepoManager.ForceStateForTest(hostedRepo.ManagedRepoID, repomanager.StateReady)

	// Request SSE progress — should get a single "done" event and close
	req := httptest.NewRequest("GET", "/api/repos/"+addResp.ID+"/progress", nil)
	w := httptest.NewRecorder()

	h.HandleRepoProgress(w, req, mustHostedRepo(t, h, addResp.ID))

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Parse the SSE data line
	output := w.Body.String()
	if !strings.Contains(output, `"done":true`) {
		t.Errorf("expected done:true in SSE output, got: %s", output)
	}
	if !strings.Contains(output, `"state":"ready"`) {
		t.Errorf("expected state:ready in SSE output, got: %s", output)
	}
}

func TestHandleRepoProgress_NotFound(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	req := httptest.NewRequest("GET", "/api/repos/nonexistent/progress", nil)
	w := httptest.NewRecorder()

	h.HandleRepoProgress(w, req, HostedRepo{AccountSlug: DefaultHostedAccountSlug, ID: "nonexistent", ManagedRepoID: "nonexistent"})

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRepoProgress_LocalMode(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest("GET", "/api/repos/test/progress", nil)
	w := httptest.NewRecorder()

	h.HandleRepoProgress(w, req, HostedRepo{AccountSlug: DefaultHostedAccountSlug, ID: "test", ManagedRepoID: "test"})

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestRepoHandlers_LocalMode(t *testing.T) {
	// In local mode, all repo management endpoints return 404
	h := &Handler{}

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"add repo", "POST", "/api/repos", `{"url":"https://example.com"}`, func(w http.ResponseWriter, r *http.Request) { h.HandleAddRepo(w, r, DefaultHostedAccountSlug) }},
		{"list repos", "GET", "/api/repos", "", func(w http.ResponseWriter, r *http.Request) { h.HandleListRepos(w, r, DefaultHostedAccountSlug) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
			}
		})
	}

	// Test status and remove with ID parameter
	t.Run("repo status", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/repos/test/status", nil)
		w := httptest.NewRecorder()
		h.HandleRepoStatus(w, req, HostedRepo{AccountSlug: DefaultHostedAccountSlug, ID: "test", ManagedRepoID: "test"})
		if w.Code != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("remove repo", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/repos/test", nil)
		w := httptest.NewRecorder()
		h.HandleRemoveRepo(w, req, HostedRepo{AccountSlug: DefaultHostedAccountSlug, ID: "test", ManagedRepoID: "test"})
		if w.Code != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

func TestHandleRepoRoutes_InvalidID_GenericError(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	// Hit a session-scoped route with a nonexistent repo ID to trigger
	// the getOrCreateSession error path.
	req := httptest.NewRequest("GET", "/api/repos/nonexistent/repository", nil)
	w := httptest.NewRecorder()

	h.HandleRepoRoutes(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "Repository not available" {
		t.Errorf("body = %q, want %q", body, "Repository not available")
	}
}

func TestHandleRepoRoutes_RequiresAccessToken(t *testing.T) {
	_, h := newTestHostedRuntime(t)
	addResp := addHostedRepoForTest(t, h)

	req := httptest.NewRequest("GET", "/api/repos/"+addResp.ID+"/repository", nil)
	w := httptest.NewRecorder()

	h.HandleRepoRoutes(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "Repository not available" {
		t.Fatalf("body = %q, want %q", body, "Repository not available")
	}
}

func TestHandleRepoRoutes_WithAccessToken(t *testing.T) {
	_, h := newTestHostedRuntime(t)
	addResp := addHostedRepoForTest(t, h)

	req := httptest.NewRequest("GET", "/api/repos/"+addResp.ID+"/status", nil)
	req.Header.Set(HostedRepoTokenHeader, addResp.RepoAccess)
	w := httptest.NewRecorder()

	h.HandleRepoRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleAddRepo_InvalidURL_GenericError(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	body := strings.NewReader(`{"url":"file:///etc/passwd"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAddRepo(w, req, DefaultHostedAccountSlug)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
	respBody := strings.TrimSpace(w.Body.String())
	if respBody != "Invalid repository URL" {
		t.Errorf("body = %q, want %q", respBody, "Invalid repository URL")
	}
}

func TestHandleAccountRoutes_AccountScopedRepoLifecycle(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/accounts/personal/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	h.HandleAccountRoutes(addW, addReq)

	if addW.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body: %s", addW.Code, http.StatusCreated, addW.Body.String())
	}

	var addResp RepoResponse
	if err := json.NewDecoder(addW.Body).Decode(&addResp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}
	if addResp.AccountID != DefaultHostedAccountSlug {
		t.Fatalf("account ID = %q, want %q", addResp.AccountID, DefaultHostedAccountSlug)
	}

	listReq := httptest.NewRequest("GET", "/api/accounts/personal/repos", nil)
	listW := httptest.NewRecorder()
	h.HandleAccountRoutes(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listW.Code, http.StatusOK)
	}

	statusReq := httptest.NewRequest("GET", "/api/accounts/personal/repos/"+addResp.ID+"/status", nil)
	statusReq.Header.Set("X-GitVista-Repo-Token", addResp.RepoAccess)
	statusW := httptest.NewRecorder()
	h.HandleAccountRoutes(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body: %s", statusW.Code, http.StatusOK, statusW.Body.String())
	}
}
