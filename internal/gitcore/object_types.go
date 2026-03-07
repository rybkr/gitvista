package gitcore

import (
	"fmt"
	"strings"
)

const (
	objectTypeUnknown = "unknown"
	objectTypeCommit  = "commit"
	objectTypeTree    = "tree"
	objectTypeBlob    = "blob"
	objectTypeTag     = "tag"
)

// Object represents a generic Git object.
type Object interface {
	Type() ObjectType
}

// ObjectType uses the same numeric values as the Git pack format.
// See: https://git-scm.com/docs/pack-format#_object_types
type ObjectType int

//nolint:revive // See: https://git-scm.com/docs/pack-format#_object_types
const (
	NoneObject   ObjectType = 0
	CommitObject ObjectType = 1
	TreeObject   ObjectType = 2
	BlobObject   ObjectType = 3
	TagObject    ObjectType = 4
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
		return objectTypeUnknown
	}
}

// ParseObjectType converts a string representation of an object type to an ObjectType.
func ParseObjectType(s string) ObjectType {
	switch s {
	case objectTypeCommit:
		return CommitObject
	case objectTypeTag:
		return TagObject
	case objectTypeBlob:
		return BlobObject
	case objectTypeTree:
		return TreeObject
	default:
		return NoneObject
	}
}

// StrToObjectType is kept as a compatibility wrapper for older call sites.
func StrToObjectType(s string) ObjectType {
	return ParseObjectType(s)
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

// Blob represents a Git blob object. Only the hash is stored since the
// traversal does not need the content, because blobs are terminal nodes.
type Blob struct {
	ID Hash `json:"hash"`
}

// Type returns the ObjectType for a Blob.
func (b *Blob) Type() ObjectType {
	return BlobObject
}

// ObjectResolver retrieves raw object data and type byte by hash.
// Used for resolving delta base objects during pack file reading.
type ObjectResolver func(id Hash, depth int) (data []byte, objectType byte, err error)

func ObjectTypeFromHeader(header string) (byte, error) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid header: %s", header)
	}

	switch parts[0] {
	case objectTypeCommit:
		return packObjectCommit, nil
	case objectTypeTree:
		return packObjectTree, nil
	case objectTypeBlob:
		return packObjectBlob, nil
	case objectTypeTag:
		return packObjectTag, nil
	default:
		return 0, fmt.Errorf("unsupported object type: %s", parts[0])
	}
}
