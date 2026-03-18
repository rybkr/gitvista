package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
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

func TestShouldLogRequestAtDebug(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		want   bool
	}{
		{name: "graph commits", path: "/api/graph/commits", status: http.StatusOK, want: true},
		{name: "websocket", path: "/api/ws", status: http.StatusOK, want: true},
		{name: "asset", path: "/local/app.js", status: http.StatusOK, want: true},
		{name: "api summary", path: "/api/graph/summary", status: http.StatusOK, want: false},
		{name: "error remains info", path: "/local/app.js", status: http.StatusNotFound, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldLogRequestAtDebug(tt.path, tt.status); got != tt.want {
				t.Fatalf("shouldLogRequestAtDebug(%q, %d) = %v, want %v", tt.path, tt.status, got, tt.want)
			}
		})
	}
}

func TestRequestLogger_DowngradesNoisyRequests(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := requestLogger(logger, nopHandler)

	req := httptest.NewRequest("GET", "/api/graph/commits?hashes=abc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	out := buf.String()
	if !strings.Contains(out, "level=DEBUG") {
		t.Fatalf("expected debug log, got: %s", out)
	}
	if !strings.Contains(out, "path=/api/graph/commits") {
		t.Fatalf("expected graph commits path in log, got: %s", out)
	}
}

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

func TestSecurityHeadersMiddleware(t *testing.T) {
	h := securityHeadersMiddleware(nopHandler)

	req := httptest.NewRequest("GET", "/repo/test", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Fatalf("Content-Security-Policy = %q", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "style-src 'self' 'unsafe-inline'") {
		t.Fatalf("Content-Security-Policy missing inline style allowance: %q", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want %q", got, "no-referrer")
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := w.Header().Get("Permissions-Policy"); got == "" {
		t.Fatal("Permissions-Policy header not set")
	}
}
