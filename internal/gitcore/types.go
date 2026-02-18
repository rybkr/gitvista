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
	NoneObject   ObjectType = 0
	CommitObject ObjectType = 1
	TreeObject   ObjectType = 2
	BlobObject   ObjectType = 3
	TagObject    ObjectType = 4
)

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

type Commit struct {
	ID        Hash      `json:"hash"`
	Tree      Hash      `json:"tree"`
	Parents   []Hash    `json:"parents"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
	Message   string    `json:"message"`
}

func (c *Commit) Type() ObjectType {
	return CommitObject
}

type Tag struct {
	ID      Hash       `json:"hash"`
	Object  Hash       `json:"object"`
	ObjType ObjectType `json:"objectType"`
	Name    string     `json:"name"`
	Tagger  Signature  `json:"tagger"`
	Message string     `json:"message"`
}

func (t *Tag) Type() ObjectType {
	return TagObject
}

type TreeEntry struct {
	ID   Hash   `json:"hash"`
	Name string `json:"name"`
	Mode string `json:"mode"`
	Type string `json:"type"`
}

type Tree struct {
	ID      Hash        `json:"hash"`
	Entries []TreeEntry `json:"entries"`
}

func (t *Tree) Type() ObjectType {
	return TreeObject
}

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

	return Signature{
		Name:  name,
		Email: email,
		When:  time.Unix(unixTime, 0),
	}, nil
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

func (p *PackIndex) FindObject(id Hash) (int64, bool) {
	offset, found := p.offsets[id]
	return offset, found
}

func (p *PackIndex) PackFile() string        { return p.packPath }
func (p *PackIndex) Version() uint32         { return p.version }
func (p *PackIndex) NumObjects() uint32      { return p.numObjects }
func (p *PackIndex) Fanout() [256]uint32     { return p.fanout }

// Offsets returns a defensive copy of the offset map.
func (p *PackIndex) Offsets() map[Hash]int64 {
	cp := make(map[Hash]int64, len(p.offsets))
	for k, v := range p.offsets {
		cp[k] = v
	}
	return cp
}

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
	Tags     map[string]string `json:"tags"`     // tag name -> target commit hash (annotated tags are peeled)
	Stashes  []StashEntry      `json:"stashes"`
}

func NewRepositoryDelta() *RepositoryDelta {
	return &RepositoryDelta{
		AddedBranches:   make(map[string]Hash),
		AmendedBranches: make(map[string]Hash),
		DeletedBranches: make(map[string]Hash),
		Tags:            make(map[string]string),
		Stashes:         []StashEntry{},
	}
}

func (d *RepositoryDelta) IsEmpty() bool {
	return len(d.AddedCommits) == 0 &&
		len(d.DeletedCommits) == 0 &&
		len(d.AddedBranches) == 0 &&
		len(d.DeletedBranches) == 0 &&
		len(d.AmendedBranches) == 0
}

type DiffStatus int

const (
	DiffStatusAdded DiffStatus = iota
	DiffStatusModified
	DiffStatusDeleted
	DiffStatusRenamed
)

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

type DiffEntry struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"oldPath,omitempty"`
	Status    DiffStatus `json:"status"`
	OldHash   Hash       `json:"oldHash,omitempty"`
	NewHash   Hash       `json:"newHash,omitempty"`
	IsBinary  bool       `json:"isBinary"`
	OldMode   string     `json:"oldMode,omitempty"`
	NewMode   string     `json:"newMode,omitempty"`
}

type CommitDiff struct {
	CommitHash Hash        `json:"commitHash"`
	Entries    []DiffEntry `json:"entries"`
	Stats      DiffStats   `json:"stats"`
}

type DiffStats struct {
	FilesChanged int `json:"filesChanged"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

type DiffLine struct {
	Type    string `json:"type"`    // "context", "addition", or "deletion"
	Content string `json:"content"`
	OldLine int    `json:"oldLine"` // 0 for additions
	NewLine int    `json:"newLine"` // 0 for deletions
}

type DiffHunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []DiffLine `json:"lines"`
}

type FileDiff struct {
	Path      string     `json:"path"`
	OldHash   Hash       `json:"oldHash"`
	NewHash   Hash       `json:"newHash"`
	IsBinary  bool       `json:"isBinary"`
	Truncated bool       `json:"truncated"`
	Hunks     []DiffHunk `json:"hunks"`
}
