package server

import "github.com/rybkr/gitvista/gitcore"

type configResponse struct {
	Host string `json:"host"`
}

type repositoryResponse struct {
	Name          string                    `json:"name"`
	CurrentBranch string                    `json:"currentBranch"`
	HeadDetached  bool                      `json:"headDetached"`
	HeadHash      gitcore.Hash              `json:"headHash"`
	Upstream      *gitcore.UpstreamTracking `json:"upstream,omitempty"`
	CommitCount   int                       `json:"commitCount"`
	BranchCount   int                       `json:"branchCount"`
	TagCount      int                       `json:"tagCount"`
	Tags          []string                  `json:"tags"`
	Description   string                    `json:"description"`
	Remotes       map[string]string         `json:"remotes"`
}

type blobResponse struct {
	Hash      string `json:"hash"`
	Size      int    `json:"size"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

type blameEntriesResponse struct {
	Entries any `json:"entries"`
}

type commitDiffEntryResponse struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath,omitempty"`
	Status  string `json:"status"`
	OldHash string `json:"oldHash"`
	NewHash string `json:"newHash"`
	Binary  bool   `json:"binary"`
}

type commitDiffStatsResponse struct {
	Added        int `json:"added"`
	Modified     int `json:"modified"`
	Deleted      int `json:"deleted"`
	Renamed      int `json:"renamed"`
	FilesChanged int `json:"filesChanged"`
}

type commitDiffResponse struct {
	CommitHash     string                    `json:"commitHash"`
	ParentTreeHash string                    `json:"parentTreeHash"`
	Entries        []commitDiffEntryResponse `json:"entries"`
	Stats          commitDiffStatsResponse   `json:"stats"`
}

type diffFileResponse struct {
	Path      string             `json:"path"`
	Status    string             `json:"status"`
	OldHash   string             `json:"oldHash"`
	NewHash   string             `json:"newHash"`
	IsBinary  bool               `json:"isBinary"`
	Truncated bool               `json:"truncated"`
	Hunks     []gitcore.DiffHunk `json:"hunks"`
}

type graphCommitsResponse struct {
	Commits []*gitcore.Commit `json:"commits"`
}
