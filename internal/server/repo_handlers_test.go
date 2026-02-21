package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/repomanager"
)

func newTestSaaSServer(t *testing.T) *Server {
	t.Helper()
	dataDir := t.TempDir()
	rm, err := repomanager.New(repomanager.Config{
		DataDir: dataDir,
		Logger:  silentLogger(),
	})
	if err != nil {
		t.Fatalf("failed to create repo manager: %v", err)
	}
	if err := rm.Start(); err != nil {
		t.Fatalf("failed to start repo manager: %v", err)
	}
	t.Cleanup(rm.Close)

	webFS := os.DirFS(t.TempDir())
	s := NewSaaSServer(rm, "127.0.0.1:0", webFS)
	s.logger = silentLogger()
	return s
}

func TestHandleAddRepo_Success(t *testing.T) {
	s := newTestSaaSServer(t)

	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleAddRepo(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp repoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("response ID is empty")
	}
	if resp.State == "" {
		t.Error("response State is empty")
	}
}

func TestHandleAddRepo_MissingURL(t *testing.T) {
	s := newTestSaaSServer(t)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleAddRepo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddRepo_InvalidURL(t *testing.T) {
	s := newTestSaaSServer(t)

	body := strings.NewReader(`{"url":"file:///tmp/repo"}`)
	req := httptest.NewRequest("POST", "/api/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleAddRepo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleListRepos(t *testing.T) {
	s := newTestSaaSServer(t)

	// Add a repo first
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	s.handleAddRepo(addW, addReq)

	if addW.Code != http.StatusCreated {
		t.Fatalf("setup: add repo failed with status %d", addW.Code)
	}

	// List repos
	req := httptest.NewRequest("GET", "/api/repos", nil)
	w := httptest.NewRecorder()

	s.handleListRepos(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var repos []repoResponse
	if err := json.NewDecoder(w.Body).Decode(&repos); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("got %d repos, want 1", len(repos))
	}
}

func TestHandleRepoStatus_NotFound(t *testing.T) {
	s := newTestSaaSServer(t)

	req := httptest.NewRequest("GET", "/api/repos/nonexistent/status", nil)
	w := httptest.NewRecorder()

	s.handleRepoStatus(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRepoStatus_Found(t *testing.T) {
	s := newTestSaaSServer(t)

	// Add a repo
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	s.handleAddRepo(addW, addReq)

	var addResp repoResponse
	if err := json.NewDecoder(addW.Body).Decode(&addResp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}

	// Check status
	req := httptest.NewRequest("GET", "/api/repos/"+addResp.ID+"/status", nil)
	w := httptest.NewRecorder()

	s.handleRepoStatus(w, req, addResp.ID)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp repoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != addResp.ID {
		t.Errorf("ID = %q, want %q", resp.ID, addResp.ID)
	}
}

func TestHandleRemoveRepo_NotFound(t *testing.T) {
	s := newTestSaaSServer(t)

	req := httptest.NewRequest("DELETE", "/api/repos/nonexistent", nil)
	w := httptest.NewRecorder()

	s.handleRemoveRepo(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRemoveRepo_Success(t *testing.T) {
	s := newTestSaaSServer(t)

	// Add a repo
	body := strings.NewReader(`{"url":"https://github.com/golang/example"}`)
	addReq := httptest.NewRequest("POST", "/api/repos", body)
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	s.handleAddRepo(addW, addReq)

	var addResp repoResponse
	if err := json.NewDecoder(addW.Body).Decode(&addResp); err != nil {
		t.Fatalf("failed to decode add response: %v", err)
	}

	// Remove it
	req := httptest.NewRequest("DELETE", "/api/repos/"+addResp.ID, nil)
	w := httptest.NewRecorder()

	s.handleRemoveRepo(w, req, addResp.ID)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify it's gone
	listReq := httptest.NewRequest("GET", "/api/repos", nil)
	listW := httptest.NewRecorder()
	s.handleListRepos(listW, listReq)

	var repos []repoResponse
	if err := json.NewDecoder(listW.Body).Decode(&repos); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("got %d repos after removal, want 0", len(repos))
	}
}

func TestRepoHandlers_LocalMode(t *testing.T) {
	// In local mode, all repo management endpoints return 404
	s := newTestServer(t)

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"add repo", "POST", "/api/repos", `{"url":"https://example.com"}`, s.handleAddRepo},
		{"list repos", "GET", "/api/repos", "", s.handleListRepos},
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
		s.handleRepoStatus(w, req, "test")
		if w.Code != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("remove repo", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/repos/test", nil)
		w := httptest.NewRecorder()
		s.handleRemoveRepo(w, req, "test")
		if w.Code != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}
