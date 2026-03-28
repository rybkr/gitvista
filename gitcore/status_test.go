package gitcore

import (
	"crypto/sha1" // #nosec G505 -- test helper
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

func computeExpectedBlobHash(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	sum := sha1.Sum(append([]byte(header), content...)) // #nosec G401 -- test helper
	return fmt.Sprintf("%x", sum)
}

func TestHashBlobContentAndFlattenTree(t *testing.T) {
	content := []byte("hello world\n")
	if got := string(hashBlobContent(content)); got != computeExpectedBlobHash(content) {
		t.Fatalf("hashBlobContent() = %q, want %q", got, computeExpectedBlobHash(content))
	}

	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("nested"))
	inner := createTree(t, repo, []TreeEntry{{ID: blob, Name: "deep.txt", Mode: "100644", Type: ObjectTypeBlob}})
	root := createTree(t, repo, []TreeEntry{{ID: inner, Name: "dir", Mode: "040000", Type: ObjectTypeTree}})

	got, err := flattenTree(repo, root, "")
	if err != nil {
		t.Fatalf("flattenTree() error = %v", err)
	}
	if got["dir/deep.txt"].Hash != blob {
		t.Fatalf("flattenTree()[dir/deep.txt].Hash = %s, want %s", got["dir/deep.txt"].Hash, blob)
	}
	if got["dir/deep.txt"].Mode != "100644" {
		t.Fatalf("flattenTree()[dir/deep.txt].Mode = %q, want %q", got["dir/deep.txt"].Mode, "100644")
	}
}

type indexEntrySpec struct {
	path     string
	blobHash Hash
	fileSize uint32
	mode     uint32
}

func writeIndexWithEntries(t *testing.T, gitDir string, entries []indexEntrySpec) {
	t.Helper()
	data := buildIndexHeader(uint32(len(entries)))
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o100644
		}
		hash := hashFromHex(string(entry.blobHash))
		start := len(data)
		data = append(data, buildIndexEntryWithStats(entry.path, hash, mode, 0, 1, 0, 1, 0)...)
		binary.BigEndian.PutUint32(data[start+36:start+40], entry.fileSize)
	}
	writeIndexFile(t, gitDir, data)
}

func statusByPath(t *testing.T, status *WorkingTreeStatus) map[string]FileState {
	t.Helper()
	m := make(map[string]FileState, len(status.Files))
	for _, file := range status.Files {
		m[file.Path] = file
	}
	return m
}

type stubFileInfo struct {
	name string
	mode fs.FileMode
}

func (s stubFileInfo) Name() string       { return s.name }
func (s stubFileInfo) Size() int64        { return 0 }
func (s stubFileInfo) Mode() fs.FileMode  { return s.mode }
func (s stubFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (s stubFileInfo) IsDir() bool        { return s.mode.IsDir() }
func (s stubFileInfo) Sys() any           { return nil }

func TestComputeWorkingTreeStatus_BasicScenarios(t *testing.T) {
	repo := setupTestRepo(t)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus(empty) error = %v", err)
	}
	if len(status.Files) != 0 {
		t.Fatalf("len(status.Files) = %d, want 0", len(status.Files))
	}

	headBlob := createBlob(t, repo, []byte("old\n"))
	root := createTree(t, repo, []TreeEntry{{ID: headBlob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: headBlob, fileSize: uint32(len("old\n"))},
		{path: "staged.txt", blobHash: createBlob(t, repo, []byte("staged\n")), fileSize: uint32(len("staged\n"))},
	})
	writeDiskFile(t, repo, "tracked.txt", []byte("modified\n"))
	writeDiskFile(t, repo, "untracked.txt", []byte("new\n"))

	status, err = ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["staged.txt"].StagedChange != ChangeTypeAdded {
		t.Fatalf("staged.txt StagedChange = %q, want %q", got["staged.txt"].StagedChange, ChangeTypeAdded)
	}
	if got["tracked.txt"].UnstagedChange != ChangeTypeModified {
		t.Fatalf("tracked.txt UnstagedChange = %q, want %q", got["tracked.txt"].UnstagedChange, ChangeTypeModified)
	}
	if !got["untracked.txt"].IsUntracked {
		t.Fatal("untracked.txt should be untracked")
	}
}

