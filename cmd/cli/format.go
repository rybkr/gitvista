package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// gitDateFormat formats a time.Time the same way git log does.
// Layout: "Mon Jan 2 15:04:05 2006 -0700".
func gitDateFormat(t time.Time) string {
	return t.Format("Mon Jan 2 15:04:05 2006 -0700")
}

// resolveHash resolves a revision string to a full hash.
// Supports: full 40-char hash, short prefix (>=4 chars), HEAD, branch names, tag names.
func resolveHash(repo *gitcore.Repository, rev string) (gitcore.Hash, error) {
	if rev == "HEAD" {
		h := repo.Head()
		if h == "" {
			return "", fmt.Errorf("HEAD is not set")
		}
		return h, nil
	}

	// Try as full hash
	if len(rev) == 40 {
		if _, err := gitcore.NewHash(rev); err == nil {
			return gitcore.Hash(rev), nil
		}
	}

	// Try as branch name
	branches := repo.Branches()
	if hash, ok := branches[rev]; ok {
		return hash, nil
	}

	// Try as tag name (peeled to commit)
	tags := repo.Tags()
	if hashStr, ok := tags[rev]; ok {
		return gitcore.Hash(hashStr), nil
	}

	// Try as short hash prefix
	if len(rev) >= 4 && len(rev) < 40 {
		commits := repo.Commits()
		var match gitcore.Hash
		count := 0
		for hash := range commits {
			if strings.HasPrefix(string(hash), rev) {
				match = hash
				count++
				if count > 1 {
					return "", fmt.Errorf("short hash %q is ambiguous", rev)
				}
			}
		}
		if count == 1 {
			return match, nil
		}
	}

	return "", fmt.Errorf("unknown revision: %s", rev)
}
