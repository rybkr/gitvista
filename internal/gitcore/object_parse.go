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
			entryType = StatusUnknown
		}

		tree.Entries = append(tree.Entries, TreeEntry{
			ID:   hash,
			Name: name,
			Mode: mode,
			Type: entryType,
		})
	}
}
