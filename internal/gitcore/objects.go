// Package gitcore provides pure Go implementation of Git object parsing and repository traversal.
package gitcore

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	objectTypeCommit = "commit"
	objectTypeTree   = "tree"
	objectTypeBlob   = "blob"
	objectTypeTag    = "tag"
)

// loadObjects loads all Git objects into the object store.
// It traverses all references and their histories.
// It assumes that all references have already been loaded.
func (r *Repository) loadObjects() {
	visited := make(map[Hash]bool)
	for _, ref := range r.refs {
		r.traverseObjects(ref, visited)
	}
}

// traverseObjects recursively loads all objects beginning from the provided reference,
// using the visited map to avoid processing the same object multiple times.
func (r *Repository) traverseObjects(ref Hash, visited map[Hash]bool) {
	if visited[ref] {
		return
	}
	visited[ref] = true

	object, err := r.readObject(ref)
	if err != nil {
		// Log the error but continue with other potentially valid objects.
		log.Printf("error traversing object: %v", err)
		return
	}

	switch object.Type() {
	case CommitObject:
		commit := object.(*Commit)
		r.commits = append(r.commits, commit)
		for _, parent := range commit.Parents {
			r.traverseObjects(parent, visited)
		}
	case TagObject:
		tag := object.(*Tag)
		r.tags = append(r.tags, tag)
		r.traverseObjects(tag.Object, visited)
	default:
		// Unrecognized type, log the error but continue on.
		log.Printf("unsupported object type: %d", object.Type())
	}
}

// readObject parses an object from its hash.
// It first attempts to read from loose objects, then falls back to pack files.
// Parse errors from loose objects are returned immediately rather than silently
// falling through to the pack search — a corrupt loose object should fail loudly.
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

// readObjectData reads any object, loose or packed, and returns raw data.
func (r *Repository) readObjectData(id Hash) ([]byte, byte, error) {
	header, content, err := r.readLooseObjectRaw(id)
	if err == nil {
		typeNum, err := objectTypeFromHeader(header)
		if err != nil {
			return nil, 0, err
		}
		return content, typeNum, nil
	}

	for _, idx := range r.packIndices {
		if offset, found := idx.FindObject(id); found {
			return r.readFromPackFile(idx.PackFile(), offset)
		}
	}

	return nil, 0, fmt.Errorf("object not found: %s", id)
}

// readFromPackFile opens a pack file, seeks to offset, and reads a pack object.
// Scoping the open+defer+close to this function prevents file descriptor leaks
// when this is called inside a loop (defer runs at function return, not loop end).
func (r *Repository) readFromPackFile(packPath string, offset int64) ([]byte, byte, error) {
	//nolint:gosec // G304: Pack file paths are controlled by git repository structure
	file, err := os.Open(packPath)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close pack file: %v", err)
		}
	}()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}
	return readPackObject(file, r.readObjectData)
}

// readLooseObjectRaw reads a loose object from disk and returns its header and content.
// This is the common implementation used by both readLooseObject and readLooseObjectData.
func (r *Repository) readLooseObjectRaw(id Hash) (header string, content []byte, err error) {
	objectPath := filepath.Join(r.gitDir, "objects", string(id)[:2], string(id)[2:])

	//nolint:gosec // G304: Object paths are controlled by git repository structure
	file, err := os.Open(objectPath)
	if err != nil {
		return "", nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close loose object file: %v", err)
		}
	}()

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

// objectTypeFromHeader converts a Git object header string to its pack object type byte.
// Uses the same numeric constants as the pack format (packObjectCommit, etc.).
func objectTypeFromHeader(header string) (byte, error) {
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

// readPackedObject reads an object from a pack file at the given offset and parses it.
func (r *Repository) readPackedObject(packPath string, offset int64, id Hash) (Object, error) {
	objectData, objectType, err := r.readFromPackFile(packPath, offset)
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
	default:
		return nil, fmt.Errorf("unknown object type: %d", objectType)
	}
}

// parseCommitBody parses the body of a commit object into a Commit struct.
func parseCommitBody(body []byte, id Hash) (*Commit, error) {
	commit := &Commit{ID: id}
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

		if strings.HasPrefix(line, "parent ") {
			parent, err := NewHash(strings.TrimPrefix(line, "parent "))
			if err != nil {
				return nil, fmt.Errorf("invalid parent hash: %w", err)
			}
			commit.Parents = append(commit.Parents, parent)
		} else if strings.HasPrefix(line, "tree ") {
			tree, err := NewHash(strings.TrimPrefix(line, "tree "))
			if err != nil {
				return nil, fmt.Errorf("invalid tree hash: %w", err)
			}
			commit.Tree = tree
		} else if strings.HasPrefix(line, "author ") {
			authorLine := strings.TrimPrefix(line, "author ")
			author, err := NewSignature(authorLine)
			if err != nil {
				return nil, fmt.Errorf("invalid author signature: %w", err)
			}
			commit.Author = author
		} else if strings.HasPrefix(line, "committer ") {
			committerLine := strings.TrimPrefix(line, "committer ")
			committer, err := NewSignature(committerLine)
			if err != nil {
				return nil, fmt.Errorf("invalid committer signature: %w", err)
			}
			commit.Committer = committer
		}
	}

	commit.Message = strings.Join(messageLines, "\n")
	commit.Message = strings.TrimSpace(commit.Message)

	return commit, nil
}

// parseTagBody parses the body of a tag object into a Tag struct.
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
			tag.ObjType = StrToObjectType(typeStr)
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

// parseTreeBody parses the body of a tree object into a Tree struct.
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

		// Determine type based on mode:
		//  - 100644/100755 = blob (file)
		//  - 040000 = tree (directory)
		//  - 120000/160000 = commit (submodule)
		var entryType string
		if strings.HasPrefix(mode, "100") {
			entryType = "blob"
		} else if mode == "040000" || mode == "40000" {
			entryType = "tree"
		} else if mode == "120000" || mode == "160000" {
			entryType = "commit"
		} else {
			entryType = "unknown"
		}

		tree.Entries = append(tree.Entries, TreeEntry{
			ID:   hash,
			Name: name,
			Mode: mode,
			Type: entryType,
		})
	}
}

// maxDecompressedSize caps the size of any single decompressed Git object.
// Objects larger than this are rejected to prevent zip-bomb style attacks.
const maxDecompressedSize = 256 * 1024 * 1024 // 256MB

// readCompressedData reads and decompresses zlib-compressed data from the given reader.
// Returns an error if the decompressed output exceeds maxDecompressedSize.
func readCompressedData(r io.Reader) ([]byte, error) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer func() {
		if err := zr.Close(); err != nil {
			log.Printf("failed to close zlib reader: %v", err)
		}
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(zr, maxDecompressedSize+1)); err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}
	if buf.Len() > maxDecompressedSize {
		return nil, fmt.Errorf("decompressed object exceeds maximum allowed size (%d bytes)", maxDecompressedSize)
	}

	return buf.Bytes(), nil
}
