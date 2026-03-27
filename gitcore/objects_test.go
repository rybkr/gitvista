package gitcore

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testHash1 = "1111111111111111111111111111111111111111"
	testHash2 = "2222222222222222222222222222222222222222"
	testHash3 = "3333333333333333333333333333333333333333"
	testHash4 = "4444444444444444444444444444444444444444"
	testHash5 = "5555555555555555555555555555555555555555"
	testHash6 = "6666666666666666666666666666666666666666"
	testHash7 = "7777777777777777777777777777777777777777"
)

func mustHash(t *testing.T, s string) Hash {
	t.Helper()
	h, err := NewHash(s)
	if err != nil {
		t.Fatalf("NewHash(%q): %v", s, err)
	}
	return h
}

func compressBytes(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(content); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

func writeLooseObject(t *testing.T, gitDir string, id Hash, objectType string, body []byte) {
	t.Helper()
	dir := filepath.Join(gitDir, "objects", string(id)[:2])
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir loose object dir: %v", err)
	}
	payload := append([]byte(fmt.Sprintf("%s %d", objectType, len(body))), 0)
	payload = append(payload, body...)
	if err := os.WriteFile(filepath.Join(dir, string(id)[2:]), compressBytes(t, payload), 0o600); err != nil {
		t.Fatalf("write loose object: %v", err)
	}
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newRepoSkeleton(t *testing.T) *Repository {
	t.Helper()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	for _, dir := range []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "tags"),
		filepath.Join(gitDir, "refs", "remotes"),
		filepath.Join(gitDir, "logs", "refs"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeTextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return &Repository{
		gitDir:        gitDir,
		workDir:       root,
		refs:          make(map[string]Hash),
		commitMap:     make(map[Hash]*Commit),
		packLocations: make(map[Hash]PackLocation),
		packReaders:   make(map[string]*PackReader),
	}
}

func TestHashHelpers(t *testing.T) {
	if _, err := NewHash("short"); err == nil {
		t.Fatal("expected invalid length error")
	}
	if _, err := NewHash("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"); err == nil {
		t.Fatal("expected invalid hex error")
	}

	h, err := NewHash("ABCDEFabcdef0123456789012345678901234567")
	if err != nil {
		t.Fatalf("NewHash: %v", err)
	}
	if got := h.Short(); got != "ABCDEFa" {
		t.Fatalf("Short() = %q", got)
	}
	if got := Hash("abc").Short(); got != "abc" {
		t.Fatalf("short hash Short() = %q", got)
	}
}

func TestSignatureParsing(t *testing.T) {
	sig, err := NewSignature("Jane Doe <jane@example.com> 1700000000 -0530")
	if err != nil {
		t.Fatalf("NewSignature: %v", err)
	}
	if sig.Name != "Jane Doe" || sig.Email != "jane@example.com" || sig.When.Format("-0700") != "-0530" {
		t.Fatalf("unexpected signature: %+v", sig)
	}

	utcSig, err := NewSignature("Jane Doe <jane@example.com> 1700000000")
	if err != nil {
		t.Fatalf("NewSignature without tz: %v", err)
	}
	if utcSig.When.Location() != time.UTC {
		t.Fatal("expected UTC fallback")
	}

	for _, input := range []string{
		"missing parts",
		"Jane Doe <jane@example.com> nope +0000",
		"Jane Doe <jane@example.com> ",
	} {
		if _, err := NewSignature(input); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}

	if parseTimezone("+0530").String() != "+0530" {
		t.Fatal("expected fixed zone")
	}
	for _, input := range []string{"UTC", "*530", "+0a30", "+053x"} {
		if parseTimezone(input) != nil {
			t.Fatalf("expected nil timezone for %q", input)
		}
	}
}

func TestObjectTypeAndParsingHelpers(t *testing.T) {
	if ObjectTypeCommit.String() != "commit" || ObjectTypeTree.String() != "tree" || ObjectTypeBlob.String() != "blob" || ObjectTypeTag.String() != "tag" || ObjectTypeInvalid.String() != "invalid" {
		t.Fatal("unexpected ObjectType.String values")
	}
	if ParseObjectType("tree") != ObjectTypeTree || ParseObjectType("other") != ObjectTypeInvalid {
		t.Fatal("unexpected ParseObjectType result")
	}
	if typ, err := objectTypeFromHeader("blob 12"); err != nil || typ != ObjectTypeBlob {
		t.Fatalf("objectTypeFromHeader valid: %v %v", typ, err)
	}
	if _, err := objectTypeFromHeader("badheader"); err == nil {
		t.Fatal("expected invalid header error")
	}
	if _, err := objectTypeFromHeader("unknown 1"); err == nil {
		t.Fatal("expected unsupported type error")
	}

	commitBody := []byte("tree " + testHash2 + "\nparent " + testHash3 + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\nignored header\n\nhello\nworld\n")
	commit, err := parseCommitBody(commitBody, mustHash(t, testHash1))
	if err != nil {
		t.Fatalf("parseCommitBody: %v", err)
	}
	if commit.Type() != ObjectTypeCommit || commit.Tree != Hash(testHash2) || len(commit.Parents) != 1 || commit.Message != "hello\nworld" {
		t.Fatalf("unexpected commit: %+v", commit)
	}

	if _, parseErr := parseCommitBody([]byte("tree no\n"), mustHash(t, testHash1)); parseErr == nil {
		t.Fatal("expected invalid tree hash")
	}
	if parseErr := parseCommitHeaderLine(commit, []byte("author bad")); parseErr == nil {
		t.Fatal("expected invalid author error")
	}
	if parseErr := parseCommitHeaderLine(commit, []byte("committer bad")); parseErr == nil {
		t.Fatal("expected invalid committer error")
	}
	if parseErr := parseCommitHeaderLine(commit, []byte("parent nope")); parseErr == nil {
		t.Fatal("expected invalid parent error")
	}
	if parseErr := parseCommitHeaderLine(commit, []byte("extra header")); parseErr != nil {
		t.Fatalf("unexpected unknown header error: %v", parseErr)
	}

	treeHash := hashFromHex(testHash2)
	blobHash := hashFromHex(testHash3)
	linkHash := hashFromHex(testHash4)
	submoduleHash := hashFromHex(testHash5)
	unknownHash := hashFromHex(testHash6)
	var treeBody bytes.Buffer
	treeBody.WriteString("040000 dir")
	treeBody.WriteByte(0)
	treeBody.Write(treeHash[:])
	treeBody.WriteString("100644 file.txt")
	treeBody.WriteByte(0)
	treeBody.Write(blobHash[:])
	treeBody.WriteString("120000 symlink")
	treeBody.WriteByte(0)
	treeBody.Write(linkHash[:])
	treeBody.WriteString("160000 submodule")
	treeBody.WriteByte(0)
	treeBody.Write(submoduleHash[:])
	treeBody.WriteString("000000 weird")
	treeBody.WriteByte(0)
	treeBody.Write(unknownHash[:])

	tree, err := parseTreeBody(treeBody.Bytes(), mustHash(t, testHash1))
	if err != nil {
		t.Fatalf("parseTreeBody: %v", err)
	}
	if tree.Type() != ObjectTypeTree || len(tree.Entries) != 5 {
		t.Fatalf("unexpected tree: %+v", tree)
	}
	if tree.Entries[0].Type != ObjectTypeTree || tree.Entries[1].Type != ObjectTypeBlob || tree.Entries[2].Type != ObjectTypeBlob || tree.Entries[3].Type != ObjectTypeCommit || tree.Entries[4].Type != ObjectTypeInvalid {
		t.Fatal("tree entry types not parsed as expected")
	}
	if _, parseErr := parseTreeBody([]byte("100644 "), mustHash(t, testHash1)); parseErr == nil {
		t.Fatal("expected truncated tree name error")
	}
	if _, parseErr := parseTreeBody(append([]byte("100644 file"), 0), mustHash(t, testHash1)); parseErr == nil {
		t.Fatal("expected truncated tree hash error")
	}

	tagBody := []byte("object " + testHash2 + "\ntype commit\ntag v1.0\ntagger Jane Doe <jane@example.com> 1700000000 +0000\n\nline 1\nline 2\n")
	tag, err := parseTagBody(tagBody, mustHash(t, testHash1))
	if err != nil {
		t.Fatalf("parseTagBody: %v", err)
	}
	if tag.Type() != ObjectTypeTag || tag.Object != Hash(testHash2) || tag.ObjType != ObjectTypeCommit || tag.Name != "v1.0" || tag.Message != "line 1\nline 2" {
		t.Fatalf("unexpected tag: %+v", tag)
	}
	if _, parseErr := parseTagBody([]byte("object bad\n"), mustHash(t, testHash1)); parseErr == nil {
		t.Fatal("expected invalid object hash")
	}
	if _, parseErr := parseTagBody([]byte("tagger bad\n"), mustHash(t, testHash1)); parseErr == nil {
		t.Fatal("expected invalid tagger")
	}

	blobObj, err := parseObject(mustHash(t, testHash1), ObjectTypeBlob, []byte("body"))
	if err != nil {
		t.Fatalf("parseObject blob: %v", err)
	}
	if blobObj.Type() != ObjectTypeBlob {
		t.Fatal("expected blob object type")
	}
	if obj, err := parseObject(mustHash(t, testHash1), ObjectTypeCommit, commitBody); err != nil || obj.Type() != ObjectTypeCommit {
		t.Fatalf("parseObject commit: %v %v", obj, err)
	}
	if obj, err := parseObject(mustHash(t, testHash1), ObjectTypeTree, treeBody.Bytes()); err != nil || obj.Type() != ObjectTypeTree {
		t.Fatalf("parseObject tree: %v %v", obj, err)
	}
	if obj, err := parseObject(mustHash(t, testHash1), ObjectTypeTag, tagBody); err != nil || obj.Type() != ObjectTypeTag {
		t.Fatalf("parseObject tag: %v %v", obj, err)
	}
	if _, err := parseObject(mustHash(t, testHash1), ObjectTypeInvalid, nil); err == nil {
		t.Fatal("expected parseObject unknown type error")
	}
}

func TestReadCompressedDataAndLooseObjects(t *testing.T) {
	if _, err := readCompressedData(strings.NewReader("not-zlib")); err == nil {
		t.Fatal("expected invalid zlib error")
	}

	if _, err := readCompressedObject(strings.NewReader("bad"), 1); err == nil {
		t.Fatal("expected invalid compressed object error")
	}

	repo := newRepoSkeleton(t)
	id := mustHash(t, testHash1)
	writeLooseObject(t, repo.gitDir, id, "blob", []byte("hello"))
	header, content, err := repo.readLooseObjectRaw(id)
	if err != nil {
		t.Fatalf("readLooseObjectRaw: %v", err)
	}
	if header != "blob 5" || string(content) != "hello" {
		t.Fatalf("unexpected loose object: %q %q", header, string(content))
	}

	badID := mustHash(t, testHash2)
	dir := filepath.Join(repo.gitDir, "objects", string(badID)[:2])
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, string(badID)[2:]), compressBytes(t, []byte("missing-null")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.readLooseObjectRaw(badID); err == nil {
		t.Fatal("expected invalid object format")
	}

	if _, _, err := repo.readLooseObjectRaw(mustHash(t, testHash3)); err == nil {
		t.Fatal("expected missing loose object error")
	}

	badCompressedID := mustHash(t, testHash4)
	badCompressedDir := filepath.Join(repo.gitDir, "objects", string(badCompressedID)[:2])
	if err := os.MkdirAll(badCompressedDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badCompressedDir, string(badCompressedID)[2:]), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.readLooseObjectRaw(badCompressedID); err == nil {
		t.Fatal("expected invalid compressed loose object error")
	}
}

func TestReadObjectPreservesLooseObjectCorruptionErrors(t *testing.T) {
	repo := newRepoSkeleton(t)

	corruptID := mustHash(t, testHash1)
	corruptDir := filepath.Join(repo.gitDir, "objects", string(corruptID)[:2])
	if err := os.MkdirAll(corruptDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, string(corruptID)[2:]), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.readObject(corruptID); err == nil || strings.Contains(err.Error(), "object not found") {
		t.Fatalf("readObject() error = %v, want corruption error", err)
	}
	if _, _, err := repo.readObjectData(corruptID, 0); err == nil || strings.Contains(err.Error(), "object not found") {
		t.Fatalf("readObjectData() error = %v, want corruption error", err)
	}

	missingID := mustHash(t, testHash2)
	if _, err := repo.readObject(missingID); err == nil || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("readObject(missing) error = %v, want not found", err)
	}
	if _, _, err := repo.readObjectData(missingID, 0); err == nil || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("readObjectData(missing) error = %v, want not found", err)
	}
}

