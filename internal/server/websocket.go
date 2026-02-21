package server

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 512
)

// CheckOrigin allows all origins; GitVista is designed for local use only.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

// handleWebSocket upgrades the connection and delegates client management to
// the session extracted from the request context.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	session := sessionFromCtx(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", "err", err)
		return
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
	go session.clientReadPump(conn, done)
	go session.clientWritePump(conn, done, writeMu)
}

// WS lifecycle methods (sendInitialState, registerClient, removeClient,
// clientReadPump, clientWritePump) have been moved to RepoSession in session.go.
