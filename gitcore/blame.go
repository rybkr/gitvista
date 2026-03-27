package gitcore

import (
	"fmt"
	"time"
)

// BlameEntry represents the last-modified metadata for a directory entry.
// See: https://git-scm.com/docs/git-blame
type BlameEntry struct {
	CommitHash    Hash      `json:"commitHash"`
	CommitMessage string    `json:"commitMessage"`
	AuthorName    string    `json:"authorName"`
	When          time.Time `json:"when"`
}

const (
	maxBlameDepth = 1000
)

// GetFileBlame returns per-entry last-modified info for the immediate children of dirPath at the given commit.
// Traversal is capped at maxBlameDepth levels. Any entries still unresolved above this depth are assigned to
// targetCommit as a best-effort fallback.
func (r *Repository) GetFileBlame(commitHash Hash, dirPath string) (map[string]*BlameEntry, error) {
	commits := r.Commits()
	targetCommit, ok := commits[commitHash]
	if !ok {
		return nil, fmt.Errorf("commit not found: %s", commitHash)
	}

	targetTree, err := r.resolveTreeAtPath(targetCommit.Tree, dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tree at path %q: %w", dirPath, err)
	}

	currentEntries := make(map[string]Hash)
	for _, entry := range targetTree.Entries {
		currentEntries[entry.Name] = entry.ID
	}

	blame := make(map[string]*BlameEntry)
	type queueItem struct {
		commit *Commit
		depth  int
	}

	queue := []queueItem{{
		commit: targetCommit,
		depth:  0,
	}}
	visited := map[Hash]bool{
		commitHash: true,
	}

	for len(queue) > 0 && len(blame) < len(currentEntries) {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= maxBlameDepth {
			continue
		}

		matchedByAnyParent := make(map[string]bool, len(currentEntries))
		hasParents := false

		for _, parentHash := range item.commit.Parents {
			if visited[parentHash] {
				continue
			}
			visited[parentHash] = true

			parentCommit, ok := commits[parentHash]
			if !ok {
				continue
			}

			hasParents = true
			queue = append(queue, queueItem{
				commit: parentCommit,
				depth:  item.depth + 1,
			})

			parentTree, err := r.resolveTreeAtPath(parentCommit.Tree, dirPath)
			if err != nil {
				// Parent didn't have this path, meaning all entries are new relative to it.
				// But other parents might still match, so we continue.
				continue
			}

			parentEntries := make(map[string]Hash, len(parentTree.Entries))
			for _, entry := range parentTree.Entries {
				parentEntries[entry.Name] = entry.ID
			}

			for name, currentHash := range currentEntries {
				if _, alreadyBlamed := blame[name]; alreadyBlamed {
					continue
				}
				if parentHash, existsInParent := parentEntries[name]; existsInParent && parentHash == currentHash {
					matchedByAnyParent[name] = true
				}
			}
		}

		for name := range currentEntries {
			if _, alreadyBlamed := blame[name]; alreadyBlamed {
				continue
			}
			if !hasParents || !matchedByAnyParent[name] {
				blame[name] = newBlameEntry(item.commit)
			}
		}
	}

	blameUnresolved(blame, currentEntries, targetCommit)

	return blame, nil
}

func blameUnresolved(blame map[string]*BlameEntry, entries map[string]Hash, commit *Commit) {
	for name := range entries {
		if _, ok := blame[name]; !ok {
			blame[name] = newBlameEntry(commit)
		}
	}
}

func newBlameEntry(c *Commit) *BlameEntry {
	return &BlameEntry{
		CommitHash:    c.ID,
		CommitMessage: firstLine(c.Message),
		AuthorName:    c.Author.Name,
		When:          c.Author.When,
	}
}

func firstLine(message string) string {
	for i, c := range message {
		if c == '\n' {
			return message[:i]
		}
	}
	return message
}
