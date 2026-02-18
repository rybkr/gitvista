package server

import (
	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
	"log"
	"net/http"
	"sync"
	"time"
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

	// Send initial state before registering for broadcasts to prevent a race
	// where a broadcast arrives before the client knows its baseline state.
	s.sendInitialState(conn)

	writeMu := &sync.Mutex{}

	s.clientsMu.Lock()
	s.clients[conn] = writeMu
	clientCount := len(s.clients)
	s.clientsMu.Unlock()

	log.Printf("%s WebSocket client registered. Total clients: %d", logInfo, clientCount)

	done := make(chan struct{})
	go s.clientReadPump(conn, done)
	go s.clientWritePump(conn, done, writeMu)
}

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

// clientReadPump blocks on reads to detect client disconnect, then closes
// the done channel to signal clientWritePump to stop.
func (s *Server) clientReadPump(conn *websocket.Conn, done chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s Recovered panic in clientReadPump: %v", logWarning, r)
		}
		close(done)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("%s WebSocket read error from %s: %v", logError, conn.RemoteAddr(), err)
			}
			return
		}
	}
}

// clientWritePump sends keepalive pings. writeMu serializes writes with broadcasts.
func (s *Server) clientWritePump(conn *websocket.Conn, done chan struct{}, writeMu *sync.Mutex) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer s.removeClient(conn)

	for {
		select {
		case <-done:
			log.Printf("%s WebSocket client disconnected: %s", logInfo, conn.RemoteAddr())
			return

		case <-ticker.C:
			writeMu.Lock()
			err1 := conn.SetWriteDeadline(time.Now().Add(writeWait))
			var err2 error
			if err1 == nil {
				err2 = conn.WriteMessage(websocket.PingMessage, nil)
			}
			writeMu.Unlock()

			if err1 != nil {
				log.Printf("%s Failed to set write deadline: %v", logError, err1)
			}
			if err2 != nil {
				log.Printf("%s WebSocket ping failed for %s: %v", logError, conn.RemoteAddr(), err2)
				return
			}
		}
	}
}

func (s *Server) removeClient(conn *websocket.Conn) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	if _, ok := s.clients[conn]; ok {
		delete(s.clients, conn)
		if err := conn.Close(); err != nil {
			log.Printf("%s Failed to close connection: %v", logError, err)
		}
		log.Printf("%s WebSocket client removed. Total clients: %d", logInfo, len(s.clients))
	}
}
