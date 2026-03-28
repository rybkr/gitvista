package gitcore

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ObjectType assigns enumeration values to the Git object types.
// We use the same numeric values as the Git pack format specification.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
// See: https://git-scm.com/docs/pack-format#_object_types
type ObjectType int

//nolint:revive // See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
const (
	ObjectTypeInvalid  ObjectType = 0
	ObjectTypeCommit   ObjectType = 1
	ObjectTypeTree     ObjectType = 2
	ObjectTypeBlob     ObjectType = 3
	ObjectTypeTag      ObjectType = 4
	ObjectTypeReserved ObjectType = 5
	ObjectTypeOfsDelta ObjectType = 6
	ObjectTypeRefDelta ObjectType = 7
)

var objectTypeNames = map[ObjectType]string{
	ObjectTypeCommit:   "commit",
	ObjectTypeTree:     "tree",
	ObjectTypeBlob:     "blob",
	ObjectTypeTag:      "tag",
	ObjectTypeReserved: "reserved",
	ObjectTypeOfsDelta: "ofsdelta",
	ObjectTypeRefDelta: "refdelta",
}

var objectTypeValues = func() map[string]ObjectType {
	m := make(map[string]ObjectType, len(objectTypeNames))
	for k, v := range objectTypeNames {
		m[v] = k
	}
	return m
}()

// String returns the Git object type name (e.g., "commit", "tree", "blob", "tag").
func (t ObjectType) String() string {
	if name, ok := objectTypeNames[t]; ok {
		return name
	}
	return "invalid"
}

func (t ObjectType) MarshalJSON() ([]byte, error) {
	if t != ObjectTypeInvalid {
		if _, ok := objectTypeNames[t]; !ok {
			return nil, fmt.Errorf("invalid ObjectType: %d", t)
		}
	}
	return json.Marshal(t.String())
}

func (t *ObjectType) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("ObjectType must be a string: %w", err)
	}
	parsed := ParseObjectType(raw)
	if parsed == ObjectTypeInvalid && raw != ObjectTypeInvalid.String() {
		return fmt.Errorf("invalid ObjectType: %q", raw)
	}
	*t = parsed
	return nil
}

// ParseObjectType converts a string representation of an object type to an ObjectType.
func ParseObjectType(s string) ObjectType {
	if t, ok := objectTypeValues[s]; ok {
		return t
	}
	return ObjectTypeInvalid
}

func objectTypeFromHeader(header string) (ObjectType, error) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ObjectTypeInvalid, fmt.Errorf("invalid header: %s", header)
	}

	objectType := ParseObjectType(parts[0])
	if objectType == ObjectTypeInvalid {
		return ObjectTypeInvalid, fmt.Errorf("unsupported object type: %s", parts[0])
	}

	return objectType, nil
}

// Object represents a generic Git object.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
type Object interface {
	Type() ObjectType
}

// ObjectResolver retrieves raw object data and type by hash.
// It is used for resolving delta base objects during pack file reading.
type ObjectResolver func(id Hash, depth int) (data []byte, objectType ObjectType, err error)

// Commit represents a Git commit object.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
type Commit struct {
	ID                Hash      `json:"hash"`
	Tree              Hash      `json:"tree"`
	Parents           []Hash    `json:"parents"`
	Author            Signature `json:"author"`
	Committer         Signature `json:"committer"`
	Message           string    `json:"message"`
	BranchLabel       string    `json:"branchLabel,omitempty"`
	BranchLabelSource string    `json:"branchLabelSource,omitempty"`
}

// Type returns the ObjectType for a Commit.
func (c *Commit) Type() ObjectType {
	return ObjectTypeCommit
}

// TreeEntry represents a single entry within a Git tree object.
type TreeEntry struct {
	ID   Hash       `json:"hash"`
	Name string     `json:"name"`
	Mode string     `json:"mode"`
	Type ObjectType `json:"type"`
}

// Tree represents a Git tree object containing a list of entries.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
type Tree struct {
	ID      Hash        `json:"hash"`
	Entries []TreeEntry `json:"entries"`
}

// Type returns the ObjectType for a Tree.
func (t *Tree) Type() ObjectType {
	return ObjectTypeTree
}

// Blob represents a Git blob object. Only the hash is stored since the
// traversal does not need the content, because blobs are terminal nodes.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
type Blob struct {
	ID Hash `json:"hash"`
}

// Type returns the ObjectType for a Blob.
func (b *Blob) Type() ObjectType {
	return ObjectTypeBlob
}

// Tag represents a Git tag object.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
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
	return ObjectTypeTag
}
