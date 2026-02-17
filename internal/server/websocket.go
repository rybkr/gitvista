package server

import (
	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
	"log"
	"net/http"
	"time"
)

// WebSocket configuration constants.
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 512
)

// upgrader configures WebSocket upgrade process.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		// TODO(rybkr): Implement proper CORS checking for production
		// Consider checking origin against whitelist or validating origin == host
		return true
	},
}

// handleWebsocket upgrades HTTP connection to WebSocket and manages client lifecycle.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s WebSocket upgrade failed: %v", logError, err)
		return
	}

	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Printf("%s Failed to set read deadline: %v", logError, err)
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	log.Printf("%s WebSocket client connection: %s", logSuccess, conn.RemoteAddr())

	// Send initial state before registering for broadcasts.
	// Prevents race where broadcast arrives before client knows baseline state.
	s.sendInitialState(conn)

	s.clientsMu.Lock()
	s.clients[conn] = true
	clientCount := len(s.clients)
	s.clientsMu.Unlock()

	log.Printf("%s WebSocket client registered. Total clients: %d", logInfo, clientCount)

	done := make(chan struct{})
	go s.clientReadPump(conn, done)
	go s.clientWritePump(conn, done)
}

// sendInitialState sends full repository state as a delta to a new client.
func (s *Server) sendInitialState(conn *websocket.Conn) {
	s.cacheMu.RLock()
	repo := s.cached.repo
	s.cacheMu.RUnlock()

	message := UpdateMessage{
		Delta:  repo.Diff(&gitcore.Repository{}),
		Status: getWorkingTreeStatus(repo.WorkDir()),
		Head:   buildHeadInfo(repo),
	}

	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		log.Printf("%s Failed to set write deadline: %v", logError, err)
		return
	}
	if err := conn.WriteJSON(message); err != nil {
		log.Printf("%s Failed to send initial state to %s: %v", logError, conn.RemoteAddr(), err)
		return
	}

	log.Printf("%s Initial state sent to %s", logInfo, conn.RemoteAddr())
}

// clientReadPump reads from WebSocket to detect disconnect.
// It doesn't process messages, just detects when the connection closes.
func (s *Server) clientReadPump(conn *websocket.Conn, done chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s Recovered panic in clientReadPump: %v", logWarning, r)
		}
		close(done)
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		// ReadMessage blocks until a message arrives or an error occurs.
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("%s WebSocket read error from %s: %v", logError, conn.RemoteAddr(), err)
			}
			return
		}

		// Message received but not yet processed.
	}
}

// clientWritePump sends pings to keep the connection alive.
// It handles disconnection by closing the done channel.
func (s *Server) clientWritePump(conn *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer s.removeClient(conn)

	for {
		select {
		case <-done:
			log.Printf("%s WebSocket client disconnected: %s", logInfo, conn.RemoteAddr())
			return

		case <-ticker.C:
			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Printf("%s Failed to set write deadline: %v", logError, err)
			}
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("%s WebSocket ping failed for %s: %v", logError, conn.RemoteAddr(), err)
				return
			}
		}
	}
}

// removeClient unregisters and closes a WebSocket connection.
func (s *Server) removeClient(conn *websocket.Conn) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	if s.clients[conn] {
		delete(s.clients, conn)
		if err := conn.Close(); err != nil {
			log.Printf("%s Failed to close connection: %v", logError, err)
		}
		log.Printf("%s WebSocket client removed. Total clients: %d", logInfo, len(s.clients))
	}
}
