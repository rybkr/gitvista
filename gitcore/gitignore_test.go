package gitcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func makeRule(baseDir, pattern string, negated, dirOnly, anchored bool) ignoreRule {
	return ignoreRule{
		baseDir: baseDir,
		pat: ignorePattern{
			pattern:  pattern,
			negated:  negated,
			dirOnly:  dirOnly,
			anchored: anchored,
		},
	}
}

func TestParseIgnoreLineAndMatchPattern(t *testing.T) {
	pat, ok := parseIgnoreLine("!build/")
	if !ok {
		t.Fatal("parseIgnoreLine() = false, want true")
	}
	if !pat.negated || !pat.dirOnly || pat.pattern != "build" {
		t.Fatalf("parseIgnoreLine() = %+v, want negated dirOnly build", pat)
	}

	rule := makeRule("", "*.log", false, false, false)
	if !matchPattern(rule, "app.log", false) {
		t.Fatal("matchPattern() = false, want true")
	}
	if matchPattern(rule, "app.txt", false) {
		t.Fatal("matchPattern() = true, want false")
	}

	anchored := makeRule("src/", "internal/*.go", false, false, true)
	if !matchPattern(anchored, "src/internal/main.go", false) {
		t.Fatal("anchored match should succeed")
	}
	if matchPattern(anchored, "other/internal/main.go", false) {
		t.Fatal("anchored baseDir match should fail")
	}
}

func TestIgnoreMatcherAndLoadIgnoreMatcher(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git/info): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".gitignore"), []byte("*.tmp\n!important.tmp\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte("vendor/\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(exclude): %v", err)
	}

	m := loadIgnoreMatcher(workDir, gitDir)
	if !m.isIgnored("debug.tmp", false) {
		t.Fatal("debug.tmp should be ignored")
	}
	if m.isIgnored("important.tmp", false) {
		t.Fatal("important.tmp should be re-included by negation")
	}
	if !m.isIgnored("vendor", true) {
		t.Fatal("vendor directory should be ignored")
	}
}

func TestMatchGlobDoubleStar(t *testing.T) {
	if !matchGlob("**/node_modules", "frontend/node_modules") {
		t.Fatal("expected ** glob to match nested directory")
	}
	if !matchGlob("src/**/test.go", "src/a/b/test.go") {
		t.Fatal("expected middle ** glob to match")
	}
	if matchGlob("src/**/test.go", "pkg/a/test.go") {
		t.Fatal("unexpected ** glob match")
	}
}

func TestParseIgnoreLine_LiteralEscapes(t *testing.T) {
	hashPattern, ok := parseIgnoreLine(`\#notes.txt`)
	if !ok {
		t.Fatal("parseIgnoreLine(escaped #) = false, want true")
	}
	if hashPattern.negated {
		t.Fatalf("escaped # pattern parsed as negation: %+v", hashPattern)
	}
	if !matchGlob(hashPattern.pattern, "#notes.txt") {
		t.Fatalf("escaped # pattern %q should match literal filename", hashPattern.pattern)
	}

	bangPattern, ok := parseIgnoreLine(`\!important.txt`)
	if !ok {
		t.Fatal("parseIgnoreLine(escaped !) = false, want true")
	}
	if bangPattern.negated {
		t.Fatalf("escaped ! pattern parsed as negation: %+v", bangPattern)
	}
	if !matchGlob(bangPattern.pattern, "!important.txt") {
		t.Fatalf("escaped ! pattern %q should match literal filename", bangPattern.pattern)
	}
}

func TestParseIgnoreLine_PreservesEscapedTrailingSpace(t *testing.T) {
	pat, ok := parseIgnoreLine("space\\ ")
	if !ok {
		t.Fatal("parseIgnoreLine(escaped trailing space) = false, want true")
	}
	if !matchGlob(pat.pattern, "space ") {
		t.Fatalf("pattern %q should match filename with trailing space", pat.pattern)
	}
	if matchGlob(pat.pattern, "space") {
		t.Fatalf("pattern %q should not match filename without trailing space", pat.pattern)
	}
}

