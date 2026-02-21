// Package server provides HTTP and WebSocket server functionality for GitVista.
package server

const broadcastChannelSize = 256

// All broadcast methods (handleBroadcast, sendToAllClients, broadcastUpdate)
// have been moved to RepoSession in session.go.
