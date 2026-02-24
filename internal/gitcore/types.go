package gitcore

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	signatureRe = regexp.MustCompile("[<>]")
)

// Hash represents a 40-character hex-encoded SHA-1 Git object identifier.
type Hash string

// NewHash creates a Hash from a 40-character hex string, returning an error if invalid.
func NewHash(s string) (Hash, error) {
	if len(s) != 40 {
		return "", fmt.Errorf("invalid hash length: %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("invalid hash: %w", err)
	}
	return Hash(s), nil
}

// NewHashFromBytes creates a Hash from a 20-byte array.
func NewHashFromBytes(b [20]byte) (Hash, error) {
	return NewHash(hex.EncodeToString(b[:]))
}

// Short returns the first 7 characters of the hash, or the full hash if shorter.
func (h Hash) Short() string {
	if len(h) < 7 {
		return string(h)
	}
	return string(h)[:7]
}

// Object represents a generic Git object.
type Object interface {
	Type() ObjectType
}

// ObjectType uses the same numeric values as the Git pack format.
// See: https://git-scm.com/docs/pack-format#_object_types
type ObjectType int

const (
	// NoneObject represents no git object.
	NoneObject ObjectType = 0
	// CommitObject represents a git commit object.
	CommitObject ObjectType = 1
	// TreeObject represents a git tree object.
	TreeObject ObjectType = 2
	// BlobObject represents a git blob object.
	BlobObject ObjectType = 3
	// TagObject represents a git tag object.
	TagObject ObjectType = 4
)

// String returns the Git object type name (e.g., "commit", "tree", "blob", "tag").
func (t ObjectType) String() string {
	switch t {
	case CommitObject:
		return objectTypeCommit
	case TreeObject:
		return objectTypeTree
	case BlobObject:
		return objectTypeBlob
	case TagObject:
		return objectTypeTag
	default:
		return StatusUnknown
	}
}

// StrToObjectType converts a string representation of an object type to an ObjectType.
func StrToObjectType(s string) ObjectType {
	switch s {
	case objectTypeCommit:
		return CommitObject
	case objectTypeTag:
		return TagObject
	case objectTypeTree:
		return TreeObject
	default:
		return NoneObject
	}
}

// Commit represents a Git commit object.
type Commit struct {
	ID        Hash      `json:"hash"`
	Tree      Hash      `json:"tree"`
	Parents   []Hash    `json:"parents"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
	Message   string    `json:"message"`
}

// Type returns the ObjectType for a Commit.
func (c *Commit) Type() ObjectType {
	return CommitObject
}

// Tag represents a Git tag object.
type Tag struct {
	ID      Hash       `json:"hash"`
	Object  Hash       `json:"object"`
	ObjType ObjectType `json:"objectType"`
	Name    string     `json:"name"`
	Tagger  Signature  `json:"tagger"`
	Message string     `json:"message"`
}

// Type returns the ObjectType for a Tag.
func (t *Tag) Type() ObjectType {
	return TagObject
}

// TreeEntry represents a single entry within a Git tree object.
type TreeEntry struct {
	ID   Hash   `json:"hash"`
	Name string `json:"name"`
	Mode string `json:"mode"`
	Type string `json:"type"`
}

// Tree represents a Git tree object containing a list of entries.
type Tree struct {
	ID      Hash        `json:"hash"`
	Entries []TreeEntry `json:"entries"`
}

// Type returns the ObjectType for a Tree.
func (t *Tree) Type() ObjectType {
	return TreeObject
}

// Signature represents the author or committer of a Git commit.
type Signature struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	When  time.Time `json:"when"`
}

// NewSignature parses a Git signature line: "Name <email> unix-timestamp timezone".
func NewSignature(signLine string) (Signature, error) {
	parts := signatureRe.Split(signLine, -1)
	if len(parts) != 3 {
		return Signature{}, fmt.Errorf("invalid signature line: %q", signLine)
	}

	name := strings.TrimSpace(parts[0])
	email := strings.TrimSpace(parts[1])

	timePart := strings.TrimSpace(parts[2])
	timeFields := strings.Fields(timePart)
	if timePart == "" || len(timeFields) == 0 {
		return Signature{}, fmt.Errorf("invalid signature line: missing timestamp: %q", signLine)
	}

	var unixTime int64
	if _, err := fmt.Sscanf(timeFields[0], "%d", &unixTime); err != nil {
		return Signature{}, fmt.Errorf("invalid signature line: invalid timestamp: %q", signLine)
	}

	var loc *time.Location
	if len(timeFields) >= 2 {
		loc = parseTimezone(timeFields[1])
	}
	if loc == nil {
		loc = time.UTC
	}

	return Signature{
		Name:  name,
		Email: email,
		When:  time.Unix(unixTime, 0).In(loc),
	}, nil
}

// parseTimezone parses a Git timezone offset string (e.g., "+0530", "-0800")
// into a *time.Location. Returns nil if the string is not a valid offset.
func parseTimezone(tz string) *time.Location {
	if len(tz) != 5 {
		return nil
	}
	sign := 1
	if tz[0] == '-' {
		sign = -1
	} else if tz[0] != '+' {
		return nil
	}
	hours, err := strconv.Atoi(tz[1:3])
	if err != nil {
		return nil
	}
	mins, err := strconv.Atoi(tz[3:5])
	if err != nil {
		return nil
	}
	offset := sign * (hours*3600 + mins*60)
	return time.FixedZone(tz, offset)
}

// ObjectResolver retrieves raw object data and type byte by hash.
// Used for resolving delta base objects during pack file reading.
type ObjectResolver func(id Hash) (data []byte, objectType byte, err error)

// PackIndex maps object hashes to their byte offsets within a pack file.
type PackIndex struct {
	path       string
	packPath   string
	version    uint32
	numObjects uint32
	fanout     [256]uint32
	offsets    map[Hash]int64
}

// FindObject looks up the byte offset of an object by its hash.
func (p *PackIndex) FindObject(id Hash) (int64, bool) {
	offset, found := p.offsets[id]
	return offset, found
}

// PackFile returns the path to the pack file associated with this index.
func (p *PackIndex) PackFile() string { return p.packPath }

// Version returns the pack index format version.
func (p *PackIndex) Version() uint32 { return p.version }

// NumObjects returns the number of objects stored in the pack file.
func (p *PackIndex) NumObjects() uint32 { return p.numObjects }

// Fanout returns the 256-entry fanout table used for binary search within the index.
func (p *PackIndex) Fanout() [256]uint32 { return p.fanout }

// Offsets returns a defensive copy of the offset map.
func (p *PackIndex) Offsets() map[Hash]int64 {
	cp := make(map[Hash]int64, len(p.offsets))
	for k, v := range p.offsets {
		cp[k] = v
	}
	return cp
}

// StashEntry represents a single Git stash entry with its hash and message.
type StashEntry struct {
	Hash    Hash   `json:"hash"`
	Message string `json:"message"`
}

// RepositoryDelta is the wire format sent to the frontend during live updates.
type RepositoryDelta struct {
	AddedCommits   []*Commit `json:"addedCommits"`
	DeletedCommits []*Commit `json:"deletedCommits"`

	AddedBranches   map[string]Hash `json:"addedBranches"`
	AmendedBranches map[string]Hash `json:"amendedBranches"`
	DeletedBranches map[string]Hash `json:"deletedBranches"`

	// HeadHash, Tags, and Stashes are sent on every delta so the frontend stays in sync.
	HeadHash string            `json:"headHash"`
	Tags     map[string]string `json:"tags"` // tag name -> target commit hash (annotated tags are peeled)
	Stashes  []*StashEntry     `json:"stashes"`
}

// NewRepositoryDelta creates a RepositoryDelta with all maps and slices initialized.
func NewRepositoryDelta() *RepositoryDelta {
	return &RepositoryDelta{
		AddedBranches:   make(map[string]Hash),
		AmendedBranches: make(map[string]Hash),
		DeletedBranches: make(map[string]Hash),
		Tags:            make(map[string]string),
		Stashes:         make([]*StashEntry, 0),
	}
}

// IsEmpty reports whether the delta contains no changes.
func (d *RepositoryDelta) IsEmpty() bool {
	return len(d.AddedCommits) == 0 &&
		len(d.DeletedCommits) == 0 &&
		len(d.AddedBranches) == 0 &&
		len(d.DeletedBranches) == 0 &&
		len(d.AmendedBranches) == 0
}

// DiffStatus represents the type of change applied to a file in a diff.
type DiffStatus int

// String constants for file change statuses, shared by DiffStatus.String()
// and FileStatus field values in status.go.
const (
	StatusAdded    = "added"
	StatusModified = "modified"
	StatusDeleted  = "deleted"
	StatusRenamed  = "renamed"
	StatusCopied   = "copied"
	StatusUnknown  = "unknown"
)

const (
	// DiffStatusAdded represents a diff addition.
	DiffStatusAdded DiffStatus = iota
	// DiffStatusModified represents a diff modification.
	DiffStatusModified
	// DiffStatusDeleted represents a diff deletion.
	DiffStatusDeleted
	// DiffStatusRenamed represents a diff renaming.
	DiffStatusRenamed
)

// String returns the string representation of a DiffStatus.
func (s DiffStatus) String() string {
	switch s {
	case DiffStatusAdded:
		return StatusAdded
	case DiffStatusModified:
		return StatusModified
	case DiffStatusDeleted:
		return StatusDeleted
	case DiffStatusRenamed:
		return StatusRenamed
	default:
		return StatusUnknown
	}
}

// DiffEntry represents a single file change within a diff, including its path, status, and hashes.
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

// CommitDiff represents the full diff associated with a single commit.
type CommitDiff struct {
	CommitHash Hash        `json:"commitHash"`
	Entries    []DiffEntry `json:"entries"`
	Stats      DiffStats   `json:"stats"`
}

// DiffStats describes the number of insertions, deletions, and changed files
// resulting from a diff operation.
type DiffStats struct {
	FilesChanged int `json:"filesChanged"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// DiffLine represents a single line within a diff hunk, including its type and line numbers.
type DiffLine struct {
	Type    string `json:"type"` // "context", "addition", or "deletion"
	Content string `json:"content"`
	OldLine int    `json:"oldLine"` // 0 for additions
	NewLine int    `json:"newLine"` // 0 for deletions
}

// DiffHunk represents a contiguous block of changes within a file diff.
type DiffHunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []DiffLine `json:"lines"`
}

// FileDiff represents the complete diff for a single file, including all hunks.
type FileDiff struct {
	Path      string     `json:"path"`
	OldHash   Hash       `json:"oldHash"`
	NewHash   Hash       `json:"newHash"`
	IsBinary  bool       `json:"isBinary"`
	Truncated bool       `json:"truncated"`
	Hunks     []DiffHunk `json:"hunks"`
}

// LineType represents the type of line in a diff.
type LineType string

const (
	// LineTypeContext represents a context line in a diff.
	LineTypeContext = "context"
	// LineTypeAddition represents an added line in a diff.
	LineTypeAddition = "addition"
	// LineTypeDeletion represents a deleted line in a diff.
	LineTypeDeletion = "deletion"
)
