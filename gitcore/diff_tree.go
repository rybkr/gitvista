package gitcore

import (
	"fmt"
)

// TreeDiff recursively compares two trees and returns a flat list of changed files.
func TreeDiff(repo *Repository, oldTreeHash, newTreeHash Hash, prefix string) ([]DiffEntry, error) {
	entries, err := treeDiffRecursive(repo, oldTreeHash, newTreeHash, prefix)
	if err != nil {
		return nil, err
	}
	return detectRenames(entries), nil
}

func treeDiffRecursive(repo *Repository, oldTreeHash, newTreeHash Hash, prefix string) ([]DiffEntry, error) {
	entries := make([]DiffEntry, 0)

	var oldTree *Tree
	if oldTreeHash != "" {
		var err error
		oldTree, err = repo.GetTree(oldTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get old tree %s: %w", oldTreeHash, err)
		}
	}

	var newTree *Tree
	if newTreeHash != "" {
		var err error
		newTree, err = repo.GetTree(newTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get new tree %s: %w", newTreeHash, err)
		}
	}

	oldEntries := make(map[string]TreeEntry)
	if oldTree != nil {
		for _, entry := range oldTree.Entries {
			oldEntries[entry.Name] = entry
		}
	}

	newEntries := make(map[string]TreeEntry)
	if newTree != nil {
		for _, entry := range newTree.Entries {
			newEntries[entry.Name] = entry
		}
	}

	allNames := make(map[string]bool)
	for name := range oldEntries {
		allNames[name] = true
	}
	for name := range newEntries {
		allNames[name] = true
	}

	for name := range allNames {
		oldEntry, existsInOld := oldEntries[name]
		newEntry, existsInNew := newEntries[name]

		path := name
		if prefix != "" {
			path = prefix + "/" + name
		}

		if len(entries) >= maxDiffEntries {
			return nil, fmt.Errorf("diff too large: exceeded maximum of %d entries", maxDiffEntries)
		}

		switch {
		case !existsInOld && existsInNew:
			if isTreeEntry(newEntry) {
				subEntries, err := treeDiffRecursive(repo, "", newEntry.ID, path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, subEntries...)
			} else {
				entries = append(entries, DiffEntry{
					Path:     path,
					Status:   DiffStatusAdded,
					NewHash:  newEntry.ID,
					IsBinary: isSubmodule(newEntry),
					NewMode:  newEntry.Mode,
				})
			}
		case existsInOld && !existsInNew:
			if isTreeEntry(oldEntry) {
				subEntries, err := treeDiffRecursive(repo, oldEntry.ID, "", path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, subEntries...)
			} else {
				entries = append(entries, DiffEntry{
					Path:     path,
					Status:   DiffStatusDeleted,
					OldHash:  oldEntry.ID,
					IsBinary: isSubmodule(oldEntry),
					OldMode:  oldEntry.Mode,
				})
			}
		case existsInOld && existsInNew:
			if oldEntry.ID != newEntry.ID {
				if isTreeEntry(oldEntry) && isTreeEntry(newEntry) {
					subEntries, err := treeDiffRecursive(repo, oldEntry.ID, newEntry.ID, path)
					if err != nil {
						return nil, err
					}
					entries = append(entries, subEntries...)
				} else if isTreeEntry(oldEntry) || isTreeEntry(newEntry) {
					if isTreeEntry(oldEntry) {
						subEntries, err := treeDiffRecursive(repo, oldEntry.ID, "", path)
						if err != nil {
							return nil, err
						}
						entries = append(entries, subEntries...)
					} else {
						entries = append(entries, DiffEntry{
							Path:     path,
							Status:   DiffStatusDeleted,
							OldHash:  oldEntry.ID,
							IsBinary: isSubmodule(oldEntry),
							OldMode:  oldEntry.Mode,
						})
					}
					if isTreeEntry(newEntry) {
						subEntries, err := treeDiffRecursive(repo, "", newEntry.ID, path)
						if err != nil {
							return nil, err
						}
						entries = append(entries, subEntries...)
					} else {
						entries = append(entries, DiffEntry{
							Path:     path,
							Status:   DiffStatusAdded,
							NewHash:  newEntry.ID,
							IsBinary: isSubmodule(newEntry),
							NewMode:  newEntry.Mode,
						})
					}
				} else {
					entries = append(entries, DiffEntry{
						Path:     path,
						Status:   DiffStatusModified,
						OldHash:  oldEntry.ID,
						NewHash:  newEntry.ID,
						IsBinary: isSubmodule(oldEntry) || isSubmodule(newEntry),
						OldMode:  oldEntry.Mode,
						NewMode:  newEntry.Mode,
					})
				}
			}
		}
	}

	return entries, nil
}

func detectRenames(entries []DiffEntry) []DiffEntry {
	type deletedInfo struct {
		index int
		path  string
		mode  string
	}

	deletedByHash := make(map[Hash][]deletedInfo)
	for i, entry := range entries {
		if entry.Status == DiffStatusDeleted && entry.OldHash != "" {
			deletedByHash[entry.OldHash] = append(deletedByHash[entry.OldHash], deletedInfo{
				index: i,
				path:  entry.Path,
				mode:  entry.OldMode,
			})
		}
	}

	if len(deletedByHash) == 0 {
		return entries
	}

	consumed := make(map[Hash]int)
	matched := make(map[int]bool)

	for i := range entries {
		if entries[i].Status != DiffStatusAdded || entries[i].NewHash == "" {
			continue
		}
		candidates := deletedByHash[entries[i].NewHash]
		idx := consumed[entries[i].NewHash]
		if idx >= len(candidates) {
			continue
		}
		info := candidates[idx]
		consumed[entries[i].NewHash] = idx + 1

		entries[i].Status = DiffStatusRenamed
		entries[i].OldPath = info.path
		entries[i].OldHash = entries[i].NewHash
		entries[i].OldMode = info.mode
		matched[info.index] = true
	}

	if len(matched) == 0 {
		return entries
	}

	result := make([]DiffEntry, 0, len(entries)-len(matched))
	for i, entry := range entries {
		if !matched[i] {
			result = append(result, entry)
		}
	}
	return result
}

func isTreeEntry(entry TreeEntry) bool {
	return entry.Type == ObjectTypeTree || entry.Mode == "040000" || entry.Mode == "40000"
}

func isSubmodule(entry TreeEntry) bool {
	return entry.Mode == "160000"
}
