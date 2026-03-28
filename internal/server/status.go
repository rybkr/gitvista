package server

import (
	"slices"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
)

// FileStatus represents the status of a file and its path.
type FileStatus struct {
	Path       string `json:"path"`
	StatusCode string `json:"statusCode"`
	BlobHash   string `json:"blobHash,omitempty"`
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
// server-layer WorkingTreeStatus, mapping gitcore change types to the
// single-letter codes the frontend expects.
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

		if code := indexStatusCode(f.StagedChange); code != "" {
			status.Staged = append(status.Staged, FileStatus{
				Path:       f.Path,
				StatusCode: code,
				BlobHash:   string(f.StagedHash),
			})
		}

		if code := workStatusCode(f.UnstagedChange); code != "" {
			status.Modified = append(status.Modified, FileStatus{
				Path:       f.Path,
				StatusCode: code,
				BlobHash:   string(f.WorktreeHash),
			})
		}
	}

	// Sort each category by path for stable ordering across refreshes.
	sortByPath := func(a, b FileStatus) int {
		return strings.Compare(a.Path, b.Path)
	}
	slices.SortFunc(status.Staged, sortByPath)
	slices.SortFunc(status.Modified, sortByPath)
	slices.SortFunc(status.Untracked, sortByPath)

	return status
}

// indexStatusCode maps a staged change type to a single-letter porcelain status code.
func indexStatusCode(change gitcore.ChangeType) string {
	switch change {
	case gitcore.ChangeTypeAdded:
		return "A"
	case gitcore.ChangeTypeModified:
		return "M"
	case gitcore.ChangeTypeDeleted:
		return "D"
	case gitcore.ChangeTypeRenamed:
		return "R"
	case gitcore.ChangeTypeCopied:
		return "C"
	case gitcore.ChangeTypeTypeChanged:
		return "T"
	default:
		return ""
	}
}

// workStatusCode maps an unstaged change type to a single-letter porcelain status code.
func workStatusCode(change gitcore.ChangeType) string {
	switch change {
	case gitcore.ChangeTypeModified:
		return "M"
	case gitcore.ChangeTypeDeleted:
		return "D"
	case gitcore.ChangeTypeTypeChanged:
		return "T"
	default:
		return ""
	}
}
