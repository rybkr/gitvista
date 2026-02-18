package gitcore

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	signatureRe = regexp.MustCompile("[<>]")
)

// Hash represents a Git object hash.
// See: https://git-scm.com/docs/git-hash-object
type Hash string

// NewHash creates a Hash from a hexadecimal string, validating its format.
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

// Short returns the truncated representation of a Hash.
// Returns the full hash if it is shorter than 7 characters.
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

// ObjectType denotes the type of a Git object (e.g., commit, tag).
// See: https://git-scm.com/docs/pack-format#_object_types
type ObjectType int

const (
	// NoneObject represents an unknown or invalid Git object type
	NoneObject ObjectType = 0
	// CommitObject represents a Git commit object
	CommitObject ObjectType = 1
	// TreeObject represents a Git tree object
	TreeObject ObjectType = 2
	// BlobObject represents a Git blob object
	BlobObject ObjectType = 3
	// TagObject represents a Git tag object
	TagObject ObjectType = 4
)

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

// Commit represents a Git commit object with its metadata and relationships.
type Commit struct {
	ID        Hash      `json:"hash"`
	Tree      Hash      `json:"tree"`
	Parents   []Hash    `json:"parents"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
	Message   string    `json:"message"`
}

// Type returns the object type for a Commit.
func (c *Commit) Type() ObjectType {
	return CommitObject
}

// Tag represents an annotated Git tag with metadata and a message.
type Tag struct {
	ID      Hash       `json:"hash"`
	Object  Hash       `json:"object"`
	ObjType ObjectType `json:"objectType"`
	Name    string     `json:"name"`
	Tagger  Signature  `json:"tagger"`
	Message string     `json:"message"`
}

// Type returns the object type for a Tag.
func (t *Tag) Type() ObjectType {
	return TagObject
}

// TreeEntry represents a single entry in a Git tree object.
type TreeEntry struct {
	ID   Hash   `json:"hash"`
	Name string `json:"name"`
	Mode string `json:"mode"`
	Type string `json:"type"`
}

// Tree represents a Git tree object.
type Tree struct {
	ID      Hash        `json:"hash"`
	Entries []TreeEntry `json:"entries"`
}

// Type returns the object type for a Tree.
func (t *Tree) Type() ObjectType {
	return TreeObject
}

// Signature represents a Git author or committer signature with name, email, and timestamp.
type Signature struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	When  time.Time `json:"when"`
}

// NewSignature parses a signature line in the format "Name <email> timestamp" and returns a Signature struct.
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

	return Signature{
		Name:  name,
		Email: email,
		When:  time.Unix(unixTime, 0),
	}, nil
}

// ObjectResolver resolves a Git object by its hash, returning raw data and type byte.
type ObjectResolver func(id Hash) (data []byte, objectType byte, err error)

// PackIndex represents a Git pack index file that maps object hashes to their locations within pack files.
type PackIndex struct {
	path       string
	packPath   string
	version    uint32
	numObjects uint32
	fanout     [256]uint32
	offsets    map[Hash]int64
}

// FindObject looks up the offset of an object in the pack file by its hash.
// Returns the offset and true if found, otherwise returns 0 and false.
func (p *PackIndex) FindObject(id Hash) (int64, bool) {
	offset, found := p.offsets[id]
	return offset, found
}

// PackFile returns the path to the pack file associated with this index.
func (p *PackIndex) PackFile() string {
	return p.packPath
}

// Version returns the pack index version.
func (p *PackIndex) Version() uint32 { return p.version }

// NumObjects returns the total number of objects in the pack index.
func (p *PackIndex) NumObjects() uint32 { return p.numObjects }

// Fanout returns the fanout table.
func (p *PackIndex) Fanout() [256]uint32 { return p.fanout }

// Offsets returns a copy of the object offset map.
func (p *PackIndex) Offsets() map[Hash]int64 {
	cp := make(map[Hash]int64, len(p.offsets))
	for k, v := range p.offsets {
		cp[k] = v
	}
	return cp
}

// StashEntry represents a single Git stash entry.
type StashEntry struct {
	Hash    Hash   `json:"hash"`
	Message string `json:"message"`
}

// RepositoryDelta represents the difference between two repositories in a digestable format.
// This structure gets sent to the front end during live updates.
type RepositoryDelta struct {
	AddedCommits   []*Commit `json:"addedCommits"`
	DeletedCommits []*Commit `json:"deletedCommits"`

	AddedBranches   map[string]Hash `json:"addedBranches"`
	AmendedBranches map[string]Hash `json:"amendedBranches"`
	DeletedBranches map[string]Hash `json:"deletedBranches"`

	// HeadHash is the current HEAD commit hash, sent on every delta.
	HeadHash string `json:"headHash"`
	// Tags maps tag names to their target commit hashes (annotated tags are peeled).
	Tags map[string]string `json:"tags"`
	// Stashes contains all stash entries, newest first.
	Stashes []StashEntry `json:"stashes"`
}

// NewRepositoryDelta returns a new RepositoryDelta struct.
func NewRepositoryDelta() *RepositoryDelta {
	return &RepositoryDelta{
		AddedBranches:   make(map[string]Hash),
		AmendedBranches: make(map[string]Hash),
		DeletedBranches: make(map[string]Hash),
		Tags:            make(map[string]string),
		Stashes:         []StashEntry{},
	}
}

// IsEmpty reports whether a RepositoryDelta represents no difference.
func (d *RepositoryDelta) IsEmpty() bool {
	return len(d.AddedCommits) == 0 &&
		len(d.DeletedCommits) == 0 &&
		len(d.AddedBranches) == 0 &&
		len(d.DeletedBranches) == 0 &&
		len(d.AmendedBranches) == 0
}

// DiffStatus represents the type of change made to a file.
type DiffStatus int

const (
	// DiffStatusAdded indicates a file was added
	DiffStatusAdded DiffStatus = iota
	// DiffStatusModified indicates a file was modified
	DiffStatusModified
	// DiffStatusDeleted indicates a file was deleted
	DiffStatusDeleted
	// DiffStatusRenamed indicates a file was renamed
	DiffStatusRenamed
)

// String returns the string representation of a DiffStatus.
func (s DiffStatus) String() string {
	switch s {
	case DiffStatusAdded:
		return "added"
	case DiffStatusModified:
		return "modified"
	case DiffStatusDeleted:
		return "deleted"
	case DiffStatusRenamed:
		return "renamed"
	default:
		return "unknown"
	}
}

// DiffEntry represents a single file change in a commit.
type DiffEntry struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"oldPath,omitempty"` // Set for renamed files
	Status    DiffStatus `json:"status"`
	OldHash   Hash       `json:"oldHash,omitempty"`
	NewHash   Hash       `json:"newHash,omitempty"`
	IsBinary  bool       `json:"isBinary"`
	OldMode   string     `json:"oldMode,omitempty"`
	NewMode   string     `json:"newMode,omitempty"`
}

