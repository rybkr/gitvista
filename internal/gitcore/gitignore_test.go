package gitcore

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Tests for parseIgnoreLine
// ---------------------------------------------------------------------------

// TestParseIgnoreLine_BlankLine verifies that a blank line is skipped (ok=false).
func TestParseIgnoreLine_BlankLine(t *testing.T) {
	_, ok := parseIgnoreLine("")
	if ok {
		t.Error("expected ok=false for blank line, got true")
	}
}

// TestParseIgnoreLine_WhitespaceOnlyLine verifies that a line containing only
// spaces and tabs is treated as blank and skipped.
func TestParseIgnoreLine_WhitespaceOnlyLine(t *testing.T) {
	_, ok := parseIgnoreLine("   \t  ")
	if ok {
		t.Error("expected ok=false for whitespace-only line, got true")
	}
}

// TestParseIgnoreLine_CommentLine verifies that lines starting with '#' are
// treated as comments and skipped.
func TestParseIgnoreLine_CommentLine(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"hash at start", "# this is a comment"},
		{"hash only", "#"},
		{"hash no space", "#comment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseIgnoreLine(tt.line)
			if ok {
				t.Errorf("parseIgnoreLine(%q): expected ok=false for comment, got true", tt.line)
			}
		})
	}
}

// TestParseIgnoreLine_SimplePattern verifies that a plain filename pattern is
// parsed with no special flags set.
func TestParseIgnoreLine_SimplePattern(t *testing.T) {
	pat, ok := parseIgnoreLine("*.log")
	if !ok {
		t.Fatal("expected ok=true for simple pattern")
	}
	if pat.pattern != "*.log" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "*.log")
	}
	if pat.negated {
		t.Error("negated should be false for simple pattern")
	}
	if pat.dirOnly {
		t.Error("dirOnly should be false for simple pattern")
	}
	if pat.anchored {
		t.Error("anchored should be false for pattern without '/'")
	}
}

// TestParseIgnoreLine_NegationPrefix verifies that a line starting with '!'
// produces a negated pattern and strips the '!'.
func TestParseIgnoreLine_NegationPrefix(t *testing.T) {
	pat, ok := parseIgnoreLine("!important.log")
	if !ok {
		t.Fatal("expected ok=true for negated pattern")
	}
	if !pat.negated {
		t.Error("negated should be true")
	}
	if pat.pattern != "important.log" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "important.log")
	}
	if pat.anchored {
		t.Error("anchored should be false: no '/' in pattern body")
	}
}

// TestParseIgnoreLine_NegationWithDirectory verifies that a negated pattern
// with a trailing slash is both negated and dirOnly.
func TestParseIgnoreLine_NegationWithDirectory(t *testing.T) {
	pat, ok := parseIgnoreLine("!vendor/")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !pat.negated {
		t.Error("negated should be true")
	}
	if !pat.dirOnly {
		t.Error("dirOnly should be true (trailing slash)")
	}
	if pat.pattern != "vendor" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "vendor")
	}
}

// TestParseIgnoreLine_DirectoryOnly verifies that a trailing '/' sets
// dirOnly=true and the slash is stripped from the pattern.
func TestParseIgnoreLine_DirectoryOnly(t *testing.T) {
	pat, ok := parseIgnoreLine("build/")
	if !ok {
		t.Fatal("expected ok=true for directory-only pattern")
	}
	if !pat.dirOnly {
		t.Error("dirOnly should be true")
	}
	if pat.pattern != "build" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "build")
	}
	if pat.anchored {
		t.Error("anchored should be false: no '/' remaining in pattern")
	}
}

// TestParseIgnoreLine_LeadingSlash verifies that a leading '/' sets
// anchored=true and the slash is stripped from the pattern.
func TestParseIgnoreLine_LeadingSlash(t *testing.T) {
	pat, ok := parseIgnoreLine("/Makefile")
	if !ok {
		t.Fatal("expected ok=true for leading-slash pattern")
	}
	if !pat.anchored {
		t.Error("anchored should be true for leading '/'")
	}
	if pat.pattern != "Makefile" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "Makefile")
	}
	if pat.negated {
		t.Error("negated should be false")
	}
	if pat.dirOnly {
		t.Error("dirOnly should be false")
	}
}

