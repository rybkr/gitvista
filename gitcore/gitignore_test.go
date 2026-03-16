package gitcore

import (
	"os"
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
