package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rybkr/gitvista/internal/gitcore"
)

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
			want:    false, // No null bytes in first 8KB
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
	// Create a test repository
	repo := &gitcore.Repository{}
	server := &Server{
		cached: struct {
			repo *gitcore.Repository
		}{
			repo: repo,
		},
	}

	req := httptest.NewRequest("GET", "/api/repository", nil)
	w := httptest.NewRecorder()

	server.handleRepository(w, req)

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
	if _, ok := response["gitDir"]; !ok {
		t.Error("response missing 'gitDir' field")
	}
}

func TestHandleTree_InvalidMethod(t *testing.T) {
	server := &Server{}

	req := httptest.NewRequest("POST", "/api/tree/abc123", nil)
	w := httptest.NewRecorder()

	server.handleTree(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleTree_MissingHash(t *testing.T) {
	server := &Server{}

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

			server.handleTree(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleTree_InvalidHash(t *testing.T) {
	server := &Server{
		cached: struct {
			repo *gitcore.Repository
		}{
			repo: &gitcore.Repository{},
		},
	}

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
			req := httptest.NewRequest("GET", "/api/tree/"+tt.hash, nil)
			w := httptest.NewRecorder()

			server.handleTree(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d for hash %q", w.Code, http.StatusBadRequest, tt.hash)
			}
		})
	}
}

func TestHandleBlob_InvalidMethod(t *testing.T) {
	server := &Server{}

	req := httptest.NewRequest("POST", "/api/blob/abc123", nil)
	w := httptest.NewRecorder()

	server.handleBlob(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleBlob_MissingHash(t *testing.T) {
	server := &Server{}

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

			server.handleBlob(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleBlob_InvalidHash(t *testing.T) {
	server := &Server{
		cached: struct {
			repo *gitcore.Repository
		}{
			repo: &gitcore.Repository{},
		},
	}

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
			req := httptest.NewRequest("GET", "/api/blob/"+tt.hash, nil)
			w := httptest.NewRecorder()

			server.handleBlob(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status code = %d, want %d for hash %q", w.Code, http.StatusBadRequest, tt.hash)
			}
		})
	}
}

func TestHandleTreeBlame_MissingCommitHash(t *testing.T) {
	server := &Server{}

	req := httptest.NewRequest("GET", "/api/tree/blame/?path=src", nil)
	w := httptest.NewRecorder()

	server.handleTreeBlame(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTreeBlame_InvalidCommitHash(t *testing.T) {
	server := &Server{
		cached: struct {
			repo *gitcore.Repository
		}{
			repo: &gitcore.Repository{},
		},
	}

	req := httptest.NewRequest("GET", "/api/tree/blame/invalidhash?path=src", nil)
	w := httptest.NewRecorder()

	server.handleTreeBlame(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
