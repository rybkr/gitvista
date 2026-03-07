package gitcore

// CommitSkeleton is the minimum data needed to position a node in the graph.
// Contains only topology (parents) and temporal position (timestamp).
type CommitSkeleton struct {
	Hash      Hash   `json:"h"`
	Parents   []Hash `json:"p,omitempty"`
	Timestamp int64  `json:"t"` // committer date, unix seconds
}

// GraphSummary is a lightweight representation of the full DAG topology.
// Sent on initial connect instead of the full commit set, enabling the
// client to compute layout positions before loading commit details on demand.
type GraphSummary struct {
	TotalCommits    int               `json:"totalCommits"`
	Skeleton        []CommitSkeleton  `json:"skeleton"`
	Branches        map[string]Hash   `json:"branches"`
	Tags            map[string]string `json:"tags"`
	HeadHash        string            `json:"headHash"`
	Stashes         []*StashEntry     `json:"stashes"`
	OldestTimestamp int64             `json:"oldestTimestamp"`
	NewestTimestamp int64             `json:"newestTimestamp"`
}
