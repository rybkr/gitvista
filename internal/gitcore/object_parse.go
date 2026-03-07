package gitcore

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

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

// parseTreeBody parses the binary tree format: repeated (mode SP name NUL hash[20]) entries.
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

		var entryType string
		if strings.HasPrefix(mode, "100") {
			entryType = objectTypeBlob
		} else if mode == "040000" || mode == "40000" {
			entryType = objectTypeTree
		} else if mode == "120000" || mode == "160000" {
			entryType = objectTypeCommit
		} else {
			entryType = objectTypeUnknown
		}

		tree.Entries = append(tree.Entries, TreeEntry{
			ID:   hash,
			Name: name,
			Mode: mode,
			Type: entryType,
		})
	}
}