// CommitDiff represents the full set of changes in a commit.
type CommitDiff struct {
	CommitHash Hash        `json:"commitHash"`
	Entries    []DiffEntry `json:"entries"`
	Stats      DiffStats   `json:"stats"`
}

// DiffStats provides summary counts of changes.
type DiffStats struct {
	FilesChanged int `json:"filesChanged"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// DiffLine represents a single line in a diff.
type DiffLine struct {
	Type    string `json:"type"`    // "context", "addition", "deletion"
	Content string `json:"content"` // Line content without prefix
	OldLine int    `json:"oldLine"` // Line number in old file (0 if addition)
	NewLine int    `json:"newLine"` // Line number in new file (0 if deletion)
}

// DiffHunk represents a contiguous block of line changes.
type DiffHunk struct {
	OldStart int        `json:"oldStart"` // Starting line in old file
	OldLines int        `json:"oldLines"` // Number of lines in old file
	NewStart int        `json:"newStart"` // Starting line in new file
	NewLines int        `json:"newLines"` // Number of lines in new file
	Lines    []DiffLine `json:"lines"`    // All lines in this hunk
}

// FileDiff represents the full line-level diff for a single file.
type FileDiff struct {
	Path      string     `json:"path"`
	OldHash   Hash       `json:"oldHash"`
	NewHash   Hash       `json:"newHash"`
	IsBinary  bool       `json:"isBinary"`
	Truncated bool       `json:"truncated"` // True if file too large
	Hunks     []DiffHunk `json:"hunks"`
}
