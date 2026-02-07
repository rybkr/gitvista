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
func (h Hash) Short() string {
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
	NoneObject   ObjectType = 0
	CommitObject ObjectType = 1
	TreeObject   ObjectType = 2
	TagObject    ObjectType = 4
)

// StrToObjectType converts a string representation of an object type to an ObjectType.
func StrToObjectType(s string) ObjectType {
	switch s {
	case "commit":
		return CommitObject
	case "tag":
		return TagObject
	case "tree":
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

// RepositoryDelta represents the difference between two repositories in a digestable format.
// This structure gets sent to the front end during live updates.
type RepositoryDelta struct {
	AddedCommits   []*Commit `json:"addedCommits"`
	DeletedCommits []*Commit `json:"deletedCommits"`

	AddedBranches   map[string]Hash `json:"addedBranches"`
	AmendedBranches map[string]Hash `json:"amendedBranches"`
	DeletedBranches map[string]Hash `json:"deletedBranches"`
}

// NewRepositoryDelta returns a new RepositoryDelta struct.
func NewRepositoryDelta() *RepositoryDelta {
	return &RepositoryDelta{
		AddedBranches:   make(map[string]Hash),
		AmendedBranches: make(map[string]Hash),
		DeletedBranches: make(map[string]Hash),
	}
}

// IsEmpty reports whether a RepositoryDelta represents no difference.
func (d *RepositoryDelta) IsEmpty() bool {
	return len(d.AddedCommits) == 0 && len(d.DeletedCommits) == 0
}
