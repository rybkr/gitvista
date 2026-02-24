package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// newTestSession creates a RepoSession with a zero-value Repository for testing
// handlers that only need a non-nil repo in the context.
func newTestSession(repo *gitcore.Repository) *RepoSession {
	if repo == nil {
		repo = gitcore.NewEmptyRepository()
	}
	return NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
	})
}

// requestWithSession creates an HTTP request with a RepoSession in the context.
//
//nolint:unparam
func requestWithSession(method, target string, session *RepoSession) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	ctx := withSessionCtx(req.Context(), session)
	return req.WithContext(ctx)
}

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{
			name:    "empty content",
			content: []byte{},
			want:    false,
		},
		{
			name:    "plain text",
			content: []byte("Hello, World!\nThis is plain text."),
			want:    false,
		},
		{
			name:    "JSON content",
			content: []byte(`{"key": "value", "number": 123}`),
			want:    false,
		},
		{
			name:    "Go source code",
			content: []byte("package main\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}\n"),
			want:    false,
		},
		{
			name:    "null byte at start",
			content: []byte{0x00, 0x48, 0x65, 0x6c, 0x6c, 0x6f},
			want:    true,
		},
		{
			name:    "null byte in middle",
			content: []byte("Hello\x00World"),
			want:    true,
		},
		{
			name:    "null byte at end",
			content: []byte("Hello World\x00"),
			want:    true,
		},
		{
			name:    "PNG file header with null bytes",
			content: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D},
			want:    true,
		},
		{
			name:    "ELF binary header",
			content: []byte{0x7F, 0x45, 0x4C, 0x46, 0x02, 0x01, 0x01, 0x00},
			want:    true,
		},
		{
			name:    "PDF header",
			content: []byte("%PDF-1.4\n%"),
			want:    false,
		},
		{
			name:    "UTF-8 text with emoji",
			content: []byte("Hello 👋 World 🌍"),
			want:    false,
		},
		{
			name:    "lots of printable chars then null",
			content: append([]byte("A very long text string with lots of content"), 0x00),
			want:    true,
		},
		{
			name:    "large text no null bytes",
			content: []byte("A" + string(make([]byte, 10000)) + "text"),
			want:    true, // make([]byte, N) creates zero-filled bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryContent(tt.content)
			if got != tt.want {
				t.Errorf("isBinaryContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleRepository_Success(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := newTestSession(repo)
	s := newTestServer(t)

	req := requestWithSession("GET", "/api/repository", session)
	w := httptest.NewRecorder()

	s.handleRepository(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response map[string]any
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := response["name"]; !ok {
		t.Error("response missing 'name' field")
	}
	// gitDir must not be present: exposing filesystem paths to unauthenticated
	// clients is a security issue (see #73).
	if _, ok := response["gitDir"]; ok {
		t.Error("response must not contain 'gitDir' field (path leak)")
	}
}

func TestHandleRepository_NoSession(t *testing.T) {
	s := newTestServer(t)

	// Request without session in context
	req := httptest.NewRequest("GET", "/api/repository", nil)
	w := httptest.NewRecorder()

	s.handleRepository(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleTree_InvalidMethod(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/tree/abc123", nil)
	w := httptest.NewRecorder()

	s.handleTree(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleTree_MissingHash(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"empty path", "/api/tree/"},
		{"just prefix", "/api/tree"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			s.handleTree(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleTree_InvalidHash(t *testing.T) {
	session := newTestSession(nil)
	s := newTestServer(t)

	tests := []struct {
		name string
		hash string
	}{
		{"too short", "abc"},
		{"invalid chars", "ghijklmnopqrstuvwxyz1234567890123456789"},
		{"wrong length", "abcdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := requestWithSession("GET", "/api/tree/"+tt.hash, session)
			w := httptest.NewRecorder()

			s.handleTree(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d for hash %q", w.Code, http.StatusBadRequest, tt.hash)
			}
		})
	}
}

func TestHandleBlob_InvalidMethod(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/blob/abc123", nil)
	w := httptest.NewRecorder()

	s.handleBlob(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleBlob_MissingHash(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"empty path", "/api/blob/"},
		{"just prefix", "/api/blob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			s.handleBlob(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleBlob_InvalidHash(t *testing.T) {
	session := newTestSession(nil)
	s := newTestServer(t)

	tests := []struct {
		name string
		hash string
	}{
		{"too short", "abc"},
		{"invalid chars", "ghijklmnopqrstuvwxyz1234567890123456789"},
		{"non-hex chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := requestWithSession("GET", "/api/blob/"+tt.hash, session)
			w := httptest.NewRecorder()

			s.handleBlob(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d for hash %q", w.Code, http.StatusBadRequest, tt.hash)
			}
		})
	}
}

func TestHandleTreeBlame_MissingCommitHash(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/tree/blame/?path=src", nil)
	w := httptest.NewRecorder()

	s.handleTreeBlame(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTreeBlame_InvalidCommitHash(t *testing.T) {
	session := newTestSession(nil)
	s := newTestServer(t)

	req := requestWithSession("GET", "/api/tree/blame/invalidhash?path=src", session)
	w := httptest.NewRecorder()

	s.handleTreeBlame(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCommitDiff_InvalidMethod(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/commit/diff/abc123", nil)
	w := httptest.NewRecorder()

	s.handleCommitDiff(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCommitDiff_MissingHash(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"empty path", "/api/commit/diff/"},
		{"just prefix", "/api/commit/diff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			s.handleCommitDiff(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleCommitDiff_InvalidHash(t *testing.T) {
	session := newTestSession(nil)
	s := newTestServer(t)

	tests := []struct {
		name string
		hash string
	}{
		{"too short", "abc"},
		{"invalid chars", "ghijklmnopqrstuvwxyz1234567890123456789"},
		{"non-hex chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := requestWithSession("GET", "/api/commit/diff/"+tt.hash, session)
			w := httptest.NewRecorder()

			s.handleCommitDiff(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d for hash %q", w.Code, http.StatusBadRequest, tt.hash)
			}
		})
	}
}

func TestHandleCommitDiff_FileDiff_MissingPath(t *testing.T) {
	session := newTestSession(nil)
	s := newTestServer(t)

	validHash := "0123456789abcdef0123456789abcdef01234567"
	req := requestWithSession("GET", "/api/commit/diff/"+validHash+"/file", session)
	w := httptest.NewRecorder()

	s.handleCommitDiff(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCommitDiff_FileDiff_InvalidPath(t *testing.T) {
	session := newTestSession(nil)
	s := newTestServer(t)

	validHash := "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name string
		path string
	}{
		{"directory traversal", "../../etc/passwd"},
		{"absolute path", "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := requestWithSession("GET", "/api/commit/diff/"+validHash+"/file?path="+tt.path, session)
			w := httptest.NewRecorder()

			s.handleCommitDiff(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d for path %q", w.Code, http.StatusBadRequest, tt.path)
			}
		})
	}
}

func TestExtractHashParam_NoSession(t *testing.T) {
	s := newTestServer(t)

	validHash := "0123456789abcdef0123456789abcdef01234567"
	req := httptest.NewRequest("GET", "/api/tree/"+validHash, nil)
	// No session in context
	w := httptest.NewRecorder()

	_, _, _, ok := s.extractHashParam(w, req, "/api/tree/")
	if ok {
		t.Error("extractHashParam returned ok=true without session in context")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestSessionFromCtx(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		session := newTestSession(nil)
		ctx := withSessionCtx(context.Background(), session)
		got := sessionFromCtx(ctx)
		if got != session {
			t.Error("sessionFromCtx did not return the expected session")
		}
	})

	t.Run("absent", func(t *testing.T) {
		got := sessionFromCtx(context.Background())
		if got != nil {
			t.Error("sessionFromCtx returned non-nil for empty context")
		}
	})
}