func TestChangeTypeJSON(t *testing.T) {
	data, err := json.Marshal(ChangeTypeModified)
	if err != nil {
		t.Fatalf("json.Marshal(ChangeTypeModified) error = %v", err)
	}
	if got := string(data); got != `"modified"` {
		t.Fatalf("json.Marshal(ChangeTypeModified) = %s, want %q", data, `"modified"`)
	}

	var changeType ChangeType
	if err := json.Unmarshal([]byte(`"deleted"`), &changeType); err != nil {
		t.Fatalf("json.Unmarshal(ChangeType) error = %v", err)
	}
	if changeType != ChangeTypeDeleted {
		t.Fatalf("json.Unmarshal(ChangeType) = %v, want %v", changeType, ChangeTypeDeleted)
	}
}

func TestChangeTypeJSONAndStringErrors(t *testing.T) {
	if got := ChangeType(99).String(); got != "unknown" {
		t.Fatalf("ChangeType(99).String() = %q, want %q", got, "unknown")
	}
	if _, err := json.Marshal(ChangeType(99)); err == nil {
		t.Fatal("expected invalid ChangeType marshal error")
	}

	var changeType ChangeType
	if err := json.Unmarshal([]byte(`123`), &changeType); err == nil {
		t.Fatal("expected non-string ChangeType unmarshal error")
	}
	if err := json.Unmarshal([]byte(`"bogus"`), &changeType); err == nil {
		t.Fatal("expected invalid ChangeType unmarshal error")
	}
}

func TestStatusModeHelpers(t *testing.T) {
	if got := entryModeKind("000000"); got != "000000" {
		t.Fatalf("entryModeKind(unknown) = %q, want %q", got, "000000")
	}
	if got := normalizeTreeMode("40000"); got != "040000" {
		t.Fatalf("normalizeTreeMode(40000) = %q, want %q", got, "040000")
	}
	if got := normalizeTreeMode("100644"); got != "100644" {
		t.Fatalf("normalizeTreeMode(default) = %q, want %q", got, "100644")
	}
	if got := indexModeString(0); got != "000000" {
		t.Fatalf("indexModeString(0) = %q, want %q", got, "000000")
	}
	if got := indexModeString(0o040000); got != "040000" {
		t.Fatalf("indexModeString(dir) = %q, want %q", got, "040000")
	}
	if got := worktreeModeString(stubFileInfo{name: "dir", mode: fs.ModeDir | 0o755}); got != "040000" {
		t.Fatalf("worktreeModeString(dir) = %q, want %q", got, "040000")
	}
	if got := worktreeModeString(stubFileInfo{name: "pipe", mode: fs.ModeNamedPipe}); got != fs.ModeNamedPipe.String() {
		t.Fatalf("worktreeModeString(pipe) = %q, want %q", got, fs.ModeNamedPipe.String())
	}
}

func TestComputeWorkingTreeStatus_StagedDeletionAndNoChanges(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("same\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "gone.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, nil)

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["gone.txt"].StagedChange != ChangeTypeDeleted {
		t.Fatalf("gone.txt StagedChange = %q, want %q", got["gone.txt"].StagedChange, ChangeTypeDeleted)
	}

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{{path: "gone.txt", blobHash: blob, fileSize: uint32(len("same\n"))}})
	writeDiskFile(t, repo, "gone.txt", []byte("same\n"))

	status, err = ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus(no changes) error = %v", err)
	}
	if len(status.Files) != 0 {
		paths := make([]string, 0, len(status.Files))
		for _, file := range status.Files {
			paths = append(paths, file.Path)
		}
		sort.Strings(paths)
		t.Fatalf("expected no changes, got %v", paths)
	}
}

func TestComputeWorkingTreeStatus_GitignoreExcludedUntracked(t *testing.T) {
	repo := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo.workDir, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}
	writeDiskFile(t, repo, "ignored.log", []byte("ignore\n"))
	writeDiskFile(t, repo, "visible.txt", []byte("keep\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if _, ok := got["ignored.log"]; ok {
		t.Fatal("ignored.log should not appear in status")
	}
	if !got["visible.txt"].IsUntracked {
		t.Fatal("visible.txt should be untracked")
	}
}

