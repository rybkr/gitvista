package gitcore

import (
	"bufio"
	"log"
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

// loadIgnoreMatcher creates an ignoreMatcher pre-loaded with the root
// .gitignore and .git/info/exclude (if present).
func loadIgnoreMatcher(workDir, gitDir string) *ignoreMatcher {
	m := &ignoreMatcher{}
	m.loadExcludeFile(filepath.Join(gitDir, "info", "exclude"))
	m.loadFile(workDir, "")
	return m
}

// loadFile reads a .gitignore at workDir/baseDir/.gitignore and appends its
// patterns. baseDir should be "" for the repository root or a slash-terminated
// relative directory path (e.g. "src/").
func (m *ignoreMatcher) loadFile(workDir, baseDir string) {
	path := filepath.Join(workDir, filepath.FromSlash(baseDir), ".gitignore")
	m.loadExcludeFileWithBase(path, baseDir)
}

// loadExcludeFile reads an exclude-format file (like .git/info/exclude) and
// appends its patterns with an empty base directory.
func (m *ignoreMatcher) loadExcludeFile(path string) {
	m.loadExcludeFileWithBase(path, "")
}

// loadExcludeFileWithBase reads a gitignore-format file and appends its
// patterns scoped to baseDir.
func (m *ignoreMatcher) loadExcludeFileWithBase(path, baseDir string) {
	f, err := os.Open(path) //nolint:gosec // path is relative to the repository
	if err != nil {
		return // file is optional
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("failed to close file: %v", err)
		}
	}()

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

	// A pattern containing '/' (after stripping leading slash) is anchored,
	// UNLESS the only slash is part of a leading "**/" prefix. Git treats
	// "**/foo" the same as the non-anchored pattern "foo".
	if strings.Contains(line, "/") {
		remainder := strings.TrimPrefix(line, "**/")
		// If there's still a '/' in the remainder, the pattern is anchored.
		if strings.Contains(remainder, "/") {
			pat.anchored = true
		} else if !strings.HasPrefix(line, "**/") {
			// Regular slash (not **/) → anchored.
			pat.anchored = true
		}
		// "**/foo" with no further slashes → NOT anchored.
	}

	pat.pattern = line
	return pat, line != ""
}

// matchPattern checks whether a relative path matches a single rule.
func matchPattern(rule ignoreRule, relPath string, _ bool) bool {
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
		return matchGlob(pat.pattern, target)
	}

	// Non-anchored patterns can match against the basename alone, or against
	// any suffix segment of the path.
	// First try matching just the basename.
	base := target
	if idx := strings.LastIndex(target, "/"); idx >= 0 {
		base = target[idx+1:]
	}
	if matchGlob(pat.pattern, base) {
		return true
	}

	// Try matching the full relative target.
	if matchGlob(pat.pattern, target) {
		return true
	}

	return false
}

// matchGlob matches a gitignore-style glob pattern against a path. Unlike
// filepath.Match, it handles "**" to match zero or more path components:
//   - A leading "**/" matches in all directories.
//   - A trailing "/**" matches everything inside.
//   - "/**/" in the middle matches zero or more directories.
func matchGlob(pattern, name string) bool {
	// Fast path: no ** in pattern — delegate to filepath.Match.
	if !strings.Contains(pattern, "**") {
		matched, _ := filepath.Match(pattern, name)
		return matched
	}

	patParts := strings.Split(pattern, "/")
	nameParts := strings.Split(name, "/")
	return matchSegments(patParts, nameParts)
}

// matchSegments recursively matches pattern segments against path segments,
// handling "**" as a wildcard for zero or more path components.
func matchSegments(patParts, nameParts []string) bool {
	pi, ni := 0, 0
	for pi < len(patParts) && ni < len(nameParts) {
		if patParts[pi] == "**" {
			pi++
			if pi >= len(patParts) {
				return true // trailing ** matches everything remaining
			}
			// Try matching the rest of the pattern against every suffix
			// of the remaining name segments.
			for tryNi := ni; tryNi <= len(nameParts); tryNi++ {
				if matchSegments(patParts[pi:], nameParts[tryNi:]) {
					return true
				}
			}
			return false
		}
		matched, _ := filepath.Match(patParts[pi], nameParts[ni])
		if !matched {
			return false
		}
		pi++
		ni++
	}
	// Consume any trailing ** segments in the pattern.
	for pi < len(patParts) {
		if patParts[pi] != "**" {
			return false
		}
		pi++
	}
	return ni >= len(nameParts)
}