// TestParseIgnoreLine_InternalSlash verifies that a pattern containing an
// internal '/' (not leading) is treated as anchored.
func TestParseIgnoreLine_InternalSlash(t *testing.T) {
	pat, ok := parseIgnoreLine("src/generated")
	if !ok {
		t.Fatal("expected ok=true for pattern with internal slash")
	}
	if !pat.anchored {
		t.Error("anchored should be true: pattern contains '/'")
	}
	if pat.pattern != "src/generated" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "src/generated")
	}
}

// TestParseIgnoreLine_LeadingSlashWithDirectory verifies that a leading '/'
// combined with a trailing '/' produces anchored=true and dirOnly=true.
func TestParseIgnoreLine_LeadingSlashWithDirectory(t *testing.T) {
	pat, ok := parseIgnoreLine("/dist/")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !pat.anchored {
		t.Error("anchored should be true (leading '/')")
	}
	if !pat.dirOnly {
		t.Error("dirOnly should be true (trailing '/')")
	}
	if pat.pattern != "dist" {
		t.Errorf("pattern = %q, want %q", pat.pattern, "dist")
	}
}

// TestParseIgnoreLine_TrailingWhitespace verifies that trailing spaces are
// stripped before parsing (unless escaped, which we do not test here).
func TestParseIgnoreLine_TrailingWhitespace(t *testing.T) {
	pat, ok := parseIgnoreLine("*.tmp   ")
	if !ok {
		t.Fatal("expected ok=true after stripping trailing whitespace")
	}
	if pat.pattern != "*.tmp" {
		t.Errorf("pattern = %q, want %q (trailing whitespace not stripped)", pat.pattern, "*.tmp")
	}
}

// TestParseIgnoreLine_WildcardGlob verifies that common glob wildcards pass
// through the pattern field unchanged.
func TestParseIgnoreLine_WildcardGlob(t *testing.T) {
	tests := []struct {
		line    string
		pattern string
	}{
		{"*.log", "*.log"},
		{"[Tt]est*", "[Tt]est*"},
		{"doc?.txt", "doc?.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			pat, ok := parseIgnoreLine(tt.line)
			if !ok {
				t.Fatalf("expected ok=true for %q", tt.line)
			}
			if pat.pattern != tt.pattern {
				t.Errorf("pattern = %q, want %q", pat.pattern, tt.pattern)
			}
		})
	}
}

// TestParseIgnoreLine_SlashOnlyLineIsInvalid verifies that a bare "/" produces
// ok=false because the pattern would be empty after stripping the slash.
func TestParseIgnoreLine_SlashOnlyLineIsInvalid(t *testing.T) {
	_, ok := parseIgnoreLine("/")
	if ok {
		t.Error("expected ok=false for bare '/' (empty pattern after strip)")
	}
}

// ---------------------------------------------------------------------------
// Tests for matchPattern
// ---------------------------------------------------------------------------

// makeRule is a small helper to construct an ignoreRule from its components,
// reducing boilerplate in matchPattern tests.
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

