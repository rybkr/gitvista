package gitcore

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// loadObjects traverses all refs and stash entries, loading reachable commits
// and tags using an iterative stack to avoid stack overflow on repositories
// with deep linear history (100K+ commits). Semantics are identical to the
// former recursive implementation: visited map prevents re-processing, and
// errors propagate immediately. Output ordering may differ (LIFO vs
// first-parent-first) but consumers use unordered maps or re-sort by date,
// so this is invisible.
// Must be called after loadRefs and loadStashes.
func (r *Repository) loadObjects() error {
	visited := make(map[Hash]bool)

	stack := make([]Hash, 0, len(r.refs)+len(r.stashes))
	for _, ref := range r.refs {
		stack = append(stack, ref)
	}
	for _, stash := range r.stashes {
		stack = append(stack, stash.Hash)
	}

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
		case CommitObject:
			commit, ok := object.(*Commit)
			if !ok {
				return fmt.Errorf("unexpected type for commit object %s", ref)
			}
			r.commits = append(r.commits, commit)
			stack = append(stack, commit.Parents...)
		case TagObject:
			tag, ok := object.(*Tag)
			if !ok {
				return fmt.Errorf("unexpected type for tag object %s", ref)
			}
			r.tags = append(r.tags, tag)
			stack = append(stack, tag.Object)
		case TreeObject, BlobObject:
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

// readObject reads and parses a Git object (loose first, then packed).
// Parse errors from loose objects are returned immediately rather than silently
// falling through to the pack search - a corrupt loose object should fail loudly.
func (r *Repository) readObject(id Hash) (Object, error) {
	header, content, err := r.readLooseObjectRaw(id)
	if err == nil {
		switch {
		case strings.HasPrefix(header, objectTypeCommit):
			return parseCommitBody(content, id)
		case strings.HasPrefix(header, objectTypeTag):
			return parseTagBody(content, id)
		case strings.HasPrefix(header, objectTypeTree):
			return parseTreeBody(content, id)
		case strings.HasPrefix(header, objectTypeBlob):
			return &Blob{ID: id}, nil
		default:
			return nil, fmt.Errorf("unrecognized loose object type: %q for %s", header, id)
		}
	}

	for _, packIndex := range r.packIndices {
		if offset, found := packIndex.FindObject(id); found {
			return r.readPackedObject(packIndex.PackFile(), offset, id)
		}
	}

	return nil, fmt.Errorf("object not found: %s", id)
}

// readObjectData returns raw bytes and type byte for any object (loose or packed).
func (r *Repository) readObjectData(id Hash, depth int) ([]byte, byte, error) {
	header, content, err := r.readLooseObjectRaw(id)
	if err == nil {
		typeNum, err := ObjectTypeFromHeader(header)
		if err != nil {
			return nil, 0, err
		}
		return content, typeNum, nil
	}

	for _, idx := range r.packIndices {
		if offset, found := idx.FindObject(id); found {
			return r.readFromPackFile(idx.PackFile(), offset, depth)
		}
	}

	return nil, 0, fmt.Errorf("object not found: %s", id)
}

// readFromPackFile opens a pack file, seeks to offset, and reads a pack object.
// Scoped to its own function so defer closes the file after each call,
// preventing fd leaks when called in a loop.
func (r *Repository) readFromPackFile(packPath string, offset int64, depth int) ([]byte, byte, error) {
	//nolint:gosec // G304: Pack file paths are controlled by git repository structure
	file, err := os.Open(packPath)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}
	return readPackObject(file, r.readObjectData, depth)
}

// readLooseObjectRaw reads and decompresses a loose object, returning its header and content.
func (r *Repository) readLooseObjectRaw(id Hash) (header string, content []byte, err error) {
	objectPath := filepath.Join(r.gitDir, "objects", string(id)[:2], string(id)[2:])

	//nolint:gosec // G304: Object paths are controlled by git repository structure
	file, err := os.Open(objectPath)
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

func (r *Repository) readPackedObject(packPath string, offset int64, id Hash) (Object, error) {
	objectData, objectType, err := r.readFromPackFile(packPath, offset, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read pack object: %w", err)
	}

	switch ObjectType(objectType) {
	case CommitObject:
		return parseCommitBody(objectData, id)
	case TagObject:
		return parseTagBody(objectData, id)
	case TreeObject:
		return parseTreeBody(objectData, id)
	case BlobObject:
		return &Blob{ID: id}, nil
	default:
		return nil, fmt.Errorf("unknown object type: %d", objectType)
	}
}

// maxDecompressedSize caps the size of any single decompressed Git object.
// Objects larger than this are rejected to prevent zip-bomb style attacks.
const maxDecompressedSize = 256 * 1024 * 1024 // 256MB

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
