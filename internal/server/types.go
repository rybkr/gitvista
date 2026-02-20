package server

import (
	"github.com/rybkr/gitvista/internal/gitcore"
)

// UpdateMessage is sent to clients via WebSocket.
type UpdateMessage struct {
	Delta  *gitcore.RepositoryDelta `json:"delta"`
	Status *WorkingTreeStatus       `json:"status,omitempty"`
	Head   *HeadInfo                `json:"head,omitempty"`
}

// HeadInfo contains information about the current HEAD state.
type HeadInfo struct {
	Hash        string            `json:"hash"`
	Ref         string            `json:"ref"`
	BranchName  string            `json:"branchName"`
	IsDetached  bool              `json:"isDetached"`
	CommitCount int               `json:"commitCount"`
	BranchCount int               `json:"branchCount"`
	TagCount    int               `json:"tagCount"`
	Description string            `json:"description"`
	Remotes     map[string]string `json:"remotes"`
	RecentTags  []string          `json:"recentTags"`
}
