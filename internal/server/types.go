package server

import (
	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

const (
	messageTypeRepoSummary         = "repoSummary"
	messageTypeGraphBootstrapChunk = "graphBootstrapChunk"
	messageTypeBootstrapComplete   = "bootstrapComplete"
	messageTypeGraphDelta          = "graphDelta"
	messageTypeStatus              = "status"
	messageTypeHead                = "head"
)

// UpdateMessage is sent to clients via WebSocket.
type UpdateMessage struct {
	Type              string                          `json:"type"`
	Repo              *repositoryResponse             `json:"repo,omitempty"`
	Bootstrap         *GraphBootstrapChunk            `json:"bootstrap,omitempty"`
	BootstrapComplete *GraphBootstrapCompletePayload  `json:"bootstrapComplete,omitempty"`
	Delta             *repositoryview.RepositoryDelta `json:"delta,omitempty"`
	Status            *WorkingTreeStatus              `json:"status,omitempty"`
	Head              *HeadInfo                       `json:"head,omitempty"`
}

// HeadInfo contains information about the current HEAD state.
type HeadInfo struct {
	Hash        string                    `json:"hash"`
	Ref         string                    `json:"ref"`
	BranchName  string                    `json:"branchName"`
	IsDetached  bool                      `json:"isDetached"`
	Upstream    *gitcore.UpstreamTracking `json:"upstream,omitempty"`
	CommitCount int                       `json:"commitCount"`
	BranchCount int                       `json:"branchCount"`
	TagCount    int                       `json:"tagCount"`
	Description string                    `json:"description"`
	Remotes     map[string]string         `json:"remotes"`
	RecentTags  []string                  `json:"recentTags"`
}

type GraphBootstrapChunk struct {
	ChunkIndex int                     `json:"chunkIndex"`
	Final      bool                    `json:"final"`
	Commits    []*gitcore.Commit       `json:"commits"`
	Branches   map[string]gitcore.Hash `json:"branches,omitempty"`
	HeadHash   string                  `json:"headHash"`
}

type GraphBootstrapCompletePayload struct {
	HeadHash string                `json:"headHash"`
	Tags     map[string]string     `json:"tags,omitempty"`
	Stashes  []*gitcore.StashEntry `json:"stashes,omitempty"`
}
