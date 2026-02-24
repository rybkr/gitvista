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