func TestIgnoreMatcherMatchesGitCheckIgnore_LocalPatterns(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	mustRunGit(t, workDir, "init", "-q")

	if err := os.WriteFile(filepath.Join(workDir, ".gitignore"), []byte(stringsJoinLines(
		`*.tmp`,
		`\#notes.txt`,
		`\!important.txt`,
		`space\ `,
		`trim `,
	)), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}

	for _, path := range []string{"debug.tmp", "#notes.txt", "!important.txt", "space ", "trim"} {
		if err := os.WriteFile(filepath.Join(workDir, path), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}

	m := loadIgnoreMatcher(workDir, gitDir)
	tests := []struct {
		path  string
		isDir bool
	}{
		{path: "debug.tmp"},
		{path: "#notes.txt"},
		{path: "!important.txt"},
		{path: "space "},
		{path: "trim"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := m.isIgnored(tt.path, tt.isDir)
			want := gitCheckIgnored(t, workDir, tt.path)
			if got != want {
				t.Fatalf("isIgnored(%q) = %v, want %v", tt.path, got, want)
			}
		})
	}
}

func TestLoadIgnoreMatcher_MatchesGitCoreExcludesFile(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	mustRunGit(t, workDir, "init", "-q")

	globalExcludes := filepath.Join(workDir, "global-ignore")
	if err := os.WriteFile(globalExcludes, []byte("global-only.txt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global-ignore): %v", err)
	}
	mustRunGit(t, workDir, "config", "core.excludesFile", globalExcludes)

	if err := os.WriteFile(filepath.Join(workDir, "global-only.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(global-only.txt): %v", err)
	}

	m := loadIgnoreMatcher(workDir, gitDir)
	got := m.isIgnored("global-only.txt", false)
	want := gitCheckIgnored(t, workDir, "global-only.txt")
	if got != want {
		t.Fatalf("isIgnored(global-only.txt) = %v, want %v", got, want)
	}
}

func TestIgnoreMatcherMatchesGitCheckIgnore_NestedPatterns(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	mustRunGit(t, workDir, "init", "-q")

	if err := os.MkdirAll(filepath.Join(workDir, "src", "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll(src/nested): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "vendor", "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(vendor/lib): %v", err)
	}

	if err := os.WriteFile(filepath.Join(workDir, ".gitignore"), []byte(stringsJoinLines(
		`/root-only.txt`,
		`vendor/`,
		`**/cache`,
		`src/*.log`,
	)), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "src", ".gitignore"), []byte(stringsJoinLines(
		`!keep.log`,
		`nested-only.txt`,
	)), 0o644); err != nil {
		t.Fatalf("WriteFile(src/.gitignore): %v", err)
	}

	files := []string{
		"root-only.txt",
		"src/root-only.txt",
		"src/debug.log",
		"src/keep.log",
		"src/nested/nested-only.txt",
		"vendor/lib/code.go",
		"tmp/cache",
	}
	for _, path := range files {
		full := filepath.Join(workDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", path, err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}

	m := loadIgnoreMatcher(workDir, gitDir)
	tests := []struct {
		path string
	}{
		{path: "root-only.txt"},
		{path: "src/root-only.txt"},
		{path: "src/debug.log"},
		{path: "src/keep.log"},
		{path: "src/nested/nested-only.txt"},
		{path: "vendor/lib/code.go"},
		{path: "tmp/cache"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := m.isIgnored(tt.path, false)
			want := gitCheckIgnored(t, workDir, tt.path)
			if got != want {
				t.Fatalf("isIgnored(%q) = %v, want %v", tt.path, got, want)
			}
		})
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func gitCheckIgnored(t *testing.T, dir, path string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "--quiet", "--no-index", "--", path)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return true
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false
	}
	t.Fatalf("git check-ignore %q failed: %v", path, err)
	return false
}

func stringsJoinLines(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for _, line := range lines[1:] {
		result += "\n" + line
	}
	return result + "\n"
}
