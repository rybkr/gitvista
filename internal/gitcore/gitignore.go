package gitcore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ignorePattern represents a single parsed .gitignore pattern.
type ignorePattern struct {
	pattern  string // the glob pattern (cleaned)
	negated  bool   // true if the original line starts with '!'
	dirOnly  bool   // true if the original pattern ends with '/'
	anchored bool   // true if the pattern contains a '/' (matches relative to .gitignore location)
}

// ignoreMatcher checks whether a given relative path should be ignored.
// It aggregates patterns from multiple .gitignore files loaded during a walk.
type ignoreMatcher struct {
	// rules are stored in order; later rules override earlier ones.
	rules []ignoreRule
}

// ignoreRule is a pattern associated with its base directory (relative to the
// repository root) so that anchored patterns are matched correctly.
type ignoreRule struct {
	baseDir string // "" for root .gitignore, or e.g. "src/" for src/.gitignore
	pat     ignorePattern
}

// loadIgnoreMatcher creates an ignoreMatcher pre-loaded with the root .gitignore.
func loadIgnoreMatcher(workDir string) *ignoreMatcher {
	m := &ignoreMatcher{}
	m.loadFile(workDir, "")
	return m
}

// loadFile reads a .gitignore at workDir/baseDir/.gitignore and appends its
// patterns. baseDir should be "" for the repository root or a slash-terminated
// relative directory path (e.g. "src/").
func (m *ignoreMatcher) loadFile(workDir, baseDir string) {
	path := filepath.Join(workDir, filepath.FromSlash(baseDir), ".gitignore")
	f, err := os.Open(path) //nolint:gosec // path is relative to the repository
	if err != nil {
		return // .gitignore is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		pat, ok := parseIgnoreLine(line)
		if !ok {
			continue
		}
		m.rules = append(m.rules, ignoreRule{baseDir: baseDir, pat: pat})
	}
}

// isIgnored returns true if the relative path (forward-slash separated) should
// be ignored. isDir indicates whether the path is a directory.
func (m *ignoreMatcher) isIgnored(relPath string, isDir bool) bool {
	ignored := false
	for _, rule := range m.rules {
		if rule.pat.dirOnly && !isDir {
			continue
		}
		if matchPattern(rule, relPath, isDir) {
			ignored = !rule.pat.negated
		}
	}
	return ignored
}

// parseIgnoreLine parses a single line from a .gitignore file.
// Returns the pattern and true if the line is a valid pattern, or false if
// the line is blank or a comment.
func parseIgnoreLine(line string) (ignorePattern, bool) {
	// Strip trailing whitespace (unless escaped with backslash).
	line = strings.TrimRight(line, " \t")

	// Skip blank lines and comments.
	if line == "" || line[0] == '#' {
		return ignorePattern{}, false
	}

	var pat ignorePattern

	// Handle negation.
	if line[0] == '!' {
		pat.negated = true
		line = line[1:]
	}

	// A trailing '/' means directory-only match.
	if strings.HasSuffix(line, "/") {
		pat.dirOnly = true
		line = strings.TrimRight(line, "/")
	}

	// Strip leading '/' — it only means "anchored to base dir".
	if strings.HasPrefix(line, "/") {
		pat.anchored = true
		line = line[1:]
	}

	// A pattern containing '/' (after stripping leading slash) is anchored.
	if strings.Contains(line, "/") {
		pat.anchored = true
	}

	pat.pattern = line
	return pat, line != ""
}

// matchPattern checks whether a relative path matches a single rule.
func matchPattern(rule ignoreRule, relPath string, isDir bool) bool {
	pat := rule.pat

	// For rules loaded from subdirectory .gitignore files, strip the base
	// directory prefix so the pattern matches relative to its own location.
	target := relPath
	if rule.baseDir != "" {
		if !strings.HasPrefix(relPath, rule.baseDir) {
			return false
		}
		target = relPath[len(rule.baseDir):]
	}

	if pat.anchored {
		// Anchored patterns must match the full remaining path.
		matched, _ := filepath.Match(pat.pattern, target)
		return matched
	}

	// Non-anchored patterns can match against the basename alone, or against
	// any suffix segment of the path.
	// First try matching just the basename.
	base := target
	if idx := strings.LastIndex(target, "/"); idx >= 0 {
		base = target[idx+1:]
	}
	if matched, _ := filepath.Match(pat.pattern, base); matched {
		return true
	}

	// Try matching the full relative target.
	if matched, _ := filepath.Match(pat.pattern, target); matched {
		return true
	}

	return false
}
