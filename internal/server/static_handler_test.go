package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func newStaticTestServer(t *testing.T, webFS fs.FS) *Server {
	t.Helper()
	s := NewServer(gitcore.NewEmptyRepository(), "127.0.0.1:0", webFS)
	s.logger = silentLogger()
	return s
}

func testWebFS() fs.FS {
	return fstest.MapFS{
		"index.html":  {Data: []byte("<!doctype html><title>GitVista</title>")},
		"app.js":      {Data: []byte("console.log('app');")},
		"styles.css":  {Data: []byte("body { color: black; }")},
		"favicon.png": {Data: []byte("png")},
	}
}

func TestStaticHandler_ServesSPAForFrontendRoutes(t *testing.T) {
	s := newStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	tests := []string{
		"/",
		"/docs",
		"/docs/hosted",
		"/docs/local",
		"/repo/test-repo",
		"/repo/test-repo/1234567890abcdef1234567890abcdef12345678",
	}

	for _, route := range tests {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if body := w.Body.String(); !strings.Contains(body, "<title>GitVista</title>") {
				t.Fatalf("expected index.html body for %s, got %q", route, body)
			}
		})
	}
}

func TestStaticHandler_ServesAssetsAndMissingAssets(t *testing.T) {
	s := newStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	t.Run("asset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); !strings.Contains(body, "console.log('app');") {
			t.Fatalf("expected asset body, got %q", body)
		}
	})

	t.Run("missing asset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

func TestStaticHandler_DoesNotOverrideAPIHandlers(t *testing.T) {
	s := newStaticTestServer(t, testWebFS())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.Handle("/", s.staticHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, `"mode":"local"`) {
		t.Fatalf("expected config payload, got %q", body)
	}
}

func TestStaticHandler_UnknownAPIPathReturnsNotFound(t *testing.T) {
	s := newStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
