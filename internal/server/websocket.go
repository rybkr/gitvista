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

// localUpgrader allows all origins; used in local mode where the server is
// only reachable from localhost.
var localUpgrader = websocket.Upgrader{
	CheckOrigin:       func(_ *http.Request) bool { return true },
	EnableCompression: true,
}

// saasUpgrader validates that the Origin header matches the request Host to
// prevent cross-site WebSocket hijacking in SaaS mode.
var saasUpgrader = websocket.Upgrader{
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

	session := sessionFromCtx(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	up := localUpgrader
	if s.mode == ModeSaaS {
		up = saasUpgrader
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

	// Send initial state before registering for broadcasts to prevent a race
	// where a broadcast arrives before the client knows its baseline state.
	session.sendInitialState(conn)

	writeMu := session.registerClient(conn)

	done := make(chan struct{})
	session.clientWg.Add(2)
	go session.clientReadPump(conn, done)
	go session.clientWritePump(conn, done, writeMu)
}

// WS lifecycle methods (sendInitialState, registerClient, removeClient,
// clientReadPump, clientWritePump) have been moved to RepoSession in session.go.
