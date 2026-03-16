package gitcore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type ignorePattern struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
}

type ignoreMatcher struct {
	rules []ignoreRule
}

type ignoreRule struct {
	baseDir string
	pat     ignorePattern
}

func loadIgnoreMatcher(workDir, gitDir string) *ignoreMatcher {
	m := &ignoreMatcher{}
	m.loadExcludeFile(filepath.Join(gitDir, "info", "exclude"))
	m.loadFile(workDir, "")
	return m
}

func (m *ignoreMatcher) loadFile(workDir, baseDir string) {
	path := filepath.Join(workDir, filepath.FromSlash(baseDir), ".gitignore")
	m.loadExcludeFileWithBase(path, baseDir)
}

func (m *ignoreMatcher) loadExcludeFile(path string) {
	m.loadExcludeFileWithBase(path, "")
}

func (m *ignoreMatcher) loadExcludeFileWithBase(path, baseDir string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		pat, ok := parseIgnoreLine(scanner.Text())
		if !ok {
			continue
		}
		m.rules = append(m.rules, ignoreRule{baseDir: baseDir, pat: pat})
	}
}

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

func parseIgnoreLine(line string) (ignorePattern, bool) {
	line = strings.TrimRight(line, " \t")
	if line == "" || line[0] == '#' {
		return ignorePattern{}, false
	}

	var pat ignorePattern
	if line[0] == '!' {
		pat.negated = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		pat.dirOnly = true
		line = strings.TrimRight(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		pat.anchored = true
		line = line[1:]
	}
	if strings.Contains(line, "/") {
		remainder := strings.TrimPrefix(line, "**/")
		if strings.Contains(remainder, "/") {
			pat.anchored = true
		} else if !strings.HasPrefix(line, "**/") {
			pat.anchored = true
		}
	}

	pat.pattern = line
	return pat, line != ""
}

func matchPattern(rule ignoreRule, relPath string, _ bool) bool {
	pat := rule.pat
	target := relPath
	if rule.baseDir != "" {
		if !strings.HasPrefix(relPath, rule.baseDir) {
			return false
		}
		target = relPath[len(rule.baseDir):]
	}

	if pat.anchored {
		return matchGlob(pat.pattern, target)
	}

	base := target
	if idx := strings.LastIndex(target, "/"); idx >= 0 {
		base = target[idx+1:]
	}
	if matchGlob(pat.pattern, base) {
		return true
	}

	return matchGlob(pat.pattern, target)
}

func matchGlob(pattern, name string) bool {
	if !strings.Contains(pattern, "**") {
		matched, _ := filepath.Match(pattern, name)
		return matched
	}

	patParts := strings.Split(pattern, "/")
	nameParts := strings.Split(name, "/")
	return matchSegments(patParts, nameParts)
}

func matchSegments(patParts, nameParts []string) bool {
	pi, ni := 0, 0
	for pi < len(patParts) && ni < len(nameParts) {
		if patParts[pi] == "**" {
			pi++
			if pi >= len(patParts) {
				return true
			}
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
	for pi < len(patParts) {
		if patParts[pi] != "**" {
			return false
		}
		pi++
	}
	return ni >= len(nameParts)
}
