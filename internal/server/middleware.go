package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey int

const sessionKey contextKey = iota

// withSessionCtx returns a new context carrying the given RepoSession.
func withSessionCtx(ctx context.Context, rs *RepoSession) context.Context {
	return context.WithValue(ctx, sessionKey, rs)
}

// sessionFromCtx extracts the RepoSession from the request context.
// Returns nil if no session is present.
func sessionFromCtx(ctx context.Context) *RepoSession {
	rs, _ := ctx.Value(sessionKey).(*RepoSession)
	return rs
}

// withLocalSession wraps a handler to inject the given (local-mode) session
// into every request's context.
func withLocalSession(session *RepoSession, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := withSessionCtx(r.Context(), session)
		next(w, r.WithContext(ctx))
	}
}

// withRepoSession extracts a repo ID from the URL path (e.g. /api/repos/{id}/...)
// and looks up or creates the corresponding session.
func (s *Server) withRepoSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// URL pattern: /api/repos/{id}/...
		// After stripping "/api/repos/" we get "{id}/..."
		path := strings.TrimPrefix(r.URL.Path, "/api/repos/")
		if path == r.URL.Path || path == "" {
			http.Error(w, "Missing repo ID", http.StatusBadRequest)
			return
		}

		id := path
		if idx := strings.Index(path, "/"); idx >= 0 {
			id = path[:idx]
		}

		session, err := s.getOrCreateSession(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		ctx := withSessionCtx(r.Context(), session)
		next(w, r.WithContext(ctx))
	}
}
