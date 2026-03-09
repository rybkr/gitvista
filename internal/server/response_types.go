package server

import "github.com/rybkr/gitvista/internal/gitcore"

type configResponse struct {
	Mode string `json:"mode"`
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

type mergePreviewResponse struct {
	OursBranch    string                      `json:"oursBranch"`
	TheirsBranch  string                      `json:"theirsBranch"`
	OursHash      string                      `json:"oursHash"`
	TheirsHash    string                      `json:"theirsHash"`
	MergeBaseHash string                      `json:"mergeBaseHash"`
	Entries       []gitcore.MergePreviewEntry `json:"entries"`
	Stats         gitcore.MergePreviewStats   `json:"stats"`
}

type mergeUnifiedDiffResponse struct {
	Mode      string             `json:"mode"`
	Path      string             `json:"path"`
	OldHash   string             `json:"oldHash"`
	NewHash   string             `json:"newHash"`
	IsBinary  bool               `json:"isBinary"`
	Truncated bool               `json:"truncated"`
	Hunks     []gitcore.DiffHunk `json:"hunks"`
}

type mergeThreeWayDiffResponse struct {
	Mode         string                    `json:"mode"`
	Path         string                    `json:"path"`
	ConflictType gitcore.ConflictType      `json:"conflictType"`
	IsBinary     bool                      `json:"isBinary"`
	Truncated    bool                      `json:"truncated"`
	Regions      []gitcore.MergeRegion     `json:"regions"`
	Stats        gitcore.ThreeWayDiffStats `json:"stats"`
}

type graphCommitsResponse struct {
	Commits []*gitcore.Commit `json:"commits"`
}

type docsCTAResponse struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type docsSummaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type docsSectionResponse struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type docsHelpResponse struct {
	Label      string          `json:"label"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	PrimaryCTA docsCTAResponse `json:"primaryCta"`
}

type docsPageResponse struct {
	Eyebrow      string                `json:"eyebrow"`
	Title        string                `json:"title"`
	Lede         string                `json:"lede"`
	PrimaryCTA   docsCTAResponse       `json:"primaryCta"`
	SecondaryCTA docsCTAResponse       `json:"secondaryCta"`
	Summary      []docsSummaryItem     `json:"summary"`
	Sections     []docsSectionResponse `json:"sections"`
	Help         docsHelpResponse      `json:"help"`
}
