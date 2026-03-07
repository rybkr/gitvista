package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// findGitDirectory walks up from startPath to locate the .git directory.
func findGitDirectory(startPath string) (gitDir string, workDir string, err error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if filepath.Base(absPath) == ".git" {
		info, err := os.Stat(absPath)
		if err == nil && info.IsDir() {
			return absPath, filepath.Dir(absPath), nil
		}
	}

	if isBareRepository(absPath) {
		return absPath, absPath, nil
	}

	currentPath := absPath
	for {
		gitPath := filepath.Join(currentPath, ".git")

		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return gitPath, currentPath, nil
			}
			return handleGitFile(gitPath, currentPath)
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return "", "", fmt.Errorf("not a git repository (or any parent up to mount point): %s", startPath)
		}
		currentPath = parentPath
	}
}

// handleGitFile handles .git files (worktrees, submodules) with format "gitdir: <path>".
func handleGitFile(gitFilePath string, workDir string) (string, string, error) {
	//nolint:gosec // G304: .git file path is controlled by repository location
	content, err := os.ReadFile(gitFilePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read .git file: %w", err)
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", "", fmt.Errorf("invalid .git file format: %s", gitFilePath)
	}

	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(gitFilePath), gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	if _, err := os.Stat(gitDir); err != nil {
		return "", "", fmt.Errorf("gitdir points to non-existent directory: %s", gitDir)
	}

	return gitDir, workDir, nil
}

// validateGitDirectory checks that gitDir exists, is a directory, and contains
// the expected Git internals (objects, refs, HEAD).
func validateGitDirectory(gitDir string) error {
	info, err := os.Stat(gitDir)
	if err != nil {
		return fmt.Errorf("git directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("git path is not a directory: %s", gitDir)
	}

	requiredPaths := []string{"objects", "refs", "HEAD"}
	for _, required := range requiredPaths {
		path := filepath.Join(gitDir, required)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("invalid git repository, missing: %s", required)
		}
	}

	return nil
}

// isBareRepository checks whether path looks like a bare Git repository.
// A bare repo is a directory containing objects/, refs/, and HEAD but no .git subdirectory.
func isBareRepository(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return false
	}
	for _, required := range []string{"objects", "refs", "HEAD"} {
		if _, err := os.Stat(filepath.Join(path, required)); err != nil {
			return false
		}
	}
	return true
}

// parseRemotesFromConfig parses a Git config file and returns a map of remote
// names to their URLs, with credentials stripped.
func parseRemotesFromConfig(config string) map[string]string {
	remotes := make(map[string]string)
	var currentRemote string

	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "[remote \"") && strings.HasSuffix(line, "\"]") {
			start := strings.Index(line, "\"") + 1
			end := strings.LastIndex(line, "\"")
			if start > 0 && end > start {
				currentRemote = line[start:end]
			}
			continue
		}

		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[remote") {
			currentRemote = ""
			continue
		}

		if currentRemote != "" && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			remotes[currentRemote] = stripCredentials(url)
			currentRemote = ""
		}
	}

	return remotes
}

// stripCredentials removes embedded credentials from HTTP/HTTPS URLs,
// returning the URL with only the host and path portions intact.
func stripCredentials(url string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, scheme) && strings.Contains(url, "@") {
			parts := strings.SplitN(url, "@", 2)
			if len(parts) == 2 {
				return scheme + parts[1]
			}
		}
	}
	return url
}