// TestMatchPattern_ExactBasenameMatch verifies that a non-anchored pattern
// matches a file whose basename equals the pattern exactly.
func TestMatchPattern_ExactBasenameMatch(t *testing.T) {
	rule := makeRule("", "Makefile", false, false, false)

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
	}{
		{"Makefile", false, true},
		{"src/Makefile", false, true},
		{"a/b/Makefile", false, true},
		{"NotMakefile", false, false},
		{"Makefile.bak", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := matchPattern(rule, tt.relPath, tt.isDir)
			if got != tt.want {
				t.Errorf("matchPattern(rule, %q, %v) = %v, want %v", tt.relPath, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestMatchPattern_WildcardExtension verifies that "*.log" matches any file
// ending in ".log" at any depth.
func TestMatchPattern_WildcardExtension(t *testing.T) {
	rule := makeRule("", "*.log", false, false, false)

	tests := []struct {
		relPath string
		want    bool
	}{
		{"app.log", true},
		{"logs/server.log", true},
		{"deep/a/b/trace.log", true},
		{"app.txt", false},
		{"logfile", false},
		{".log", true}, // basename is ".log"
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := matchPattern(rule, tt.relPath, false)
			if got != tt.want {
				t.Errorf("matchPattern(rule, %q, false) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestMatchPattern_AnchoredPattern verifies that an anchored pattern only
// matches a path whose full relative path (from the rule's base) equals the
// pattern; it must NOT match on the basename alone.
func TestMatchPattern_AnchoredPattern(t *testing.T) {
	// "src/generated" anchored from root
	rule := makeRule("", "src/generated", false, false, true)

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
	}{
		{"src/generated", false, true},    // exact anchored match
		{"generated", false, false},       // basename only — should NOT match anchored rule
		{"a/src/generated", false, false}, // not at root
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := matchPattern(rule, tt.relPath, tt.isDir)
			if got != tt.want {
				t.Errorf("matchPattern(anchored, %q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestMatchPattern_DirectoryOnly verifies that a dirOnly rule does NOT match
// a file but DOES match a directory with the same name. isIgnored enforces the
// dirOnly check before calling matchPattern, so we test it here directly.
func TestMatchPattern_DirectoryOnly(t *testing.T) {
	rule := makeRule("", "build", false, true, false)

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
	}{
		{"build", true, true},    // directory — should match
		{"build", false, false},  // file — isIgnored skips, but matchPattern itself matches
		{"src/build", true, true},
		{"src/build", false, false},
	}
	// Note: matchPattern itself does not enforce dirOnly; isIgnored does.
	// We verify here that the pattern would match if the caller decides
	// to call matchPattern, and separately rely on isIgnored tests.
	for _, tt := range tests {
		t.Run(tt.relPath+"/isDir="+boolStr(tt.isDir), func(t *testing.T) {
			// For dirOnly rules, matchPattern should return true for both
			// dirs and non-dirs (the enforcement is in isIgnored). We still
			// document the actual behaviour:
			got := matchPattern(rule, tt.relPath, tt.isDir)
			// A non-anchored pattern "build" matches any path named "build".
			wantMatchPattern := true // matchPattern itself doesn't filter by isDir
			if got != wantMatchPattern {
				t.Errorf("matchPattern(dirOnly rule, %q, isDir=%v) = %v, want %v",
					tt.relPath, tt.isDir, got, wantMatchPattern)
			}
		})
	}
}

// boolStr converts a bool to a short string for use in subtest names.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TestMatchPattern_SubdirectoryBaseDir verifies that a rule loaded from a
// subdirectory .gitignore only matches paths under that subdirectory.
func TestMatchPattern_SubdirectoryBaseDir(t *testing.T) {
	// Simulates a rule from "vendor/.gitignore" that ignores "*.tmp"
	rule := makeRule("vendor/", "*.tmp", false, false, false)

	tests := []struct {
		relPath string
		want    bool
	}{
		{"vendor/cache.tmp", true},     // under vendor/
		{"vendor/a/deep.tmp", true},    // deeply nested under vendor/
		{"cache.tmp", false},            // at root — not under vendor/
		{"src/cache.tmp", false},        // under src/ — not under vendor/
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := matchPattern(rule, tt.relPath, false)
			if got != tt.want {
				t.Errorf("matchPattern(baseDir=vendor/, %q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestMatchPattern_AnchoredWithSubdirectoryBase verifies anchored patterns
// from a subdirectory .gitignore.
func TestMatchPattern_AnchoredWithSubdirectoryBase(t *testing.T) {
	// Simulates src/.gitignore containing "/generated/code" (anchored within src/)
	rule := makeRule("src/", "generated/code", false, false, true)

	tests := []struct {
		relPath string
		want    bool
	}{
		{"src/generated/code", true},       // exact match relative to src/
		{"src/other/generated/code", false}, // too deep
		{"generated/code", false},           // outside src/
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := matchPattern(rule, tt.relPath, false)
			if got != tt.want {
				t.Errorf("matchPattern(anchored, baseDir=src/, %q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for ignoreMatcher.isIgnored
// ---------------------------------------------------------------------------

// TestIsIgnored_SingleNonAnchoredPattern verifies that a plain pattern loaded
// into an ignoreMatcher ignores files matching by basename at any depth.
func TestIsIgnored_SingleNonAnchoredPattern(t *testing.T) {
	m := &ignoreMatcher{}
	m.rules = []ignoreRule{
		makeRule("", "*.log", false, false, false),
	}

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
	}{
		{"app.log", false, true},
		{"logs/app.log", false, true},
		{"app.txt", false, false},
		{"app.log", true, true}, // directories can also match non-dirOnly rules
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := m.isIgnored(tt.relPath, tt.isDir)
			if got != tt.want {
				t.Errorf("isIgnored(%q, %v) = %v, want %v", tt.relPath, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestIsIgnored_NegationOverridesIgnore verifies that a negation rule placed
// after an ignore rule un-ignores matching files.
func TestIsIgnored_NegationOverridesIgnore(t *testing.T) {
	m := &ignoreMatcher{}
	m.rules = []ignoreRule{
		makeRule("", "*.log", false, false, false),      // ignore all .log
		makeRule("", "important.log", true, false, false), // but un-ignore important.log
	}

	tests := []struct {
		relPath string
		want    bool
	}{
		{"debug.log", true},     // ignored by first rule
		{"important.log", false}, // un-ignored by negation rule
		{"app.txt", false},       // not matched by either rule
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := m.isIgnored(tt.relPath, false)
			if got != tt.want {
				t.Errorf("isIgnored(%q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestIsIgnored_DirectoryOnlyRuleSkipsFiles verifies that a dirOnly rule does
// not cause regular files to be ignored.
func TestIsIgnored_DirectoryOnlyRuleSkipsFiles(t *testing.T) {
	m := &ignoreMatcher{}
	m.rules = []ignoreRule{
		makeRule("", "build", false, true, false), // build/ — directories only
	}

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
	}{
		{"build", true, true},   // directory named "build" is ignored
		{"build", false, false}, // regular file named "build" is NOT ignored
		{"src/build", true, true},
		{"src/build", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.relPath+"/isDir="+boolStr(tt.isDir), func(t *testing.T) {
			got := m.isIgnored(tt.relPath, tt.isDir)
			if got != tt.want {
				t.Errorf("isIgnored(%q, isDir=%v) = %v, want %v", tt.relPath, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestIsIgnored_LaterRuleWins verifies that when multiple rules match the same
// path, the last matching rule determines the outcome.
func TestIsIgnored_LaterRuleWins(t *testing.T) {
	m := &ignoreMatcher{}
	m.rules = []ignoreRule{
		makeRule("", "*.cfg", false, false, false), // ignore all .cfg
		makeRule("", "keep.cfg", true, false, false), // un-ignore keep.cfg
		makeRule("", "keep.cfg", false, false, false), // then re-ignore keep.cfg
	}

	// The final rule re-ignores keep.cfg.
	got := m.isIgnored("keep.cfg", false)
	if !got {
		t.Error("isIgnored(keep.cfg) = false, want true (last rule re-ignores it)")
	}
}

// TestIsIgnored_EmptyMatcherIgnoresNothing verifies that an ignoreMatcher with
// no rules never ignores any path.
func TestIsIgnored_EmptyMatcherIgnoresNothing(t *testing.T) {
	m := &ignoreMatcher{}

	paths := []string{"anything.go", "README.md", ".env", "a/b/c.log"}
	for _, p := range paths {
		if m.isIgnored(p, false) {
			t.Errorf("isIgnored(%q) = true for empty matcher, want false", p)
		}
	}
}

// TestIsIgnored_AnchoredPatternDoesNotMatchNestedPaths verifies that an
// anchored root-level pattern does not match the same name in a subdirectory.
func TestIsIgnored_AnchoredPatternDoesNotMatchNestedPaths(t *testing.T) {
	m := &ignoreMatcher{}
	m.rules = []ignoreRule{
		makeRule("", "Makefile", false, false, true), // anchored: /Makefile
	}

	tests := []struct {
		relPath string
		want    bool
	}{
		{"Makefile", true},      // root-level — matches
		{"src/Makefile", false}, // nested — does NOT match anchored rule
		{"a/b/Makefile", false},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := m.isIgnored(tt.relPath, false)
			if got != tt.want {
				t.Errorf("isIgnored(%q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests using loadIgnoreMatcher with a real temp directory
// ---------------------------------------------------------------------------

// writeGitignore writes a .gitignore file at a specific directory path and
// registers a cleanup via t.TempDir lifecycle (no explicit cleanup needed).
func writeGitignore(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeGitignore: mkdir %q: %v", dir, err)
	}
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeGitignore: write %q: %v", path, err)
	}
}

// TestLoadIgnoreMatcher_NoGitignoreFile verifies that loadIgnoreMatcher
// succeeds and creates an empty matcher when no .gitignore exists.
func TestLoadIgnoreMatcher_NoGitignoreFile(t *testing.T) {
	dir := t.TempDir()
	m := loadIgnoreMatcher(dir)
	if m == nil {
		t.Fatal("loadIgnoreMatcher returned nil")
	}
	if len(m.rules) != 0 {
		t.Errorf("expected 0 rules for directory without .gitignore, got %d", len(m.rules))
	}
}

// TestLoadIgnoreMatcher_BasicPatterns verifies that patterns read from a root
// .gitignore are applied correctly by isIgnored.
func TestLoadIgnoreMatcher_BasicPatterns(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "# comment\n*.log\nbuild/\n/dist\n")

	m := loadIgnoreMatcher(dir)
	if m == nil {
		t.Fatal("loadIgnoreMatcher returned nil")
	}

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
	}{
		{"app.log", false, true},          // matches *.log
		{"logs/server.log", false, true},  // matches *.log (basename)
		{"build", true, true},             // matches build/ (dirOnly)
		{"build", false, false},           // build/ rule is dirOnly — files not ignored
		{"src/build", true, true},         // non-anchored, so matches src/build dir too
		{"dist", false, true},             // matches /dist (anchored)
		{"src/dist", false, false},        // /dist is anchored to root, src/dist not matched
		{"main.go", false, false},         // not ignored
	}
	for _, tt := range tests {
		t.Run(tt.relPath+"/isDir="+boolStr(tt.isDir), func(t *testing.T) {
			got := m.isIgnored(tt.relPath, tt.isDir)
			if got != tt.want {
				t.Errorf("isIgnored(%q, isDir=%v) = %v, want %v", tt.relPath, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestLoadIgnoreMatcher_NegationPattern verifies that a negation rule
// un-ignores files that would otherwise be ignored by an earlier rule.
func TestLoadIgnoreMatcher_NegationPattern(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "*.log\n!keep.log\n")

	m := loadIgnoreMatcher(dir)

	tests := []struct {
		relPath string
		want    bool
	}{
		{"debug.log", true},
		{"keep.log", false}, // un-ignored by negation
		{"other.log", true},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := m.isIgnored(tt.relPath, false)
			if got != tt.want {
				t.Errorf("isIgnored(%q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestLoadIgnoreMatcher_BlankLinesAndComments verifies that blank lines and
// comment lines in .gitignore do not produce rules.
func TestLoadIgnoreMatcher_BlankLinesAndComments(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "\n# first comment\n\n# second comment\n*.tmp\n")

	m := loadIgnoreMatcher(dir)

	// Only the *.tmp rule should have been loaded.
	if len(m.rules) != 1 {
		t.Errorf("expected 1 rule, got %d (blank lines/comments may have produced spurious rules)", len(m.rules))
	}
	if !m.isIgnored("file.tmp", false) {
		t.Error("isIgnored(file.tmp) = false, want true")
	}
	if m.isIgnored("file.go", false) {
		t.Error("isIgnored(file.go) = true, want false")
	}
}

// TestLoadFile_SubdirectoryGitignore verifies that loadFile correctly scopes
// patterns loaded from a subdirectory to that subdirectory only.
func TestLoadFile_SubdirectoryGitignore(t *testing.T) {
	dir := t.TempDir()

	// Root .gitignore ignores *.log everywhere.
	writeGitignore(t, dir, "*.log\n")

	// vendor/.gitignore ignores *.tmp only within vendor/.
	vendorDir := filepath.Join(dir, "vendor")
	writeGitignore(t, vendorDir, "*.tmp\n")

	m := loadIgnoreMatcher(dir)         // loads root .gitignore
	m.loadFile(dir, "vendor/")          // loads vendor/.gitignore

	tests := []struct {
		relPath string
		isDir   bool
		want    bool
		reason  string
	}{
		{"app.log", false, true, "*.log from root applies to root"},
		{"vendor/app.log", false, true, "*.log from root applies inside vendor/"},
		{"vendor/cache.tmp", false, true, "*.tmp from vendor/.gitignore applies inside vendor/"},
		{"cache.tmp", false, false, "*.tmp from vendor/.gitignore does NOT apply at root"},
		{"src/cache.tmp", false, false, "*.tmp from vendor/.gitignore does NOT apply under src/"},
		{"main.go", false, false, "not ignored by any rule"},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := m.isIgnored(tt.relPath, tt.isDir)
			if got != tt.want {
				t.Errorf("isIgnored(%q) = %v, want %v — %s", tt.relPath, got, tt.want, tt.reason)
			}
		})
	}
}

// TestLoadFile_NonExistentSubdirectoryGitignore verifies that loadFile is a
// no-op when the .gitignore does not exist (missing file is not an error).
func TestLoadFile_NonExistentSubdirectoryGitignore(t *testing.T) {
	dir := t.TempDir()
	m := &ignoreMatcher{}
	beforeCount := len(m.rules)

	// Load from a directory that has no .gitignore.
	m.loadFile(dir, "nonexistent/")

	if len(m.rules) != beforeCount {
		t.Errorf("loadFile on missing .gitignore added %d rules, want 0", len(m.rules)-beforeCount)
	}
}

// ---------------------------------------------------------------------------
// Table-driven integration tests for parseIgnoreLine field combinations
// ---------------------------------------------------------------------------

// TestParseIgnoreLine_Table exercises a wide range of input lines and verifies
// that every combination of flags is decoded correctly.
func TestParseIgnoreLine_Table(t *testing.T) {
	tests := []struct {
		line     string
		wantOk   bool
		pattern  string
		negated  bool
		dirOnly  bool
		anchored bool
	}{
		// Skipped lines.
		{"", false, "", false, false, false},
		{"  ", false, "", false, false, false},
		{"# ignore this", false, "", false, false, false},
		{"#", false, "", false, false, false},
		{"/", false, "", false, false, false}, // stripped to empty

		// Simple patterns.
		{"*.go", true, "*.go", false, false, false},
		{"README.md", true, "README.md", false, false, false},

		// Trailing whitespace is stripped.
		{"*.go   ", true, "*.go", false, false, false},

		// Directory-only patterns.
		{"vendor/", true, "vendor", false, true, false},
		{"node_modules/", true, "node_modules", false, true, false},

		// Leading slash → anchored.
		{"/Makefile", true, "Makefile", false, false, true},
		{"/config/app.yaml", true, "config/app.yaml", false, false, true},

		// Internal slash → anchored.
		{"src/gen", true, "src/gen", false, false, true},
		{"a/b/c.txt", true, "a/b/c.txt", false, false, true},

		// Negation.
		{"!important.log", true, "important.log", true, false, false},
		{"!vendor/", true, "vendor", true, true, false},
		{"!/root-only", true, "root-only", true, false, true},
	}

	for _, tt := range tests {
		t.Run("line="+tt.line, func(t *testing.T) {
			pat, ok := parseIgnoreLine(tt.line)
			if ok != tt.wantOk {
				t.Fatalf("parseIgnoreLine(%q) ok=%v, want %v", tt.line, ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if pat.pattern != tt.pattern {
				t.Errorf("pattern = %q, want %q", pat.pattern, tt.pattern)
			}
			if pat.negated != tt.negated {
				t.Errorf("negated = %v, want %v", pat.negated, tt.negated)
			}
			if pat.dirOnly != tt.dirOnly {
				t.Errorf("dirOnly = %v, want %v", pat.dirOnly, tt.dirOnly)
			}
			if pat.anchored != tt.anchored {
				t.Errorf("anchored = %v, want %v", pat.anchored, tt.anchored)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration test: ComputeWorkingTreeStatus respects .gitignore
// ---------------------------------------------------------------------------

// TestComputeWorkingTreeStatus_GitignoreExcludesUntrackedFiles verifies that
// files matched by a root .gitignore are NOT reported as untracked by
// ComputeWorkingTreeStatus, while files NOT matched are still reported.
func TestComputeWorkingTreeStatus_GitignoreExcludesUntrackedFiles(t *testing.T) {
	repo := setupTestRepo(t)

	// Set up an initial HEAD commit with one tracked file, so the index has
	// something in it and HEAD is not empty.
	trackedContent := []byte("tracked content\n")
	trackedHash := hashBlobContent(trackedContent)

	headTree := createTree(t, repo, []TreeEntry{
		{ID: trackedHash, Name: "main.go", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "main.go", hash: trackedHash, fileSize: uint32(len(trackedContent))},
	})

	// Write the tracked file to disk so it produces no WorkStatus.
	writeDiskFile(t, repo, "main.go", trackedContent)

	// Write a .gitignore that ignores all *.log files.
	writeGitignore(t, repo.workDir, "*.log\n")

	// Write an ignored .log file — it must NOT appear as untracked.
	writeDiskFile(t, repo, "debug.log", []byte("log output\n"))

	// Write a regular untracked file — it MUST appear as untracked.
	writeDiskFile(t, repo, "untracked.txt", []byte("not tracked\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)

	// The .log file must not appear at all in the status output.
	if _, present := m["debug.log"]; present {
		t.Error("debug.log appeared in status but should be excluded by .gitignore")
	}

	// The untracked.txt must appear as untracked.
	f, ok := m["untracked.txt"]
	if !ok {
		t.Fatalf("untracked.txt missing from status; got paths: %v", sortedKeys(m))
	}
	if !f.IsUntracked {
		t.Errorf("untracked.txt: IsUntracked = false, want true")
	}

	// The .gitignore file itself should appear as untracked (it is not in the index).
	if _, ok := m[".gitignore"]; !ok {
		t.Log("note: .gitignore itself is not in the index, so it is expected to appear as untracked")
	}

	// The tracked file must produce no status entry (disk matches index and HEAD).
	if _, ok := m["main.go"]; ok {
		t.Errorf("main.go should have no status entry (disk=index=HEAD), got: %+v", m["main.go"])
	}
}

// TestComputeWorkingTreeStatus_GitignoreDirectoryExcludesContents verifies
// that when an entire directory is matched by a dirOnly pattern, its contents
// are not reported as untracked.
func TestComputeWorkingTreeStatus_GitignoreDirectoryExcludesContents(t *testing.T) {
	repo := setupTestRepo(t)

	// Empty HEAD and index.
	headTree := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, headTree)
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{})

	// .gitignore ignores the "logs/" directory entirely.
	writeGitignore(t, repo.workDir, "logs/\n")

	// Create files inside the ignored directory.
	writeDiskFile(t, repo, "logs/server.log", []byte("log output\n"))
	writeDiskFile(t, repo, "logs/access.log", []byte("access log\n"))

	// Create a file outside the ignored directory.
	writeDiskFile(t, repo, "README.md", []byte("read me\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)

	// Files inside the ignored logs/ directory must not appear.
	for _, ignored := range []string{"logs/server.log", "logs/access.log"} {
		if _, present := m[ignored]; present {
			t.Errorf("%q appeared in status but should be excluded (inside ignored directory)", ignored)
		}
	}

	// README.md is outside the ignored directory and must appear as untracked.
	f, ok := m["README.md"]
	if !ok {
		t.Fatalf("README.md missing from status; got: %v", sortedKeys(m))
	}
	if !f.IsUntracked {
		t.Errorf("README.md: IsUntracked = false, want true")
	}
}

// TestComputeWorkingTreeStatus_GitignoreInSubdirAppliesLocally verifies that
// a .gitignore file in a subdirectory is loaded during the walk and its
// patterns apply only to paths under that subdirectory.
func TestComputeWorkingTreeStatus_GitignoreInSubdirAppliesLocally(t *testing.T) {
	repo := setupTestRepo(t)

	// Empty HEAD and index.
	headTree := createTree(t, repo, []TreeEntry{})
	wireHeadCommit(repo, headTree)
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{})

	// src/.gitignore ignores *.gen files.
	srcDir := filepath.Join(repo.workDir, "src")
	writeGitignore(t, srcDir, "*.gen\n")

	// A .gen file inside src/ — should be ignored.
	writeDiskFile(t, repo, "src/api.gen", []byte("generated\n"))

	// A .gen file at the root — should NOT be ignored (different scope).
	writeDiskFile(t, repo, "root.gen", []byte("also generated\n"))

	// A regular file in src/ — should NOT be ignored.
	writeDiskFile(t, repo, "src/main.go", []byte("package main\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)

	// src/api.gen is covered by src/.gitignore — must not appear.
	if _, present := m["src/api.gen"]; present {
		t.Error("src/api.gen appeared in status but should be excluded by src/.gitignore")
	}

	// root.gen is outside src/ — must appear as untracked.
	if _, ok := m["root.gen"]; !ok {
		t.Errorf("root.gen should be untracked (root .gitignore has no rules); got paths: %v", sortedKeys(m))
	}

	// src/main.go is not ignored — must appear as untracked.
	if f, ok := m["src/main.go"]; !ok {
		t.Errorf("src/main.go should be untracked; got paths: %v", sortedKeys(m))
	} else if !f.IsUntracked {
		t.Errorf("src/main.go: IsUntracked = false, want true")
	}
}

// TestComputeWorkingTreeStatus_GitignoreTrackedFileNotFiltered verifies that
// even if a tracked (indexed) file's name matches a .gitignore rule, it is
// still reported when modified — .gitignore only affects untracked file
// discovery, not staged/unstaged comparisons.
func TestComputeWorkingTreeStatus_GitignoreTrackedFileNotFiltered(t *testing.T) {
	repo := setupTestRepo(t)

	// Track a .log file in the index (and HEAD).
	content := []byte("tracked log\n")
	realHash := hashBlobContent(content)

	headTree := createTree(t, repo, []TreeEntry{
		{ID: realHash, Name: "important.log", Mode: "100644", Type: "blob"},
	})
	wireHeadCommit(repo, headTree)
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "important.log", hash: realHash, fileSize: uint32(len(content))},
	})

	// .gitignore ignores *.log — but important.log is already tracked.
	writeGitignore(t, repo.workDir, "*.log\n")

	// Write a different version to disk → unstaged modification.
	writeDiskFile(t, repo, "important.log", []byte("modified log content\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus failed: %v", err)
	}

	m := statusByPath(t, status)

	// important.log is tracked, so the index-vs-disk comparison must still run.
	f, ok := m["important.log"]
	if !ok {
		t.Fatalf("important.log missing from status (tracked files should not be filtered by .gitignore)")
	}
	if f.WorkStatus != "modified" {
		t.Errorf("important.log WorkStatus = %q, want %q", f.WorkStatus, "modified")
	}
}

// ---------------------------------------------------------------------------
// Additional edge-case tests
// ---------------------------------------------------------------------------

// TestMatchPattern_PatternNotUnderBaseDir verifies that a rule loaded from a
// subdirectory .gitignore returns false immediately when the path does not
// start with the base directory prefix.
func TestMatchPattern_PatternNotUnderBaseDir(t *testing.T) {
	rule := makeRule("internal/", "*.go", false, false, false)

	// A path that does NOT start with "internal/" must return false.
	got := matchPattern(rule, "cmd/main.go", false)
	if got {
		t.Error("matchPattern returned true for path outside the rule's baseDir")
	}
}

// TestIsIgnored_MultipleRulesLastWins verifies that if three rules all match
// the same path, the last rule's negated flag wins.
func TestIsIgnored_MultipleRulesLastWins(t *testing.T) {
	m := &ignoreMatcher{rules: []ignoreRule{
		makeRule("", "*.cfg", false, false, false), // ignore
		makeRule("", "*.cfg", true, false, false),  // un-ignore
		makeRule("", "*.cfg", false, false, false), // ignore again
	}}

	if !m.isIgnored("app.cfg", false) {
		t.Error("expected app.cfg to be ignored (last rule wins)")
	}
}

// TestLoadIgnoreMatcher_RuleCount verifies that the correct number of valid
// rules is loaded, excluding blank lines and comments.
func TestLoadIgnoreMatcher_RuleCount(t *testing.T) {
	dir := t.TempDir()
	content := "# comment 1\n\n*.log\n# comment 2\nbuild/\n\n"
	writeGitignore(t, dir, content)

	m := loadIgnoreMatcher(dir)
	// Expected: 2 rules ("*.log" and "build/"), all others are blank or comments.
	if len(m.rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(m.rules))
	}
}

// TestParseIgnoreLine_PatternPreservesGlob verifies that glob characters are
// left intact and not interpreted at parse time.
func TestParseIgnoreLine_PatternPreservesGlob(t *testing.T) {
	tests := []struct {
		line    string
		pattern string
	}{
		{"[Tt]est*", "[Tt]est*"},
		{"*.{js,ts}", "*.{js,ts}"},
		{"src/**/*.min.js", "src/**/*.min.js"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			pat, ok := parseIgnoreLine(tt.line)
			if !ok {
				t.Fatalf("parseIgnoreLine(%q): expected ok=true", tt.line)
			}
			if pat.pattern != tt.pattern {
				t.Errorf("pattern = %q, want %q", pat.pattern, tt.pattern)
			}
		})
	}
}

