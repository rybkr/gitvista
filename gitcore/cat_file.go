package gitcore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CatFileMode selects which cat-file operation to perform for a revision.
type CatFileMode int

const (
	CatFileModeType CatFileMode = iota
	CatFileModeSize
	CatFileModePretty
)

// CatFileOptions configures a Repository.CatFile lookup.
type CatFileOptions struct {
	Revision string
	Mode     CatFileMode
}

// CatFileResult contains the resolved object metadata and optional pretty output.
type CatFileResult struct {
	Hash Hash
	Type ObjectType
	Size int
	Data []byte
}

// CatFile resolves a revision to an object and returns its type, size, or pretty output.
func (r *Repository) CatFile(opts CatFileOptions) (*CatFileResult, error) {
	hash, err := r.resolveObjectRevision(opts.Revision)
	if err != nil {
		return nil, err
	}

	data, objectType, err := r.readObjectData(hash, 0)
	if err != nil {
		return nil, fmt.Errorf("object not found: %s", hash)
	}

	result := &CatFileResult{
		Hash: hash,
		Type: objectType,
		Size: len(data),
	}

	if opts.Mode != CatFileModePretty {
		return result, nil
	}

	result.Data, err = formatCatFilePretty(hash, objectType, data)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (r *Repository) resolveObjectRevision(revision string) (Hash, error) {
	if revision == "HEAD" {
		head := r.Head()
		if head == "" {
			return "", ambiguousObjectRevisionError(revision)
		}
		return head, nil
	}

	if revision == "" {
		return "", ambiguousObjectRevisionError(revision)
	}

	if hash, ok := r.Branches()[revision]; ok {
		return hash, nil
	}

	if hash, ok := r.GraphBranches()[revision]; ok {
		return hash, nil
	}

	if hash, ok := r.refs["refs/tags/"+revision]; ok {
		return hash, nil
	}

	if hash, err := NewHash(revision); err == nil {
		if _, _, readErr := r.readObjectData(hash, 0); readErr == nil {
			return hash, nil
		}
	}

	matches, err := r.matchingObjectHashes(revision)
	if err != nil {
		return "", err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	return "", ambiguousObjectRevisionError(revision)
}

func (r *Repository) matchingObjectHashes(prefix string) ([]Hash, error) {
	if len(prefix) == 0 || len(prefix) >= 40 {
		return nil, nil
	}

	matches := make(map[Hash]struct{})

	for _, hash := range r.knownObjectHashes() {
		if strings.HasPrefix(string(hash), prefix) {
			matches[hash] = struct{}{}
		}
	}

	looseMatches, err := r.matchLooseObjectHashes(prefix)
	if err != nil {
		return nil, err
	}
	for _, hash := range looseMatches {
		matches[hash] = struct{}{}
	}

	out := make([]Hash, 0, len(matches))
	for hash := range matches {
		out = append(out, hash)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *Repository) knownObjectHashes() []Hash {
	seen := make(map[Hash]struct{})
	add := func(hash Hash) {
		if hash == "" {
			return
		}
		seen[hash] = struct{}{}
	}

	add(r.Head())
	for _, hash := range r.refs {
		add(hash)
	}
	for hash := range r.commitMap {
		add(hash)
	}
	for _, tag := range r.tags {
		add(tag.ID)
		add(tag.Object)
	}
	for hash := range r.packLocations {
		add(hash)
	}

	out := make([]Hash, 0, len(seen))
	for hash := range seen {
		out = append(out, hash)
	}
	return out
}

func (r *Repository) matchLooseObjectHashes(prefix string) ([]Hash, error) {
	if len(prefix) < 2 {
		return nil, nil
	}

	dir := filepath.Join(r.gitDir, "objects", prefix[:2])
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	suffixPrefix := prefix[2:]
	matches := make([]Hash, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if len(name) != 38 || !strings.HasPrefix(name, suffixPrefix) {
			continue
		}

		hash, err := NewHash(prefix[:2] + name)
		if err != nil {
			continue
		}
		matches = append(matches, hash)
	}

	return matches, nil
}

func ambiguousObjectRevisionError(revision string) error {
	return fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
}

func formatCatFilePretty(hash Hash, objectType ObjectType, data []byte) ([]byte, error) {
	switch objectType {
	case ObjectTypeBlob, ObjectTypeCommit, ObjectTypeTag:
		return append([]byte(nil), data...), nil
	case ObjectTypeTree:
		tree, err := parseTreeBody(data, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tree object: %w", err)
		}
		return formatTreePretty(tree), nil
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objectType.String())
	}
}

func formatTreePretty(tree *Tree) []byte {
	var buf bytes.Buffer
	for _, entry := range tree.Entries {
		fmt.Fprintf(&buf, "%s %s %s\t%s\n", entry.Mode, entry.Type.String(), entry.ID, entry.Name)
	}
	return buf.Bytes()
}