func TestRepositoryAndRefsFlow(t *testing.T) {
	repo := newRepoSkeleton(t)
	config := `[remote "origin"]
	url = ` + "https://" + "user" + ":" + "pass" + "@example.com/repo.git" + `
[remote "ssh"]
	url = git@example.com:repo.git
[core]
	bare = false
`
	writeTextFile(t, filepath.Join(repo.gitDir, "config"), config)

	blobID := mustHash(t, testHash1)
	treeID := mustHash(t, testHash2)
	parentID := mustHash(t, testHash3)
	commitID := mustHash(t, testHash4)
	tagID := mustHash(t, testHash5)

	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("hello"))
	var treeBody bytes.Buffer
	blobBytes := hashFromHex(testHash1)
	treeBody.WriteString("100644 file.txt")
	treeBody.WriteByte(0)
	treeBody.Write(blobBytes[:])
	writeLooseObject(t, repo.gitDir, treeID, "tree", treeBody.Bytes())
	writeLooseObject(t, repo.gitDir, parentID, "commit", []byte("tree "+testHash2+"\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nparent"))
	writeLooseObject(t, repo.gitDir, commitID, "commit", []byte("tree "+testHash2+"\nparent "+testHash3+"\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nchild"))
	writeLooseObject(t, repo.gitDir, tagID, "tag", []byte("object "+testHash4+"\ntype commit\ntag v1.0\ntagger Jane Doe <jane@example.com> 1700000000 +0000\n\nrelease"))

	writeTextFile(t, filepath.Join(repo.gitDir, "packed-refs"), strings.Join([]string{
		"# packed-refs with: peeled fully-peeled",
		"",
		"bad line",
		testHash4 + " refs/heads/main",
		"invalid refs/tags/bad",
		testHash5 + " refs/tags/v1.0",
		"^" + testHash4,
	}, "\n"))
	if err := repo.loadPackedRefs(); err == nil {
		t.Fatal("expected joined packed-ref parse errors")
	}
	if repo.refs["refs/heads/main"] != commitID {
		t.Fatal("expected valid packed ref to load")
	}

	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "main"), testHash4+"\n")
	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "bad"), "nope\n")
	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "remotes", "origin", "HEAD"), "ref: refs/heads/main\n")
	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "tags", "v1.0"), testHash5+"\n")
	if err := repo.loadLooseRefs("heads"); err == nil {
		t.Fatal("expected loose ref error for invalid ref")
	}
	if err := repo.loadLooseRefs("remotes"); err != nil {
		t.Fatalf("loadLooseRefs remotes: %v", err)
	}
	if err := repo.loadLooseRefs("tags"); err != nil {
		t.Fatalf("loadLooseRefs tags: %v", err)
	}
	if err := repo.loadLooseRefs("missing"); err != nil {
		t.Fatalf("missing loose refs should be ignored: %v", err)
	}

	if err := repo.loadHEAD(); err != nil {
		t.Fatalf("loadHEAD symbolic: %v", err)
	}
	if repo.head != commitID || repo.headRef != "refs/heads/main" || repo.headDetached {
		t.Fatalf("unexpected symbolic HEAD state: %+v", repo)
	}

	writeTextFile(t, filepath.Join(repo.gitDir, "HEAD"), testHash3+"\n")
	if err := repo.loadHEAD(); err != nil {
		t.Fatalf("loadHEAD detached: %v", err)
	}
	if repo.head != parentID || !repo.headDetached || repo.headRef != "" {
		t.Fatalf("unexpected detached HEAD state: %+v", repo)
	}
	writeTextFile(t, filepath.Join(repo.gitDir, "HEAD"), "bad\n")
	if err := repo.loadHEAD(); err == nil {
		t.Fatal("expected invalid HEAD error")
	}
	writeTextFile(t, filepath.Join(repo.gitDir, "HEAD"), "ref: refs/heads/main\n")
	delete(repo.refs, "refs/heads/main")
	if err := repo.loadHEAD(); err != nil {
		t.Fatalf("loadHEAD unborn branch: %v", err)
	}
	if repo.head != "" {
		t.Fatal("expected empty HEAD for unborn branch")
	}
	repo.refs["refs/heads/main"] = commitID

	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "stash"), testHash4+"\n")
	writeTextFile(t, filepath.Join(repo.gitDir, "logs", "refs", "stash"), strings.Join([]string{
		testHash1 + " " + testHash3 + " a <a@b> 1 +0000\tfirst",
		"broken line",
		testHash3 + " " + testHash4 + " a <a@b> 1 +0000",
	}, "\n"))
	if err := repo.loadStashes(); err == nil {
		t.Fatal("expected invalid stash reflog error")
	}
	if len(repo.stashes) != 2 || repo.stashes[0].Hash != commitID || repo.stashes[0].Message != "stash@{0}" || repo.stashes[1].Message != "first" {
		t.Fatalf("unexpected stashes: %+v", repo.stashes)
	}

	repo3 := newRepoSkeleton(t)
	if err := repo3.loadStashes(); err != nil {
		t.Fatalf("missing stash should be ignored: %v", err)
	}

	repo2 := newRepoSkeleton(t)
	writeTextFile(t, filepath.Join(repo2.gitDir, "refs", "stash"), testHash2+"\n")
	if err := repo2.loadStashes(); err != nil {
		t.Fatalf("loadStashes fallback: %v", err)
	}
	if len(repo2.stashes) != 1 || repo2.stashes[0].Message != "stash@{0}" {
		t.Fatalf("unexpected fallback stash: %+v", repo2.stashes)
	}

	resolved, err := repo.resolveRef(filepath.Join(repo.gitDir, "refs", "heads", "main"))
	if err != nil || resolved != commitID {
		t.Fatalf("resolveRef main: %v %v", resolved, err)
	}
	if _, err := repo.resolveRefDepth(filepath.Join(repo.gitDir, "..", "oops"), 0); err == nil {
		t.Fatal("expected path escape error")
	}
	chainPath := filepath.Join(repo.gitDir, "refs", "heads", "chain")
	writeTextFile(t, chainPath, "ref: refs/heads/step1\n")
	for i := 1; i <= 11; i++ {
		next := fmt.Sprintf("step%d", i+1)
		content := "ref: refs/heads/" + next + "\n"
		if i == 11 {
			content = testHash4 + "\n"
		}
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", fmt.Sprintf("step%d", i)), content)
	}
	if _, err := repo.resolveRefDepth(chainPath, 0); err == nil {
		t.Fatal("expected deep symref error")
	}

	if err := ensurePathWithinBase(repo.gitDir, filepath.Join(repo.gitDir, "refs")); err != nil {
		t.Fatalf("ensurePathWithinBase valid: %v", err)
	}
	if err := ensurePathWithinBase(repo.gitDir, filepath.Join(repo.gitDir, "..", "refs")); err == nil {
		t.Fatal("expected ensurePathWithinBase escape error")
	}

	if remotes := parseRemotesFromConfig(config); remotes["origin"] != "https://example.com/repo.git" || remotes["ssh"] != "git@example.com:repo.git" {
		t.Fatalf("unexpected remotes: %+v", remotes)
	}
	if got := stripCredentials("http://user:pass@example.com/repo.git"); got != "http://example.com/repo.git" {
		t.Fatalf("stripCredentials http: %q", got)
	}
	if got := stripCredentials("ssh://git@example.com/repo.git"); got != "ssh://git@example.com/repo.git" {
		t.Fatalf("stripCredentials passthrough: %q", got)
	}

	if err := repo.loadObjects(); err != nil {
		t.Fatalf("loadObjects: %v", err)
	}
	if len(repo.commits) != 2 || len(repo.tags) != 1 || repo.commitMap[commitID] == nil {
		t.Fatalf("unexpected loaded objects: commits=%d tags=%d", len(repo.commits), len(repo.tags))
	}

	if obj, err := repo.readObject(commitID); err != nil || obj.Type() != ObjectTypeCommit {
		t.Fatalf("readObject loose commit: %v %v", obj, err)
	}
	if data, typ, err := repo.readObjectData(blobID, 0); err != nil || typ != ObjectTypeBlob || string(data) != "hello" {
		t.Fatalf("readObjectData loose blob: %v %v %v", string(data), typ, err)
	}
	if _, err := repo.readObject(mustHash(t, testHash6)); err == nil {
		t.Fatal("expected readObject miss")
	}
	writeLooseObject(t, repo.gitDir, mustHash(t, testHash6), "bogus", []byte("x"))
	if _, err := repo.readObject(mustHash(t, testHash6)); err == nil {
		t.Fatal("expected unrecognized loose object type")
	}
	if _, _, err := repo.readObjectData(mustHash(t, testHash6), 0); err == nil {
		t.Fatal("expected readObjectData invalid loose type error")
	}

	if repo.Name() != filepath.Base(repo.workDir) || repo.GitDir() != repo.gitDir || repo.WorkDir() != repo.workDir || repo.IsBare() {
		t.Fatal("repository accessors returned unexpected values")
	}
	if remotes := repo.Remotes(); remotes["origin"] != "https://example.com/repo.git" {
		t.Fatalf("Repository.Remotes() = %+v", remotes)
	}

	empty := NewEmptyRepository()
	if empty.refs == nil || empty.commitMap == nil || empty.packReaders == nil {
		t.Fatal("NewEmptyRepository did not initialize maps")
	}

	loadRefsRepo := newRepoSkeleton(t)
	writeTextFile(t, filepath.Join(loadRefsRepo.gitDir, "refs", "heads", "main"), testHash4+"\n")
	if err := loadRefsRepo.loadRefs(); err != nil {
		t.Fatalf("loadRefs success: %v", err)
	}
	writeTextFile(t, filepath.Join(loadRefsRepo.gitDir, "HEAD"), "bad\n")
	if err := loadRefsRepo.loadRefs(); err == nil {
		t.Fatal("expected loadRefs HEAD failure")
	}
}

