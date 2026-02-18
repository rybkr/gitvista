// Package server provides HTTP and WebSocket server functionality for GitVista.
package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const broadcastChannelSize = 256

func (s *Server) handleBroadcast() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("Broadcast handler exiting")
			return

		case message := <-s.broadcast:
			s.sendToAllClients(message)
		}
	}
}

func (s *Server) sendToAllClients(message UpdateMessage) {
	var failedClients []*websocket.Conn

	// Snapshot under read lock; release before I/O to avoid blocking on slow writes.
	s.clientsMu.RLock()
	snapshot := make(map[*websocket.Conn]*sync.Mutex, len(s.clients))
	for conn, mu := range s.clients {
		snapshot[conn] = mu
	}
	s.clientsMu.RUnlock()

	for conn, mu := range snapshot {
		mu.Lock()
		err1 := conn.SetWriteDeadline(time.Now().Add(writeWait))
		var err2 error
		if err1 == nil {
			err2 = conn.WriteJSON(message)
		}
		mu.Unlock()

		if err1 != nil {
			s.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err1)
			failedClients = append(failedClients, conn)
		} else if err2 != nil {
			s.logger.Error("Broadcast failed", "addr", conn.RemoteAddr(), "err", err2)
			failedClients = append(failedClients, conn)
		}
	}

	if len(failedClients) > 0 {
		s.clientsMu.Lock()
		for _, conn := range failedClients {
			delete(s.clients, conn)
			if err := conn.Close(); err != nil {
				s.logger.Error("Failed to close client connection", "err", err)
			}
		}
		remainingClients := len(s.clients)
		s.clientsMu.Unlock()

		s.logger.Info("Removed failed clients",
			"removed", len(failedClients),
			"remaining", remainingClients,
		)
	}
}

// broadcastUpdate queues a message for broadcast. Non-blocking: drops the message
// if the channel is full.
func (s *Server) broadcastUpdate(message UpdateMessage) {
	select {
	case s.broadcast <- message:
	default:
		// Warn at the Warn level rather than Info — a dropped broadcast means
		// clients may miss an update and will need to reconnect for consistency.
		s.logger.Warn("Broadcast channel full, dropping message; clients may be slow")
	}
}
