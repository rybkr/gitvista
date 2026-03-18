package server

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rybkr/gitvista/gitcore"
)

func TestSessionFromCtx_Present(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := newTestSession(repo)

	ctx := WithSessionContext(context.Background(), session)
	got := SessionFromContext(ctx)

	if got != session {
		t.Error("sessionFromCtx did not return the injected session")
	}
}

func TestSessionFromCtx_Absent(t *testing.T) {
	got := SessionFromContext(context.Background())
	if got != nil {
		t.Error("sessionFromCtx returned non-nil for empty context")
	}
}

func TestWithLocalSession(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := newTestSession(repo)

	var captured *RepoSession
	handler := withLocalSession(session, func(w http.ResponseWriter, r *http.Request) {
		captured = SessionFromContext(r.Context())
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

func TestRepoNameOverrideFromCtx_Present(t *testing.T) {
	ctx := WithRepoNameOverrideContext(context.Background(), "golang/example")

	got, ok := repoNameOverrideFromCtx(ctx)
	if !ok {
		t.Fatal("expected repo name override to be present")
	}
	if got != "golang/example" {
		t.Fatalf("repoNameOverrideFromCtx() = %q, want %q", got, "golang/example")
	}
}

func TestRepoNameOverrideFromCtx_AbsentOrEmpty(t *testing.T) {
	tests := []context.Context{
		context.Background(),
		WithRepoNameOverrideContext(context.Background(), ""),
	}

	for i, ctx := range tests {
		if got, ok := repoNameOverrideFromCtx(ctx); ok || got != "" {
			t.Fatalf("case %d: repoNameOverrideFromCtx() = (%q, %v), want (\"\", false)", i, got, ok)
		}
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

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	d.deadline = deadline
	return nil
}

type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
	flushed  bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(&bytes.Buffer{})), nil
}

func (h *hijackableRecorder) Flush() {
	h.flushed = true
}

func TestWriteDeadline_SetsPerResponseDeadline(t *testing.T) {
	recorder := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	var called bool

	handler := writeDeadline(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	})

	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))

	if !called {
		t.Fatal("wrapped handler was not called")
	}
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	if recorder.deadline.IsZero() {
		t.Fatal("write deadline was not set")
	}
	if remaining := time.Until(recorder.deadline); remaining <= 0 || remaining > apiWriteDeadline+time.Second {
		t.Fatalf("deadline remaining = %s, want within (0, %s]", remaining, apiWriteDeadline+time.Second)
	}
}

func TestStatusRecorder_HijackDelegates(t *testing.T) {
	recorder := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	sr := &statusRecorder{ResponseWriter: recorder}

	_, _, err := sr.Hijack()
	if err != nil {
		t.Fatalf("Hijack() error = %v", err)
	}
	if !recorder.hijacked {
		t.Fatal("expected underlying writer Hijack to be called")
	}
}

func TestStatusRecorder_HijackReturnsErrorWhenUnsupported(t *testing.T) {
	sr := &statusRecorder{ResponseWriter: httptest.NewRecorder()}

	_, _, err := sr.Hijack()
	if err == nil {
		t.Fatal("expected Hijack() to fail when unsupported")
	}
	if !strings.Contains(err.Error(), "does not implement http.Hijacker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStatusRecorder_FlushDelegates(t *testing.T) {
	recorder := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	sr := &statusRecorder{ResponseWriter: recorder}

	sr.Flush()

	if !recorder.flushed {
		t.Fatal("expected underlying writer Flush to be called")
	}
}
