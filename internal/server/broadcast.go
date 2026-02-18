// Package server provides HTTP and WebSocket server functionality for GitVista.
package server

import (
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

const broadcastChannelSize = 256

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
			log.Printf("%s Failed to set write deadline: %v", logError, err1)
			failedClients = append(failedClients, conn)
		} else if err2 != nil {
			log.Printf("%s Broadcast failed to %s: %v", logError, conn.RemoteAddr(), err2)
			failedClients = append(failedClients, conn)
		}
	}

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

// broadcastUpdate queues a message for broadcast. Non-blocking: drops the message
// if the channel is full.
func (s *Server) broadcastUpdate(message UpdateMessage) {
	select {
	case s.broadcast <- message:
	default:
		log.Println("WARNING: Broadcast channel full, dropping message. Clients may be slow.")
	}
}
