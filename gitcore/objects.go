package gitcore

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// String returns the Git object type name (e.g., "commit", "tree", "blob", "tag").
func (t ObjectType) String() string {
	switch t {
	case ObjectTypeCommit:
		return "commit"
	case ObjectTypeTree:
		return "tree"
	case ObjectTypeBlob:
		return "blob"
	case ObjectTypeTag:
		return "tag"
	default:
		return "invalid"
	}
}

// ParseObjectType converts a string representation of an object type to an ObjectType.
func ParseObjectType(s string) ObjectType {
	switch s {
	case "commit":
		return ObjectTypeCommit
	case "tree":
		return ObjectTypeTree
	case "blob":
		return ObjectTypeBlob
	case "tag":
		return ObjectTypeTag
	default:
		return ObjectTypeInvalid
	}
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
// It is used to define a common contract for each Obejct type.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
type Object interface {
	Type() ObjectType
}

// ObjectResolver retrieves raw object data and type by hash.
// Used for resolving delta base objects during pack file reading.
type ObjectResolver func(id Hash, depth int) (data []byte, objectType ObjectType, err error)

// Commit represents a Git commit object.
// See: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
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

func (r *Repository) loadObjects() error {
	visited := make(map[Hash]bool)
	stack := make([]Hash, 0, len(r.refs)+len(r.stashes))
	for _, ref := range r.refs {
		stack = append(stack, ref)
	}
	for _, stash := range r.stashes {
		stack = append(stack, stash.Hash)
	}

	// We use an iterative stack to avoid stack overflow on repositories with a deep
	// linear history (100K+ commits).
	for len(stack) > 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[ref] {
			continue
		}
		visited[ref] = true

		object, err := r.readObject(ref)
		if err != nil {
			return fmt.Errorf("error traversing object: %w", err)
		}

		switch object.Type() {
		case ObjectTypeCommit:
			commit, ok := object.(*Commit)
			if !ok {
				return fmt.Errorf("unexpected type for commit object %s", ref)
			}
			r.commits = append(r.commits, commit)
			stack = append(stack, commit.Parents...)
		case ObjectTypeTag:
			tag, ok := object.(*Tag)
			if !ok {
				return fmt.Errorf("unexpected type for tag object %s", ref)
			}
			r.tags = append(r.tags, tag)
			stack = append(stack, tag.Object)
		case ObjectTypeTree, ObjectTypeBlob:
			continue
		default:
			return fmt.Errorf("unsupported object type: %d", object.Type())
		}
	}

	r.commitMap = make(map[Hash]*Commit, len(r.commits))
	for _, c := range r.commits {
		r.commitMap[c.ID] = c
	}

	return nil
}

func (r *Repository) readObject(id Hash) (Object, error) {
	if location, found := r.packLocations[id]; found {
		objectData, objectType, err := r.readPackedObjectData(location.packPath, location.offset, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to read pack object: %w", err)
		}
		return parseObject(id, objectType, objectData)
	}

	header, content, err := r.readLooseObjectRaw(id)
	if err == nil {
		objectType, err := objectTypeFromHeader(header)
		if err != nil {
			return nil, fmt.Errorf("unrecognized loose object type: %q for %s", header, id)
		}
		return parseObject(id, objectType, content)
	}

	return nil, fmt.Errorf("object not found: %s", id)
}

func (r *Repository) readObjectData(id Hash, depth int) ([]byte, ObjectType, error) {
	if location, found := r.packLocations[id]; found {
		return r.readPackedObjectData(location.packPath, location.offset, depth)
	}

	header, content, err := r.readLooseObjectRaw(id)
	if err == nil {
		objectType, err := objectTypeFromHeader(header)
		if err != nil {
			return nil, ObjectTypeInvalid, err
		}
		return content, objectType, nil
	}

	return nil, ObjectTypeInvalid, fmt.Errorf("object not found: %s", id)
}

func (r *Repository) readLooseObjectRaw(id Hash) (header string, content []byte, err error) {
	path := filepath.Join(r.gitDir, "objects", string(id)[:2], string(id)[2:])

	//nolint:gosec // G304: Object paths are controlled by git repository structure
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := readCompressedData(file)
	if err != nil {
		return "", nil, fmt.Errorf("invalid compressed data: %w", err)
	}

	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx == -1 {
		return "", nil, fmt.Errorf("invalid object format")
	}

	header, content = string(data[:nullIdx]), data[nullIdx+1:]
	return header, content, nil
}

func readCompressedData(r io.Reader) ([]byte, error) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer func() { _ = zr.Close() }()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(zr, maxDecompressedSize+1)); err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}
	if buf.Len() > maxDecompressedSize {
		return nil, fmt.Errorf("decompressed object exceeds maximum allowed size (%d bytes)", maxDecompressedSize)
	}

	return buf.Bytes(), nil
}

const (
	// Objects larger than this size are rejected to prevent zip-bomb style attacks.
	maxDecompressedSize = 256 * 1024 * 1024
)

