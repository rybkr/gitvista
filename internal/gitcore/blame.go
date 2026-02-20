// Package gitcore provides pure Go implementation of Git object parsing and repository traversal.
package gitcore

import (
	"fmt"
	"time"
)

// BlameEntry represents the commit, message, author, and time of a change to be blamed.
type BlameEntry struct {
	CommitHash    Hash      `json:"commitHash"`
	CommitMessage string    `json:"commitMessage"`
	AuthorName    string    `json:"authorName"`
	When          time.Time `json:"when"`
}

// GetFileBlame returns per-entry last-modified info for the immediate children of
// dirPath at the given commit. It walks backward through history via BFS (up to
// 1000 commits) comparing tree entries to find the commit that last changed each one.
// Entries not resolved within the depth limit are attributed to the target commit.
func (r *Repository) GetFileBlame(commitHash Hash, dirPath string) (map[string]*BlameEntry, error) {
	const maxDepth = 1000

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
	queue := []queueItem{{commit: targetCommit, depth: 0}}
	visited := make(map[Hash]bool)
	visited[commitHash] = true

	for len(queue) > 0 && len(blame) < len(currentEntries) {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= maxDepth {
			continue
		}

		for _, parentHash := range item.commit.Parents {
			if visited[parentHash] {
				continue
			}
			visited[parentHash] = true

			parentCommit, ok := commits[parentHash]
			if !ok {
				continue
			}

			parentTree, err := r.resolveTreeAtPath(parentCommit.Tree, dirPath)
			if err != nil {
				// Path doesn't exist in parent -- all unblamed entries were introduced by item.commit
				blameUnresolved(blame, currentEntries, item.commit)
				continue
			}

			parentEntries := make(map[string]Hash)
			for _, entry := range parentTree.Entries {
				parentEntries[entry.Name] = entry.ID
			}

			for name, currentHash := range currentEntries {
				if _, alreadyBlamed := blame[name]; alreadyBlamed {
					continue
				}
				parentHash, existedInParent := parentEntries[name]
				if !existedInParent || parentHash != currentHash {
					blame[name] = newBlameEntry(item.commit)
				}
			}

			queue = append(queue, queueItem{commit: parentCommit, depth: item.depth + 1})
		}

		// Root commit: attribute all remaining unblamed entries
		if len(item.commit.Parents) == 0 {
			blameUnresolved(blame, currentEntries, item.commit)
		}
	}

	// Fallback for entries not resolved within maxDepth
	blameUnresolved(blame, currentEntries, targetCommit)

	return blame, nil
}

// blameUnresolved assigns commit as the blame for any entries not yet in the blame map.
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
