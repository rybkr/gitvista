package gitcore

import (
	"fmt"
)

var loadObjectForTraversal = func(r *Repository, id Hash) (Object, error) {
	return r.readObject(id)
}

func (r *Repository) loadObjects() error {
	visited := make(map[Hash]bool)
	stack := make([]Hash, 0, len(r.refs)+len(r.stashes)+1)
	if r.head != "" {
		stack = append(stack, r.head)
	}
	for _, ref := range r.refs {
		stack = append(stack, ref)
	}
	for _, stash := range r.stashes {
		stack = append(stack, stash.Hash)
	}

	// We use an iterative stack to avoid stack overflow on repositories with a deep
	// linear history (100K+ commits).
	for len(stack) > 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[ref] {
			continue
		}
		visited[ref] = true

		object, err := loadObjectForTraversal(r, ref)
		if err != nil {
			return fmt.Errorf("error traversing object: %w", err)
		}

		switch object.Type() {
		case ObjectTypeCommit:
			commit, ok := object.(*Commit)
			if !ok {
				return fmt.Errorf("unexpected type for commit object %s", ref)
			}
			r.commits = append(r.commits, commit)
			stack = append(stack, commit.Parents...)
		case ObjectTypeTag:
			tag, ok := object.(*Tag)
			if !ok {
				return fmt.Errorf("unexpected type for tag object %s", ref)
			}
			r.tags = append(r.tags, tag)
			stack = append(stack, tag.Object)
		case ObjectTypeTree, ObjectTypeBlob:
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