func TestComputeWorkingTreeStatus_TrackedRegularFileReplacedBySymlink(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("tracked\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("tracked\n"))},
	})

	if err := os.Symlink(filepath.Join("..", "missing-target.txt"), filepath.Join(repo.workDir, "tracked.txt")); err != nil {
		t.Fatalf("Symlink(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["tracked.txt"].UnstagedChange != ChangeTypeTypeChanged {
		t.Fatalf("tracked.txt UnstagedChange = %q, want %q", got["tracked.txt"].UnstagedChange, ChangeTypeTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_TrackedSymlinkReplacedByRegularFile(t *testing.T) {
	repo := setupTestRepo(t)
	linkHash := Hash(computeExpectedBlobHash([]byte("target.txt")))

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "link.txt", blobHash: linkHash, fileSize: uint32(len("target.txt")), mode: 0o120000},
	})
	writeDiskFile(t, repo, "link.txt", []byte("plain file\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["link.txt"].UnstagedChange != ChangeTypeTypeChanged {
		t.Fatalf("link.txt UnstagedChange = %q, want %q", got["link.txt"].UnstagedChange, ChangeTypeTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_TrackedFileReplacedByDirectory(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("tracked\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("tracked\n"))},
	})

	if err := os.Mkdir(filepath.Join(repo.workDir, "tracked.txt"), 0o755); err != nil {
		t.Fatalf("Mkdir(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)
	if got["tracked.txt"].UnstagedChange != ChangeTypeTypeChanged {
		t.Fatalf("tracked.txt UnstagedChange = %q, want %q", got["tracked.txt"].UnstagedChange, ChangeTypeTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_PropagatesWalkErrors(t *testing.T) {
	repo := setupTestRepo(t)

	originalWalk := walkWorktree
	t.Cleanup(func() {
		walkWorktree = originalWalk
	})

	walkWorktree = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "blocked"), nil, fs.ErrPermission)
	}

	if _, err := ComputeWorkingTreeStatus(repo); err == nil {
		t.Fatal("ComputeWorkingTreeStatus() error = nil, want walk error")
	}
}

func TestComputeWorkingTreeStatus_HeadTreeAndIndexErrors(t *testing.T) {
	t.Run("invalid index", func(t *testing.T) {
		repo := setupTestRepo(t)
		if err := os.WriteFile(filepath.Join(repo.gitDir, "index"), []byte("bad"), 0o644); err != nil {
			t.Fatalf("WriteFile(index): %v", err)
		}
		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "reading index") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want index error", err)
		}
	})

	t.Run("invalid head tree", func(t *testing.T) {
		repo := setupTestRepo(t)
		head := mustHash(t, testHash1)
		repo.head = head
		repo.commitMap[head] = &Commit{ID: head, Tree: Hash("abc")}
		writeIndexWithEntries(t, repo.gitDir, nil)

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "flattening HEAD tree") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want flatten error", err)
		}
	})

	t.Run("invalid relative index path", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("bad\n"))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "../bad.txt", blobHash: blob, fileSize: uint32(len("bad\n"))},
		})

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "invalid index path") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want invalid index path error", err)
		}
	})

	t.Run("invalid absolute index path", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("abs\n"))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "/abs.txt", blobHash: blob, fileSize: uint32(len("abs\n"))},
		})

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "invalid index path") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want invalid index path error", err)
		}
	})

	t.Run("invalid worktree root", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("bad\n"))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "file.txt", blobHash: blob, fileSize: uint32(len("bad\n"))},
		})
		originalAbs := worktreePathAbs
		worktreePathAbs = func(string) (string, error) {
			return "", fs.ErrInvalid
		}
		t.Cleanup(func() { worktreePathAbs = originalAbs })

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "invalid worktree path") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want invalid worktree path error", err)
		}
	})
}

func TestComputeWorkingTreeStatus_ReturnsFilesSortedByPath(t *testing.T) {
	repo := setupTestRepo(t)

	writeDiskFile(t, repo, "z-last.txt", []byte("z\n"))
	writeDiskFile(t, repo, "a-first.txt", []byte("a\n"))
	writeDiskFile(t, repo, "m-middle.txt", []byte("m\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}

	got := make([]string, 0, len(status.Files))
	for _, file := range status.Files {
		got = append(got, file.Path)
	}

	want := []string{"a-first.txt", "m-middle.txt", "z-last.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("status file order = %v, want %v", got, want)
	}
}

func TestComputeWorkingTreeStatus_GitlinkAndUntrackedOrdering(t *testing.T) {
	t.Run("gitlink replaced by file", func(t *testing.T) {
		repo := setupTestRepo(t)
		submoduleCommit := Hash(strings.Repeat("b", 40))
		root := createTree(t, repo, []TreeEntry{{ID: submoduleCommit, Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})
		wireHeadCommit(repo, root)

		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "mod", blobHash: submoduleCommit, fileSize: 0, mode: 0o160000},
		})
		writeDiskFile(t, repo, "mod", []byte("not a directory\n"))

		status, err := ComputeWorkingTreeStatus(repo)
		if err != nil {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
		}
		got := statusByPath(t, status)
		if got["mod"].UnstagedChange != ChangeTypeTypeChanged {
			t.Fatalf("mod UnstagedChange = %q, want %q", got["mod"].UnstagedChange, ChangeTypeTypeChanged)
		}
	})

	t.Run("tracked before untracked", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("old\n"))
		root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
		wireHeadCommit(repo, root)
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("old\n"))}})
		writeDiskFile(t, repo, "tracked.txt", []byte("new\n"))
		writeDiskFile(t, repo, "zzz.txt", []byte("untracked\n"))

		status, err := ComputeWorkingTreeStatus(repo)
		if err != nil {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
		}
		if len(status.Files) != 2 || status.Files[0].Path != "tracked.txt" || status.Files[1].Path != "zzz.txt" {
			t.Fatalf("status ordering = %#v", status.Files)
		}
	})
}

