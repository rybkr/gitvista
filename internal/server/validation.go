package server

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validatePath ensures a path is safe for use within a git repository.
// It prevents directory traversal attacks by rejecting paths that:
// - Contain ".." components
// - Start with "/" (absolute paths)
// - Contain null bytes
// - Would escape the repository root when cleaned
func validatePath(path string) error {
	// Reject empty paths (root is represented as "")
	// This is actually valid, so we allow it
	if path == "" {
		return nil
	}

	// Reject paths with null bytes (potential for binary exploits)
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}

	// Reject absolute paths (Unix and Windows style)
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed")
	}
	// Also check for Windows absolute paths (C:\, D:\, etc.)
	if len(path) >= 2 && path[1] == ':' {
		return fmt.Errorf("absolute paths not allowed")
	}

	// Check for ".." components before and after cleaning
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains '..' component")
	}

	// Clean the path and verify it doesn't try to escape
	cleaned := filepath.Clean(path)

	// After cleaning, the path should not start with ".."
	if strings.HasPrefix(cleaned, "..") {
		return fmt.Errorf("path attempts directory traversal")
	}

	// After cleaning, the path should not be absolute
	if filepath.IsAbs(cleaned) {
		return fmt.Errorf("path resolves to absolute path")
	}

	return nil
}

// sanitizePath cleans a path and returns it in a safe form.
// Returns an error if the path is unsafe.
func sanitizePath(path string) (string, error) {
	if err := validatePath(path); err != nil {
		return "", err
	}

	// Handle empty path (root)
	if path == "" {
		return "", nil
	}

	// Normalize backslashes to forward slashes first (Git uses forward slashes)
	normalized := strings.ReplaceAll(path, "\\", "/")

	// Clean the path
	cleaned := filepath.Clean(normalized)

	// Convert back to forward slashes (filepath.Clean may use OS separators)
	cleaned = filepath.ToSlash(cleaned)

	// Remove leading "./" if present
	cleaned = strings.TrimPrefix(cleaned, "./")

	// If it cleaned to ".", that's the root
	if cleaned == "." {
		return "", nil
	}

	return cleaned, nil
}
