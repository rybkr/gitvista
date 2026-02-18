// Package server provides HTTP and WebSocket server functionality for GitVista.
package server

import (
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

// Broadcast channel configuration constants.
const (
	broadcastChannelSize = 256
)

// handleBroadcast receives messages from broadcast channel and sends to all clients.
// It runs as goroutine for entire server lifetime.
func (s *Server) handleBroadcast() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			log.Println("Broadcast handler exiting")
			return

		case message := <-s.broadcast:
			s.sendToAllClients(message)
		}
	}
}

// sendToAllClients broadcasts message to all connected WebSocket clients.
// Each client's write mutex is held during the write to prevent concurrent
// writes from clientWritePump's ping goroutine.
func (s *Server) sendToAllClients(message UpdateMessage) {
	var failedClients []*websocket.Conn

	// Snapshot the client map under read lock, then release before doing I/O.
	// This prevents holding the read lock during slow network writes.
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
			log.Printf("%s Failed to set write deadline: %v", logError, err1)
			failedClients = append(failedClients, conn)
		} else if err2 != nil {
			log.Printf("%s Broadcast failed to %s: %v", logError, conn.RemoteAddr(), err2)
			failedClients = append(failedClients, conn)
		}
	}

	// Remove failed clients (write lock needed)
	if len(failedClients) > 0 {
		s.clientsMu.Lock()
		for _, conn := range failedClients {
			delete(s.clients, conn)
			if err := conn.Close(); err != nil {
				log.Printf("%s Failed to close client connection: %v", logError, err)
			}
		}
		remainingClients := len(s.clients)
		s.clientsMu.Unlock()

		log.Printf("%s Removed %d failed clients. Total clients: %d", logInfo,
			len(failedClients), remainingClients)
	}
}

// broadcastUpdate queues message for broadcast to all clients.
// Non-blocking: drops message if channel is full.
func (s *Server) broadcastUpdate(message UpdateMessage) {
	select {
	case s.broadcast <- message:
		// Message queued successfully
	default:
		log.Println("WARNING: Broadcast channel full, dropping message. Clients may be slow.")
	}
}
