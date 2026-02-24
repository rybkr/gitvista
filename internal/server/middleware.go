package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
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

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// requestLogger logs method, path, status, and duration for each HTTP request.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"duration", time.Since(start).Round(time.Microsecond),
			"ip", getClientIP(r),
		)
	})
}

// writeDeadline wraps a handler to set a per-response write deadline using
// ResponseController. This enforces a timeout on individual HTTP responses
// without affecting long-lived WebSocket connections (which are not wrapped).
func writeDeadline(d time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Now().Add(d))
		next(w, r)
	}
}

// corsMiddleware adds permissive CORS headers for SaaS mode, where the
// frontend may be served from a different origin than the API.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
