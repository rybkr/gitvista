package server

import (
	"compress/flate"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 512
)

// localUpgrader validates local-mode origins. It allows same-host requests and
// exact same-host requests to prevent cross-site WebSocket hijacking from
// other local applications running on loopback.
var localUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
	EnableCompression: true,
}

// hostedUpgrader validates that the Origin header matches the request Host to
// prevent cross-site WebSocket hijacking in hosted mode.
var hostedUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
	EnableCompression: true,
}

// handleWebSocket upgrades the connection and delegates client management to
// the session extracted from the request context. WebSocket upgrades go through
// the rate limiter to prevent resource exhaustion.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Rate-limit WebSocket upgrades to prevent connection exhaustion.
	ip := getClientIP(r)
	if !s.rateLimiter.allow(ip) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	session := SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	up := localUpgrader
	if s.mode == ModeHosted {
		up = hostedUpgrader
	}

	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", "err", err)
		return
	}

	conn.EnableWriteCompression(true)
	if err := conn.SetCompressionLevel(flate.BestSpeed); err != nil {
		s.logger.Error("Failed to set compression level", "err", err)
	}
	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		s.logger.Error("Failed to set read deadline", "addr", conn.RemoteAddr(), "err", err)
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	s.logger.Info("WebSocket client connected", "addr", conn.RemoteAddr())

	repo := session.Repo()
	if repo == nil {
		_ = conn.Close()
		s.logger.Error("WebSocket client bootstrap failed: repository not available", "addr", conn.RemoteAddr())
		return
	}

	// Send bootstrap payloads before registering for live broadcasts so the
	// client receives a coherent initial snapshot from cached repo state.
	if err := session.sendInitialRepoSummary(conn, buildRepositoryResponse(repo, r.Context())); err != nil {
		s.logger.Error("Failed to send initial repo summary", "addr", conn.RemoteAddr(), "err", err)
		_ = conn.Close()
		return
	}
	if err := session.sendInitialBootstrap(conn); err != nil {
		s.logger.Error("Failed to send initial bootstrap", "addr", conn.RemoteAddr(), "err", err)
		_ = conn.Close()
		return
	}

	writeMu := session.registerClient(conn)

	done := make(chan struct{})
	session.clientWg.Add(2)
	go session.clientReadPump(conn, done)
	go session.clientWritePump(conn, done, writeMu)
}

// WS lifecycle methods (sendInitialState, registerClient, removeClient,
// clientReadPump, clientWritePump) have been moved to RepoSession in session.go.