func TestComputeWorkingTreeStatus_AdditionalErrorBranches(t *testing.T) {
	t.Run("missing file without existing staged entry", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("gone\n"))
		root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "gone.txt", Mode: "100644", Type: ObjectTypeBlob}})
		wireHeadCommit(repo, root)
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "gone.txt", blobHash: blob, fileSize: uint32(len("gone\n"))},
		})

		status, err := ComputeWorkingTreeStatus(repo)
		if err != nil {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
		}
		got := statusByPath(t, status)
		if got["gone.txt"].UnstagedChange != ChangeTypeDeleted {
			t.Fatalf("gone.txt UnstagedChange = %q, want %q", got["gone.txt"].UnstagedChange, ChangeTypeDeleted)
		}
	})

	t.Run("stat error", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("x\n"))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "file.txt", blobHash: blob, fileSize: uint32(len("x\n"))},
		})

		originalLstat := statusLstat
		statusLstat = func(string) (fs.FileInfo, error) {
			return nil, fs.ErrPermission
		}
		t.Cleanup(func() { statusLstat = originalLstat })

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "stat") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want stat error", err)
		}
	})

	t.Run("readlink error", func(t *testing.T) {
		repo := setupTestRepo(t)
		linkHash := Hash(computeExpectedBlobHash([]byte("target.txt")))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "link.txt", blobHash: linkHash, fileSize: uint32(len("target.txt")), mode: 0o120000},
		})
		if err := os.Symlink("target.txt", filepath.Join(repo.workDir, "link.txt")); err != nil {
			t.Fatalf("Symlink(link.txt): %v", err)
		}

		originalReadlink := statusReadlink
		statusReadlink = func(string) (string, error) {
			return "", fs.ErrPermission
		}
		t.Cleanup(func() { statusReadlink = originalReadlink })

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "readlink") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want readlink error", err)
		}
	})

	t.Run("symlink target modified", func(t *testing.T) {
		repo := setupTestRepo(t)
		linkHash := Hash(computeExpectedBlobHash([]byte("target.txt")))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "link.txt", blobHash: linkHash, fileSize: uint32(len("target.txt")), mode: 0o120000},
		})
		if err := os.Symlink("other.txt", filepath.Join(repo.workDir, "link.txt")); err != nil {
			t.Fatalf("Symlink(link.txt): %v", err)
		}

		status, err := ComputeWorkingTreeStatus(repo)
		if err != nil {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
		}
		got := statusByPath(t, status)
		if got["link.txt"].UnstagedChange != ChangeTypeModified || got["link.txt"].WorktreeHash == "" {
			t.Fatalf("link.txt status = %+v", got["link.txt"])
		}
	})

	t.Run("non regular without type change", func(t *testing.T) {
		repo := setupTestRepo(t)
		hash := mustHash(t, testHash1)
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "dir", blobHash: hash, fileSize: 0, mode: 0o040000},
		})
		if err := os.Mkdir(filepath.Join(repo.workDir, "dir"), 0o755); err != nil {
			t.Fatalf("Mkdir(dir): %v", err)
		}

		status, err := ComputeWorkingTreeStatus(repo)
		if err != nil {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
		}
		got := statusByPath(t, status)
		if got["dir"].UnstagedChange != ChangeTypeModified {
			t.Fatalf("dir UnstagedChange = %q, want %q", got["dir"].UnstagedChange, ChangeTypeModified)
		}
	})

	t.Run("size mismatch read error", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("x\n"))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "file.txt", blobHash: blob, fileSize: 999},
		})
		writeDiskFile(t, repo, "file.txt", []byte("x\n"))

		originalReadFile := statusReadWorktreeFile
		statusReadWorktreeFile = func(string, string) ([]byte, error) {
			return nil, fs.ErrPermission
		}
		t.Cleanup(func() { statusReadWorktreeFile = originalReadFile })

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "reading") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want read error", err)
		}
	})

	t.Run("regular read error", func(t *testing.T) {
		repo := setupTestRepo(t)
		blob := createBlob(t, repo, []byte("x\n"))
		writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
			{path: "file.txt", blobHash: blob, fileSize: uint32(len("x\n"))},
		})
		writeDiskFile(t, repo, "file.txt", []byte("x\n"))

		originalReadFile := statusReadWorktreeFile
		statusReadWorktreeFile = func(string, string) ([]byte, error) {
			return nil, fs.ErrPermission
		}
		t.Cleanup(func() { statusReadWorktreeFile = originalReadFile })

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "reading") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want read error", err)
		}
	})

	t.Run("walk rel error", func(t *testing.T) {
		repo := setupTestRepo(t)
		writeIndexWithEntries(t, repo.gitDir, nil)

		originalRel := filepathRel
		filepathRel = func(string, string) (string, error) {
			return "", fs.ErrPermission
		}
		t.Cleanup(func() { filepathRel = originalRel })

		if _, err := ComputeWorkingTreeStatus(repo); err == nil || !strings.Contains(err.Error(), "walking work dir") {
			t.Fatalf("ComputeWorkingTreeStatus() error = %v, want walk rel error", err)
		}
	})
}

