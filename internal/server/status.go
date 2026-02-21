package server

import (
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// File status label constants used across the server package.
const (
	fileStatusAdded    = "added"
	fileStatusModified = "modified"
	fileStatusDeleted  = "deleted"
	fileStatusRenamed  = "renamed"
	fileStatusCopied   = "copied"
)

// FileStatus represents the status of a file and its path.
type FileStatus struct {
	Path       string `json:"path"`
	StatusCode string `json:"statusCode"`
}

// WorkingTreeStatus represents the files that are staged, modified, and untracked.
type WorkingTreeStatus struct {
	Staged    []FileStatus `json:"staged"`
	Modified  []FileStatus `json:"modified"`
	Untracked []FileStatus `json:"untracked"`
}

// getWorkingTreeStatus returns the working tree status for the given repository.
// Returns nil if the status cannot be computed (e.g., bare repository).
func getWorkingTreeStatus(repo *gitcore.Repository) *WorkingTreeStatus {
	wts, err := gitcore.ComputeWorkingTreeStatus(repo)
	if err != nil {
		return nil
	}

	return translateWorkingTreeStatus(wts)
}

// translateWorkingTreeStatus converts a gitcore.WorkingTreeStatus into the
// server-layer WorkingTreeStatus, mapping the gitcore string status names
// ("added", "modified", etc.) to the single-letter codes the frontend expects.
func translateWorkingTreeStatus(wts *gitcore.WorkingTreeStatus) *WorkingTreeStatus {
	status := &WorkingTreeStatus{
		Staged:    []FileStatus{},
		Modified:  []FileStatus{},
		Untracked: []FileStatus{},
	}

	for _, f := range wts.Files {
		if f.IsUntracked {
			status.Untracked = append(status.Untracked, FileStatus{
				Path:       f.Path,
				StatusCode: "?",
			})
			continue
		}

		// Map the index (staged) status to its single-letter code.
		if code := indexStatusCode(f.IndexStatus); code != "" {
			status.Staged = append(status.Staged, FileStatus{
				Path:       f.Path,
				StatusCode: code,
			})
		}

		// Map the worktree (unstaged) status to its single-letter code.
		if code := workStatusCode(f.WorkStatus); code != "" {
			status.Modified = append(status.Modified, FileStatus{
				Path:       f.Path,
				StatusCode: code,
			})
		}
	}

	return status
}

// indexStatusCode maps a gitcore IndexStatus string to a single-letter porcelain
// status code. Returns "" for unrecognized or empty values so the caller can skip them.
func indexStatusCode(s string) string {
	switch s {
	case fileStatusAdded:
		return "A"
	case fileStatusModified:
		return "M"
	case fileStatusDeleted:
		return "D"
	case fileStatusRenamed:
		return "R"
	case fileStatusCopied:
		return "C"
	default:
		return ""
	}
}

// workStatusCode maps a gitcore WorkStatus string to a single-letter porcelain
// status code. Returns "" for unrecognized or empty values.
func workStatusCode(s string) string {
	switch s {
	case fileStatusModified:
		return "M"
	case fileStatusDeleted:
		return "D"
	default:
		return ""
	}
}

// parsePorcelainStatus parses "git status --porcelain" output.
// Format: XY PATH where X = index status, Y = worktree status.
// Retained for use in unit tests.
func parsePorcelainStatus(output string) *WorkingTreeStatus {
	status := &WorkingTreeStatus{
		Staged:    []FileStatus{},
		Modified:  []FileStatus{},
		Untracked: []FileStatus{},
	}

	for line := range strings.SplitSeq(output, "\n") {
		if len(line) < 3 {
			continue
		}

		x := line[0]
		y := line[1]
		path := line[3:]

		if x == 'R' || y == 'R' {
			if idx := strings.Index(path, " -> "); idx >= 0 {
				path = path[idx+4:]
			}
		}

		if x == '?' && y == '?' {
			status.Untracked = append(status.Untracked, FileStatus{
				Path:       path,
				StatusCode: "?",
			})
			continue
		}

		// Staged changes (index has modifications)
		if x == 'M' || x == 'A' || x == 'D' || x == 'R' || x == 'C' {
			status.Staged = append(status.Staged, FileStatus{
				Path:       path,
				StatusCode: string(x),
			})
		}

		// Worktree modifications
		if y == 'M' || y == 'D' {
			status.Modified = append(status.Modified, FileStatus{
				Path:       path,
				StatusCode: string(y),
			})
		}
	}

	return status
}
