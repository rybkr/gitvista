package server

import (
	"github.com/rybkr/gitvista/internal/gitcore"
)

// Log prefixes for visual scanning of logs.
const (
	logError   = "\x1b[31m[!]\x1b[0m"
	logWarning = "\x1b[33m[-]\x1b[0m"
	logSuccess = "\x1b[32m[+]\x1b[0m"
	logInfo    = "[>]"
)

// UpdateMessage is sent to clients via WebSocket.
type UpdateMessage struct {
	Delta  *gitcore.RepositoryDelta `json:"delta"`
	Status *WorkingTreeStatus       `json:"status,omitempty"`
}