func TestComputeWorkingTreeStatus_StagedModeOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("#!/bin/sh\necho hi\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "script.sh", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "script.sh", blobHash: blob, fileSize: uint32(len("#!/bin/sh\necho hi\n")), mode: 0o100755},
	})
	writeDiskFile(t, repo, "script.sh", []byte("#!/bin/sh\necho hi\n"))
	if err := os.Chmod(filepath.Join(repo.workDir, "script.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(script.sh): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["script.sh"].StagedChange != ChangeTypeModified {
		t.Fatalf("script.sh StagedChange = %q, want %q", got["script.sh"].StagedChange, ChangeTypeModified)
	}
}

func TestComputeWorkingTreeStatus_WorktreeModeOnlyChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("#!/bin/sh\necho hi\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "script.sh", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "script.sh", blobHash: blob, fileSize: uint32(len("#!/bin/sh\necho hi\n")), mode: 0o100644},
	})
	writeDiskFile(t, repo, "script.sh", []byte("#!/bin/sh\necho hi\n"))
	if err := os.Chmod(filepath.Join(repo.workDir, "script.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(script.sh): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["script.sh"].UnstagedChange != ChangeTypeModified {
		t.Fatalf("script.sh UnstagedChange = %q, want %q", got["script.sh"].UnstagedChange, ChangeTypeModified)
	}
}

