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
	rules       []ignoreRule
	loadedBases map[string]struct{}
}

type ignoreRule struct {
	baseDir string
	pat     ignorePattern
}

func loadIgnoreMatcher(workDir, gitDir string) *ignoreMatcher {
	m := &ignoreMatcher{
		loadedBases: make(map[string]struct{}),
	}
	m.loadConfiguredGlobalExcludes(gitDir)
	m.loadExcludeFile(filepath.Join(gitDir, "info", "exclude"))
	_ = filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(workDir, path)
		if relErr != nil {
			return relErr
		}
		baseDir := ""
		if relPath != "." {
			baseDir = filepath.ToSlash(relPath) + "/"
		}
		m.loadFile(workDir, baseDir)
		return nil
	})
	return m
}

func (m *ignoreMatcher) loadFile(workDir, baseDir string) {
	if _, loaded := m.loadedBases[baseDir]; loaded {
		return
	}
	m.loadedBases[baseDir] = struct{}{}
	path := filepath.Join(workDir, filepath.FromSlash(baseDir), ".gitignore")
	m.loadExcludeFileWithBase(path, baseDir)
}

func (m *ignoreMatcher) loadExcludeFile(path string) {
	m.loadExcludeFileWithBase(path, "")
}

func (m *ignoreMatcher) loadConfiguredGlobalExcludes(gitDir string) {
	configPath := filepath.Join(gitDir, "config")
	// #nosec G304 -- config path is derived from the repository git dir.
	content, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	path := parseCoreExcludesFileFromConfig(string(content))
	if path == "" {
		return
	}
	if strings.HasPrefix(path, "~/") {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(gitDir, path)
	}
	m.loadExcludeFile(path)
}

func (m *ignoreMatcher) loadExcludeFileWithBase(path, baseDir string) {
	// #nosec G304 -- callers build paths from the repository worktree or gitDir.
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
		if matchPattern(rule, relPath, isDir) {
			ignored = !rule.pat.negated
		}
	}
	return ignored
}

func parseIgnoreLine(line string) (ignorePattern, bool) {
	line = trimTrailingIgnoreWhitespace(line)
	if line == "" || line[0] == '#' {
		return ignorePattern{}, false
	}

	var pat ignorePattern
	if line[0] == '!' {
		pat.negated = true
		line = line[1:]
	} else if strings.HasPrefix(line, `\!`) || strings.HasPrefix(line, `\#`) {
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

func trimTrailingIgnoreWhitespace(line string) string {
	end := len(line)
	for end > 0 {
		last := line[end-1]
		if last != ' ' && last != '\t' {
			break
		}

		backslashes := 0
		for i := end - 2; i >= 0 && line[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			break
		}
		end--
	}
	return line[:end]
}

func parseCoreExcludesFileFromConfig(config string) string {
	inCore := false
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "[core]"):
			inCore = true
			continue
		case strings.HasPrefix(line, "["):
			inCore = false
			continue
		}

		if !inCore {
			continue
		}
		if strings.HasPrefix(line, "excludesFile = ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "excludesFile = "))
		}
	}
	return ""
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

	if pat.dirOnly && matchDirectoryPattern(pat.pattern, target, pat.anchored) {
		return true
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

func matchDirectoryPattern(pattern, target string, anchored bool) bool {
	target = strings.TrimPrefix(target, "/")
	if target == "" {
		return false
	}

	if anchored {
		return target == pattern || strings.HasPrefix(target, pattern+"/")
	}

	parts := strings.Split(target, "/")
	for i := range parts {
		candidate := parts[i]
		if matchGlob(pattern, candidate) {
			return true
		}
	}
	return false
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