func TestRepositoryDiscoveryAndClose(t *testing.T) {
	bare := t.TempDir()
	for _, sub := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(bare, sub), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeTextFile(t, filepath.Join(bare, "HEAD"), "ref: refs/heads/main\n")

	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	for _, sub := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(gitDir, sub), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeTextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")

	if _, _, err := findGitDirectory(filepath.Join(t.TempDir(), "child")); err == nil {
		t.Fatal("expected missing repo from nonexistent child path")
	}
	child := filepath.Join(workDir, "child")
	if err := os.MkdirAll(child, 0o750); err != nil {
		t.Fatal(err)
	}
	if gotGit, gotWork, err := findGitDirectory(child); err != nil || gotGit != gitDir || gotWork != workDir {
		t.Fatalf("findGitDirectory child: %q %q %v", gotGit, gotWork, err)
	}
	if gotGit, gotWork, err := findGitDirectory(gitDir); err != nil || gotGit != gitDir || gotWork != workDir {
		t.Fatalf("findGitDirectory dotgit: %q %q %v", gotGit, gotWork, err)
	}
	if gotGit, gotWork, err := findGitDirectory(bare); err != nil || gotGit != bare || gotWork != bare {
		t.Fatalf("findGitDirectory bare: %q %q %v", gotGit, gotWork, err)
	}

	worktree := t.TempDir()
	linkedGit := filepath.Join(t.TempDir(), "actual.git")
	for _, sub := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(linkedGit, sub), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeTextFile(t, filepath.Join(linkedGit, "HEAD"), "ref: refs/heads/main\n")
	writeTextFile(t, filepath.Join(worktree, ".git"), "gitdir: ../"+filepath.Base(filepath.Dir(linkedGit))+"/"+filepath.Base(linkedGit)+"\n")
	if gotGit, gotWork, err := findGitDirectory(worktree); err != nil || gotGit != linkedGit || gotWork != worktree {
		t.Fatalf("findGitDirectory gitfile: %q %q %v", gotGit, gotWork, err)
	}
	writeTextFile(t, filepath.Join(worktree, ".git"), "bad\n")
	if _, _, err := handleGitFile(filepath.Join(worktree, ".git"), worktree); err == nil {
		t.Fatal("expected invalid .git file format")
	}
	if _, _, err := handleGitFile(filepath.Join(worktree, "missing"), worktree); err == nil {
		t.Fatal("expected .git file read error")
	}
	writeTextFile(t, filepath.Join(worktree, ".git"), "gitdir: missing\n")
	if _, _, err := handleGitFile(filepath.Join(worktree, ".git"), worktree); err == nil {
		t.Fatal("expected missing gitdir error")
	}
	writeTextFile(t, filepath.Join(worktree, ".git"), "gitdir: "+linkedGit+"\n")
	if gotGit, gotWork, err := handleGitFile(filepath.Join(worktree, ".git"), worktree); err != nil || gotGit != linkedGit || gotWork != worktree {
		t.Fatalf("handleGitFile absolute: %q %q %v", gotGit, gotWork, err)
	}

	if err := validateGitDirectory(linkedGit); err != nil {
		t.Fatalf("validateGitDirectory valid: %v", err)
	}
	if err := validateGitDirectory(filepath.Join(linkedGit, "HEAD")); err == nil {
		t.Fatal("expected non-directory validation error")
	}
	missing := t.TempDir()
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	if err := validateGitDirectory(missing); err == nil {
		t.Fatal("expected missing git directory error")
	}

	incomplete := t.TempDir()
	if err := os.MkdirAll(filepath.Join(incomplete, "objects"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := validateGitDirectory(incomplete); err == nil {
		t.Fatal("expected invalid git repository error")
	}

	if isBareRepository(workDir) {
		t.Fatal("worktree should not be bare")
	}
	if !isBareRepository(bare) {
		t.Fatal("bare repo should be bare")
	}
	if isBareRepository(filepath.Join(workDir, "missing")) {
		t.Fatal("missing path should not be bare")
	}

	repo, err := NewRepository(filepath.Join(filepath.Dir(workDir), filepath.Base(workDir), "child"))
	if err == nil {
		if closeErr := repo.Close(); closeErr != nil {
			t.Fatalf("close discovered repo: %v", closeErr)
		}
	}

	successRepoRoot := t.TempDir()
	successGitDir := filepath.Join(successRepoRoot, ".git")
	for _, dir := range []string{
		filepath.Join(successGitDir, "objects"),
		filepath.Join(successGitDir, "refs", "heads"),
		filepath.Join(successGitDir, "refs", "tags"),
	} {
		if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
	}
	writeTextFile(t, filepath.Join(successGitDir, "HEAD"), "ref: refs/heads/main\n")
	writeTextFile(t, filepath.Join(successGitDir, "refs", "heads", "main"), testHash4+"\n")
	successBlobHash := hashFromHex(testHash1)
	writeLooseObject(t, successGitDir, mustHash(t, testHash2), "tree", append([]byte("100644 file"), append([]byte{0}, successBlobHash[:]...)...))
	writeLooseObject(t, successGitDir, mustHash(t, testHash1), "blob", []byte("hello"))
	writeLooseObject(t, successGitDir, mustHash(t, testHash4), "commit", []byte("tree "+testHash2+"\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nmsg"))
	repo, err = NewRepository(successRepoRoot)
	if err != nil {
		t.Fatalf("NewRepository success: %v", err)
	}
	if repo.head != Hash(testHash4) || len(repo.commits) != 1 {
		t.Fatalf("unexpected NewRepository state: head=%v commits=%d", repo.head, len(repo.commits))
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatalf("close loaded repo: %v", closeErr)
	}

	if _, newRepoErr := NewRepository(filepath.Join(t.TempDir(), "missing")); newRepoErr == nil {
		t.Fatal("expected NewRepository missing path error")
	}

	badRepoRoot := t.TempDir()
	badGitDir := filepath.Join(badRepoRoot, ".git")
	if mkdirErr := os.MkdirAll(filepath.Join(badGitDir, "objects"), 0o750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if _, newRepoErr := NewRepository(badRepoRoot); newRepoErr == nil {
		t.Fatal("expected NewRepository invalid git dir error")
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "pack-*.pack")
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	repo = NewEmptyRepository()
	repo.packReaders[tmpFile.Name()] = &PackReader{file: tmpFile}
	if closeErr := repo.Close(); closeErr == nil {
		t.Fatal("expected Close to report closed file error")
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatalf("second Close should be a no-op: %v", closeErr)
	}

	noConfigRepo := &Repository{gitDir: t.TempDir()}
	if len(noConfigRepo.Remotes()) != 0 {
		t.Fatal("expected empty remotes when config is missing")
	}
}
