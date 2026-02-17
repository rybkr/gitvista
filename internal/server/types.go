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
	Head   *HeadInfo                `json:"head,omitempty"`
}

// HeadInfo contains information about the current HEAD state.
type HeadInfo struct {
	Hash         string `json:"hash"`
	Ref          string `json:"ref"`
	BranchName   string `json:"branchName"`
	IsDetached   bool   `json:"isDetached"`
	CommitCount  int    `json:"commitCount"`
	BranchCount  int    `json:"branchCount"`
	TagCount     int    `json:"tagCount"`
	Description  string `json:"description"`
	Remotes      map[string]string `json:"remotes"`
	RecentTags   []string `json:"recentTags"`
}
