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

// loadObjects loads all Git objects into the object store.
// It traverses all references and their histories.
// It assumes that all references have already been loaded.
func (r *Repository) loadObjects() error {
	visited := make(map[Hash]bool)
	for _, ref := range r.refs {
		r.traverseObjects(ref, visited)
	}

	return nil
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
func (r *Repository) readObject(id Hash) (Object, error) {
	header, content, err := r.readLooseObject(id)
	if err == nil {
		switch {
		case strings.HasPrefix(header, "commit"):
			if commit, err := r.parseCommitBody(content, id); err == nil {
				return commit, nil
			}
		case strings.HasPrefix(header, "tag"):
			if tag, err := r.parseTagBody(content, id); err == nil {
				return tag, nil
			}
        case strings.HasPrefix(header, "tree"):
            if tree, err := r.parseTreeBody(content, id); err == nil {
                return tree, nil
            }
		default:
			err = fmt.Errorf("unrecognized object: %q", header)
		}
	}

	for _, packIndex := range r.packIndices {
		if offset, found := packIndex.FindObject(id); found {
			return r.readPackedObject(packIndex.PackFile(), offset, id)
		}
	}

	// We didn't find the object in either packed or loose storage.
	return nil, err
}

// readObjectData reads any object, loose or packed, and returns raw data.
func (r *Repository) readObjectData(id Hash) ([]byte, byte, error) {
	objectPath := filepath.Join(r.gitDir, "objects", string(id)[:2], string(id)[2:])
	if _, err := os.Stat(objectPath); err == nil {
		return r.readLooseObjectData(objectPath)
	}

	for _, idx := range r.packIndices {
		if offset, found := idx.FindObject(id); found {
			file, err := os.Open(idx.PackFile())
			if err != nil {
				continue
			}
			defer file.Close()

			if _, err := file.Seek(offset, 0); err != nil {
				continue
			}
			return r.readPackObject(file)
		}
	}

	return nil, 0, fmt.Errorf("object not found: %s", id)
}

// readLooseObjectData reads a loose object and returns raw data.
func (r *Repository) readLooseObjectData(objectPath string) ([]byte, byte, error) {
	file, err := os.Open(objectPath)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	content, err := r.readCompressedData(file)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid compressed data: %w", err)
	}

	nullIdx := bytes.IndexByte(content, 0)
	if nullIdx == -1 {
		return nil, 0, fmt.Errorf("invalid object format")
	}

	header := string(content[:nullIdx])
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return nil, 0, fmt.Errorf("invalid header: %s", header)
	}

	objectType := parts[0]

	var typeNum byte
	switch objectType {
	case "commit":
		typeNum = 1
	case "tree":
		typeNum = 2
	case "blob":
		typeNum = 3
	case "tag":
		typeNum = 4
	default:
		return nil, 0, fmt.Errorf("unsupported object type: %s", objectType)
	}

	return content[nullIdx+1:], typeNum, nil
}

// readLooseObject reads an object from loose object storage.
func (r *Repository) readLooseObject(id Hash) (header string, content []byte, err error) {
	objectPath := filepath.Join(r.gitDir, "objects", string(id)[:2], string(id)[2:])

	file, err := os.Open(objectPath)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	content, err = r.readCompressedData(file)
	if err != nil {
		return "", nil, fmt.Errorf("invalid compressed data: %w", err)
	}

	nullIdx := bytes.IndexByte(content, 0)
	if nullIdx == -1 {
		return "", nil, fmt.Errorf("invalid object format")
	}

	header, content = string(content[:nullIdx]), content[nullIdx+1:]
	return header, content, nil
}

// readPackedObject reads an object from a pack file at the given offset.
func (r *Repository) readPackedObject(packPath string, offset int64, id Hash) (Object, error) {
	file, err := os.Open(packPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open pack file: %w", err)
	}
	defer file.Close()

	if _, err := file.Seek(offset, 0); err != nil {
		return nil, fmt.Errorf("failed to seek to offset %d: %w", offset, err)
	}

	objectData, objectType, err := r.readPackObject(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read pack object: %w", err)
	}

	switch ObjectType(objectType) {
	case CommitObject:
		return r.parseCommitBody(objectData, id)
	case TagObject:
		return r.parseTagBody(objectData, id)
    case TreeObject:
        return r.parseTreeBody(objectData, id)
	default:
		return nil, fmt.Errorf("unknown object type: %d", objectType)
	}
}

// parseCommitBody parses the body of a commit object into a Commit struct.
func (r *Repository) parseCommitBody(body []byte, id Hash) (*Commit, error) {
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
			parent := Hash(strings.TrimPrefix(line, "parent "))
			commit.Parents = append(commit.Parents, parent)
		} else if strings.HasPrefix(line, "tree ") {
			tree := Hash(strings.TrimPrefix(line, "tree "))
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
func (r *Repository) parseTagBody(body []byte, id Hash) (*Tag, error) {
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
func (r *Repository) parseTreeBody(body []byte, id Hash) (*Tree, error) {
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
		} else if mode == "040000" {
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

// readCompressedData reads and decompresses zlib-compressed data at the current file position.
func (r *Repository) readCompressedData(file *os.File) ([]byte, error) {
	zr, err := zlib.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer zr.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zr); err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return buf.Bytes(), nil
}
