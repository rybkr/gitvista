package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rybkr/gitvista/gitcore"
)

func newStaticTestServer(t *testing.T, webFS fs.FS) *Server {
	t.Helper()
	s := NewLocalServer(gitcore.NewEmptyRepository(), "127.0.0.1:0", webFS)
	s.logger = silentLogger()
	return s
}

func newHostedStaticTestServer(t *testing.T, webFS fs.FS) *Server {
	t.Helper()
	s := NewFrontendServer("127.0.0.1:0", webFS, FrontendConfig{
		IndexPath:   "site/index.html",
		SPAFallback: true,
		ConfigMode:  "hosted",
	})
	s.logger = silentLogger()
	return s
}

func testWebFS() fs.FS {
	return fstest.MapFS{
		"local/index.html": {Data: []byte("<!doctype html><title>GitVista Local</title>")},
		"site/index.html":  {Data: []byte("<!doctype html><title>GitVista Site</title>")},
		"local/app.js":     {Data: []byte("console.log('local');")},
		"site/app.js":      {Data: []byte("console.log('site');")},
		"styles.css":       {Data: []byte("body { color: black; }")},
		"favicon.png":      {Data: []byte("png")},
	}
}

func TestStaticHandler_ServesSPAForFrontendRoutes(t *testing.T) {
	s := newHostedStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	tests := []string{
		"/",
		"/docs",
		"/docs/install",
		"/docs/hosted",
		"/docs/local",
		"/a/personal/r/test-repo",
		"/a/personal/r/test-repo/loading",
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
			if body := w.Body.String(); !strings.Contains(body, "<title>GitVista Site</title>") {
				t.Fatalf("expected index.html body for %s, got %q", route, body)
			}
		})
	}
}

func TestStaticHandler_ServesAssetsAndMissingAssets(t *testing.T) {
	s := newStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	t.Run("asset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/local/app.js", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); !strings.Contains(body, "console.log('local');") {
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

	t.Run("install script unavailable in local app", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

func TestHostedStaticHandler_DoesNotServeInstallScript(t *testing.T) {
	s := newHostedStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
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

func TestLocalStaticHandler_DoesNotFallbackUnknownRoutes(t *testing.T) {
	s := newStaticTestServer(t, testWebFS())
	handler := s.staticHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
