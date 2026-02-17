// Package server provides HTTP and WebSocket server functionality for GitVista.
package server

import (
	"github.com/gorilla/websocket"
	"log"
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
// It removes clients that fail to receive message.
func (s *Server) sendToAllClients(message UpdateMessage) {
	var failedClients []*websocket.Conn

	// Send to all clients (read lock only)
	s.clientsMu.RLock()
	for client := range s.clients {
		if err := client.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			log.Printf("failed to set write deadline: %v", err)
			continue
		}
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Broadcast failed to %s: %v", client.RemoteAddr(), err)
			failedClients = append(failedClients, client)
		}
	}
	s.clientsMu.RUnlock()

	// Remove failed clients (write lock needed)
	if len(failedClients) > 0 {
		s.clientsMu.Lock()
		for _, client := range failedClients {
			delete(s.clients, client)
			if err := client.Close(); err != nil {
				log.Printf("failed to close client connection: %v", err)
			}
		}
		remainingClients := len(s.clients)
		s.clientsMu.Unlock()

		log.Printf("Removed %d failed clients. Total clients: %d",
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
