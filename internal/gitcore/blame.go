// Package gitcore provides pure Go implementation of Git object parsing and repository traversal.
package gitcore

import (
	"fmt"
	"time"
)

// BlameEntry records which commit last modified a file or directory entry.
type BlameEntry struct {
	CommitHash    Hash      `json:"commitHash"`
	CommitMessage string    `json:"commitMessage"`
	AuthorName    string    `json:"authorName"`
	When          time.Time `json:"when"`
}

// GetFileBlame returns per-file last-modified information for the immediate children
// of dirPath at the given commit. It walks backward through commit history up to
// maxDepth commits (default 1000) to determine which commit last modified each entry.
//
// The returned map keys are entry names (filenames or directory names), and values
// are BlameEntry structs with the last-modifying commit's metadata.
func (r *Repository) GetFileBlame(commitHash Hash, dirPath string) (map[string]*BlameEntry, error) {
	const maxDepth = 1000

	// Look up the target commit
	commits := r.Commits()
	targetCommit, ok := commits[commitHash]
	if !ok {
		return nil, fmt.Errorf("commit not found: %s", commitHash)
	}

	// Resolve the tree at the target directory path
	targetTree, err := r.resolveTreeAtPath(targetCommit.Tree, dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tree at path %q: %w", dirPath, err)
	}

	// Build a map of entry name -> current hash for all entries in the target tree
	currentEntries := make(map[string]Hash)
	for _, entry := range targetTree.Entries {
		currentEntries[entry.Name] = entry.ID
	}

	// Result map: entry name -> blame entry
	blame := make(map[string]*BlameEntry)

	// BFS queue for walking commit history
	type queueItem struct {
		commit *Commit
		depth  int
	}
	queue := []queueItem{{commit: targetCommit, depth: 0}}
	visited := make(map[Hash]bool)
	visited[commitHash] = true

	// Walk backward through history
	for len(queue) > 0 && len(blame) < len(currentEntries) {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= maxDepth {
			continue
		}

		// Process each parent
		for _, parentHash := range item.commit.Parents {
			if visited[parentHash] {
				continue
			}
			visited[parentHash] = true

			parentCommit, ok := commits[parentHash]
			if !ok {
				// Parent not loaded, skip
				continue
			}

			// Resolve the same directory path in the parent's tree
			parentTree, err := r.resolveTreeAtPath(parentCommit.Tree, dirPath)
			if err != nil {
				// Path doesn't exist in parent (directory was added in child)
				// All entries in current tree that aren't blamed yet were added by item.commit
				for name := range currentEntries {
					if _, alreadyBlamed := blame[name]; !alreadyBlamed {
						blame[name] = &BlameEntry{
							CommitHash:    item.commit.ID,
							CommitMessage: firstLine(item.commit.Message),
							AuthorName:    item.commit.Author.Name,
							When:          item.commit.Author.When,
						}
					}
				}
				continue
			}

			// Build parent entry map
			parentEntries := make(map[string]Hash)
			for _, entry := range parentTree.Entries {
				parentEntries[entry.Name] = entry.ID
			}

			// Compare current entries with parent entries
			for name, currentHash := range currentEntries {
				if _, alreadyBlamed := blame[name]; alreadyBlamed {
					continue
				}

				parentHash, existedInParent := parentEntries[name]
				if !existedInParent || parentHash != currentHash {
					// Entry was added or modified in item.commit
					blame[name] = &BlameEntry{
						CommitHash:    item.commit.ID,
						CommitMessage: firstLine(item.commit.Message),
						AuthorName:    item.commit.Author.Name,
						When:          item.commit.Author.When,
					}
				}
			}

			// Add parent to queue for further traversal
			queue = append(queue, queueItem{commit: parentCommit, depth: item.depth + 1})
		}

		// If this commit has no parents and some entries are still unblamed,
		// they were present since this initial commit
		if len(item.commit.Parents) == 0 {
			for name := range currentEntries {
				if _, alreadyBlamed := blame[name]; !alreadyBlamed {
					blame[name] = &BlameEntry{
						CommitHash:    item.commit.ID,
						CommitMessage: firstLine(item.commit.Message),
						AuthorName:    item.commit.Author.Name,
						When:          item.commit.Author.When,
					}
				}
			}
		}
	}

	// Entries not resolved within maxDepth are marked with the target commit as fallback
	for name := range currentEntries {
		if _, alreadyBlamed := blame[name]; !alreadyBlamed {
			blame[name] = &BlameEntry{
				CommitHash:    targetCommit.ID,
				CommitMessage: firstLine(targetCommit.Message),
				AuthorName:    targetCommit.Author.Name,
				When:          targetCommit.Author.When,
			}
		}
	}

	return blame, nil
}

// firstLine extracts the first line of a commit message (subject line).
func firstLine(message string) string {
	for i, c := range message {
		if c == '\n' {
			return message[:i]
		}
	}
	return message
}