func parseObject(id Hash, objectType ObjectType, objectData []byte) (Object, error) {
	switch objectType {
	case ObjectTypeCommit:
		return parseCommitBody(objectData, id)
	case ObjectTypeTag:
		return parseTagBody(objectData, id)
	case ObjectTypeTree:
		return parseTreeBody(objectData, id)
	case ObjectTypeBlob:
		return &Blob{ID: id}, nil
	default:
		return nil, fmt.Errorf("unknown object type: %d", objectType)
	}
}

func parseCommitBody(body []byte, id Hash) (*Commit, error) {
	commit := &Commit{ID: id}
	headerEnd := bytes.Index(body, []byte("\n\n"))
	headers := body
	if headerEnd >= 0 {
		headers = body[:headerEnd]
		commit.Message = strings.TrimSpace(string(body[headerEnd+2:]))
	}

	start := 0
	for start <= len(headers) {
		end := bytes.IndexByte(headers[start:], '\n')
		if end < 0 {
			end = len(headers) - start
		}
		line := headers[start : start+end]
		if len(line) > 0 {
			if err := parseCommitHeaderLine(commit, line); err != nil {
				return nil, err
			}
		}
		start += end + 1
		if start > len(headers) {
			break
		}
	}

	return commit, nil
}

func parseCommitHeaderLine(commit *Commit, line []byte) error {
	switch {
	case bytes.HasPrefix(line, []byte("parent ")):
		parent, err := NewHash(string(line[len("parent "):]))
		if err != nil {
			return fmt.Errorf("invalid parent hash: %w", err)
		}
		commit.Parents = append(commit.Parents, parent)
	case bytes.HasPrefix(line, []byte("tree ")):
		tree, err := NewHash(string(line[len("tree "):]))
		if err != nil {
			return fmt.Errorf("invalid tree hash: %w", err)
		}
		commit.Tree = tree
	case bytes.HasPrefix(line, []byte("author ")):
		author, err := NewSignature(string(line[len("author "):]))
		if err != nil {
			return fmt.Errorf("invalid author signature: %w", err)
		}
		commit.Author = author
	case bytes.HasPrefix(line, []byte("committer ")):
		committer, err := NewSignature(string(line[len("committer "):]))
		if err != nil {
			return fmt.Errorf("invalid committer signature: %w", err)
		}
		commit.Committer = committer
	}
	return nil
}

func parseTreeBody(body []byte, id Hash) (*Tree, error) {
	tree := &Tree{
		ID:      id,
		Entries: make([]TreeEntry, 0),
	}
	reader := bytes.NewReader(body)

	for {
		var modeBuilder strings.Builder
		for {
			b, err := reader.ReadByte()
			if err == io.EOF {
				return tree, nil
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read mode: %w", err)
			}
			if b == ' ' {
				break
			}
			modeBuilder.WriteByte(b)
		}
		mode := modeBuilder.String()

		var nameBuilder strings.Builder
		for {
			b, err := reader.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("failed to read name: %w", err)
			}
			if b == 0 {
				break
			}
			nameBuilder.WriteByte(b)
		}
		name := nameBuilder.String()

		var hashBytes [20]byte
		if _, err := io.ReadFull(reader, hashBytes[:]); err != nil {
			return nil, fmt.Errorf("failed to read hash: %w", err)
		}

		hash, err := NewHashFromBytes(hashBytes)
		if err != nil {
			return nil, fmt.Errorf("invalid hash in tree entry: %w", err)
		}

		var entryType ObjectType
		if strings.HasPrefix(mode, "100") {
			entryType = ObjectTypeBlob
		} else if mode == "040000" || mode == "40000" {
			entryType = ObjectTypeTree
		} else if mode == "120000" || mode == "160000" {
			entryType = ObjectTypeCommit
		} else {
			entryType = ObjectTypeInvalid
		}

		tree.Entries = append(tree.Entries, TreeEntry{
			ID:   hash,
			Name: name,
			Mode: mode,
			Type: entryType,
		})
	}
}

func parseTagBody(body []byte, id Hash) (*Tag, error) {
	tag := &Tag{ID: id}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	inMessage := false
	var messageLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if inMessage {
			messageLines = append(messageLines, line)
			continue
		}
		if line == "" {
			inMessage = true
			continue
		}

		if strings.HasPrefix(line, "object ") {
			objectHash, err := NewHash(strings.TrimPrefix(line, "object "))
			if err != nil {
				return nil, fmt.Errorf("invalid object hash: %w", err)
			}
			tag.Object = objectHash
		} else if strings.HasPrefix(line, "type ") {
			typeStr := strings.TrimPrefix(line, "type ")
			tag.ObjType = ParseObjectType(typeStr)
		} else if strings.HasPrefix(line, "tag ") {
			tag.Name = strings.TrimPrefix(line, "tag ")
		} else if strings.HasPrefix(line, "tagger ") {
			taggerLine := strings.TrimPrefix(line, "tagger ")
			tagger, err := NewSignature(taggerLine)
			if err != nil {
				return nil, fmt.Errorf("invalid tagger: %w", err)
			}
			tag.Tagger = tagger
		}
	}

	tag.Message = strings.Join(messageLines, "\n")
	tag.Message = strings.TrimSpace(tag.Message)

	return tag, nil
}
