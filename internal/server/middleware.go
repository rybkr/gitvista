package server

import (
	"context"
	"net/http"
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

