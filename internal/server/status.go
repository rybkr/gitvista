package server

import (
	"os/exec"
	"strings"
)

type FileStatus struct {
	Path       string `json:"path"`
	StatusCode string `json:"statusCode"`
}

type WorkingTreeStatus struct {
	Staged    []FileStatus `json:"staged"`
	Modified  []FileStatus `json:"modified"`
	Untracked []FileStatus `json:"untracked"`
}

// getWorkingTreeStatus returns nil if git is unavailable or the command fails.
func getWorkingTreeStatus(workDir string) *WorkingTreeStatus {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parsePorcelainStatus(string(out))
}

// parsePorcelainStatus parses "git status --porcelain" output.
// Format: XY PATH where X = index status, Y = worktree status.
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
