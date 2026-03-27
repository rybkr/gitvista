package gitcore

const (
	maxDiffEntries = 500
	maxBlobSize    = 512 * 1024

	// DefaultContextLines is the number of unchanged lines to include around each
	// change in unified diff output.
	DefaultContextLines = 3
)

// DiffStatus represents the type of change applied to a file in a diff.
type DiffStatus int

//nolint:revive // See: https://git-scm.com/docs/git-diff
const (
	DiffStatusAdded DiffStatus = iota
	DiffStatusModified
	DiffStatusDeleted
	DiffStatusRenamed
)

var diffStatusNames = map[DiffStatus]string{
	DiffStatusAdded:    "added",
	DiffStatusModified: "modified",
	DiffStatusDeleted:  "deleted",
	DiffStatusRenamed:  "renamed",
}

// String returns the string representation of a DiffStatus.
func (s DiffStatus) String() string {
	if name, ok := diffStatusNames[s]; ok {
		return name
	}
	return "unknown"
}

// LineType represents the type of line in a unified diff.
type LineType int

const (
	LineTypeContext LineType = iota
	LineTypeAddition
	LineTypeDeletion
)

var lineTypeNames = map[LineType]string{
	LineTypeContext:  "context",
	LineTypeAddition: "addition",
	LineTypeDeletion: "deletion",
}

// String returns the string representation of a LineType.
func (t LineType) String() string {
	if name, ok := lineTypeNames[t]; ok {
		return name
	}
	return "unknown"
}

// DiffEntry represents a single file change within a diff.
// See: https://git-scm.com/docs/git-diff
type DiffEntry struct {
	Path     string     `json:"path"`
	OldPath  string     `json:"oldPath,omitempty"`
	Status   DiffStatus `json:"status"`
	OldHash  Hash       `json:"oldHash,omitempty"`
	NewHash  Hash       `json:"newHash,omitempty"`
	IsBinary bool       `json:"isBinary"`
	OldMode  string     `json:"oldMode,omitempty"`
	NewMode  string     `json:"newMode,omitempty"`
}

// DiffStats describes the number of insertions, deletions, and changed files.
type DiffStats struct {
	FilesChanged int `json:"filesChanged"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// CommitDiff represents the full diff associated with a single commit.
type CommitDiff struct {
	CommitHash Hash        `json:"commitHash"`
	Entries    []DiffEntry `json:"entries"`
	Stats      DiffStats   `json:"stats"`
}

// DiffLine represents a single line within a diff hunk.
type DiffLine struct {
	Type    LineType `json:"type"`
	Content string   `json:"content"`
	OldLine int      `json:"oldLine"`
	NewLine int      `json:"newLine"`
}

// DiffHunk represents a contiguous block of changes within a file diff.
type DiffHunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []DiffLine `json:"lines"`
}

// FileDiff represents the complete diff for a single file.
type FileDiff struct {
	Path      string     `json:"path"`
	OldHash   Hash       `json:"oldHash"`
	NewHash   Hash       `json:"newHash"`
	IsBinary  bool       `json:"isBinary"`
	Truncated bool       `json:"truncated"`
	Hunks     []DiffHunk `json:"hunks"`
}
