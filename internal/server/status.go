package server

import (
	"os/exec"
	"strings"
)

// FileStatus represents the status of a single file in the working tree.
type FileStatus struct {
	Path       string `json:"path"`
	StatusCode string `json:"statusCode"`
}

// WorkingTreeStatus groups files by their working tree state.
type WorkingTreeStatus struct {
	Staged    []FileStatus `json:"staged"`
	Modified  []FileStatus `json:"modified"`
	Untracked []FileStatus `json:"untracked"`
}

// getWorkingTreeStatus runs git status --porcelain in the given working directory
// and returns categorized file statuses. Returns nil if git is unavailable or fails.
func getWorkingTreeStatus(workDir string) *WorkingTreeStatus {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parsePorcelainStatus(string(out))
}

// parsePorcelainStatus parses the output of git status --porcelain into categorized statuses.
// Porcelain format: XY PATH (or XY ORIG -> PATH for renames)
// X = index status, Y = worktree status
func parsePorcelainStatus(output string) *WorkingTreeStatus {
	status := &WorkingTreeStatus{
		Staged:    []FileStatus{},
		Modified:  []FileStatus{},
		Untracked: []FileStatus{},
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		x := line[0] // index status
		y := line[1] // worktree status
		path := line[3:]

		// Handle renames: "R  old -> new"
		if x == 'R' || y == 'R' {
			if idx := strings.Index(path, " -> "); idx >= 0 {
				path = path[idx+4:]
			}
		}

		// Untracked files
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