func TestComputeWorkingTreeStatus_StagedTypeChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("replace me\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	linkHash := Hash(computeExpectedBlobHash([]byte("target.txt")))
	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: linkHash, fileSize: uint32(len("target.txt")), mode: 0o120000},
	})
	if err := os.Symlink("target.txt", filepath.Join(repo.workDir, "tracked.txt")); err != nil {
		t.Fatalf("Symlink(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["tracked.txt"].StagedChange != ChangeTypeTypeChanged {
		t.Fatalf("tracked.txt StagedChange = %q, want %q", got["tracked.txt"].StagedChange, ChangeTypeTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_WorktreeTypeChange(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("replace me\n"))
	root := createTree(t, repo, []TreeEntry{{ID: blob, Name: "tracked.txt", Mode: "100644", Type: ObjectTypeBlob}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "tracked.txt", blobHash: blob, fileSize: uint32(len("replace me\n")), mode: 0o100644},
	})
	if err := os.Symlink("target.txt", filepath.Join(repo.workDir, "tracked.txt")); err != nil {
		t.Fatalf("Symlink(tracked.txt): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["tracked.txt"].UnstagedChange != ChangeTypeTypeChanged {
		t.Fatalf("tracked.txt UnstagedChange = %q, want %q", got["tracked.txt"].UnstagedChange, ChangeTypeTypeChanged)
	}
}

func TestComputeWorkingTreeStatus_CollapsesUntrackedDirectories(t *testing.T) {
	repo := setupTestRepo(t)

	writeDiskFile(t, repo, "nested/untracked-a.txt", []byte("a\n"))
	writeDiskFile(t, repo, "nested/untracked-b.txt", []byte("b\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if !got["nested/"].IsUntracked {
		t.Fatalf("nested/ IsUntracked = %v, want true", got["nested/"].IsUntracked)
	}
	if _, ok := got["nested/untracked-a.txt"]; ok {
		t.Fatal("nested/untracked-a.txt should be collapsed into nested/")
	}
	if _, ok := got["nested/untracked-b.txt"]; ok {
		t.Fatal("nested/untracked-b.txt should be collapsed into nested/")
	}
}

func TestComputeWorkingTreeStatus_TrackedFileUnderIgnoredDirectoryIsReported(t *testing.T) {
	repo := setupTestRepo(t)
	blob := createBlob(t, repo, []byte("tracked\n"))
	buildTree := createTree(t, repo, []TreeEntry{{ID: blob, Name: "keep.txt", Mode: "100644", Type: ObjectTypeBlob}})
	root := createTree(t, repo, []TreeEntry{{ID: buildTree, Name: "build", Mode: "040000", Type: ObjectTypeTree}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "build/keep.txt", blobHash: blob, fileSize: uint32(len("tracked\n"))},
	})
	if err := os.WriteFile(filepath.Join(repo.workDir, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}
	writeDiskFile(t, repo, "build/keep.txt", []byte("changed\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if got["build/keep.txt"].UnstagedChange != ChangeTypeModified {
		t.Fatalf("build/keep.txt UnstagedChange = %q, want %q", got["build/keep.txt"].UnstagedChange, ChangeTypeModified)
	}
}

func TestComputeWorkingTreeStatus_IgnoredDirectoryDoesNotLoadNestedGitignore(t *testing.T) {
	repo := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo.workDir, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo.workDir, "ignored", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll(ignored/sub): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo.workDir, "ignored", ".gitignore"), []byte("!keep.txt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(ignored/.gitignore): %v", err)
	}
	writeDiskFile(t, repo, "ignored/sub/keep.txt", []byte("keep\n"))

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	got := statusByPath(t, status)

	if _, ok := got["ignored/sub/keep.txt"]; ok {
		t.Fatal("ignored/sub/keep.txt should remain ignored")
	}
	if _, ok := got["ignored/"]; ok {
		t.Fatal("ignored/ should not appear in status")
	}
}

func TestComputeWorkingTreeStatus_CleanSubmoduleIsNotReportedModified(t *testing.T) {
	repo := setupTestRepo(t)
	submoduleCommit := Hash(strings.Repeat("b", 40))
	root := createTree(t, repo, []TreeEntry{{ID: submoduleCommit, Name: "mod", Mode: "160000", Type: ObjectTypeCommit}})
	wireHeadCommit(repo, root)

	writeIndexWithEntries(t, repo.gitDir, []indexEntrySpec{
		{path: "mod", blobHash: submoduleCommit, fileSize: 0, mode: 0o160000},
	})
	if err := os.Mkdir(filepath.Join(repo.workDir, "mod"), 0o755); err != nil {
		t.Fatalf("Mkdir(mod): %v", err)
	}

	status, err := ComputeWorkingTreeStatus(repo)
	if err != nil {
		t.Fatalf("ComputeWorkingTreeStatus() error = %v", err)
	}
	if len(status.Files) != 0 {
		t.Fatalf("expected clean submodule to be omitted from status, got %#v", status.Files)
	}
}

func TestFlattenTreeErrorBranches(t *testing.T) {
	repo := setupTestRepo(t)

	if _, err := flattenTree(repo, Hash("abc"), ""); err == nil || !strings.Contains(err.Error(), "flattenTree: reading tree") {
		t.Fatalf("flattenTree(invalid root) error = %v", err)
	}

	badSubtree := mustHash(t, testHash1)
	root := createTree(t, repo, []TreeEntry{{ID: badSubtree, Name: "dir", Mode: "040000", Type: ObjectTypeTree}})
	if _, err := flattenTree(repo, root, ""); err == nil || !strings.Contains(err.Error(), "flattenTree: reading tree") {
		t.Fatalf("flattenTree(invalid subtree) error = %v", err)
	}
}
