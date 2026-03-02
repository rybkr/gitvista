package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func TestSessionFromCtx_Present(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := newTestSession(repo)

	ctx := withSessionCtx(context.Background(), session)
	got := sessionFromCtx(ctx)

	if got != session {
		t.Error("sessionFromCtx did not return the injected session")
	}
}

func TestSessionFromCtx_Absent(t *testing.T) {
	got := sessionFromCtx(context.Background())
	if got != nil {
		t.Error("sessionFromCtx returned non-nil for empty context")
	}
}

func TestWithLocalSession(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := newTestSession(repo)

	var captured *RepoSession
	handler := withLocalSession(session, func(w http.ResponseWriter, r *http.Request) {
		captured = sessionFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if captured != session {
		t.Error("withLocalSession did not inject the session into the request context")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
}

// nopHandler is a simple handler that returns 200 OK.
var nopHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestCorsMiddleware_AllowedOrigin(t *testing.T) {
	allowed := map[string]bool{"https://app.example.com": true}
	h := corsMiddleware(allowed, nopHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods to be set")
	}
}

func TestCorsMiddleware_DisallowedOrigin(t *testing.T) {
	allowed := map[string]bool{"https://app.example.com": true}
	h := corsMiddleware(allowed, nopHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q (must be set even for disallowed origins)", got, "Origin")
	}
}

func TestCorsMiddleware_NoOrigin(t *testing.T) {
	allowed := map[string]bool{"https://app.example.com": true}
	h := corsMiddleware(allowed, nopHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	// No Origin header set
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := w.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q, want empty (no Origin header in request)", got)
	}
}

func TestCorsMiddleware_Preflight(t *testing.T) {
	allowed := map[string]bool{"https://app.example.com": true}
	h := corsMiddleware(allowed, nopHandler)

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods on preflight")
	}
}
