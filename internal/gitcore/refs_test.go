package gitcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRefs_LooseRefOverridesPackedRef(t *testing.T) {
	gitDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs/heads: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/dev\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	const packedDev = "1111111111111111111111111111111111111111"
	const looseDev = "2222222222222222222222222222222222222222"

	packedRefs := "# pack-refs with: peeled fully-peeled sorted\n" +
		packedDev + " refs/heads/dev\n"
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(packedRefs), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "dev"), []byte(looseDev+"\n"), 0o644); err != nil {
		t.Fatalf("write refs/heads/dev: %v", err)
	}

	repo := &Repository{
		gitDir: gitDir,
		refs:   make(map[string]Hash),
	}

	if err := repo.loadRefs(); err != nil {
		t.Fatalf("loadRefs() error: %v", err)
	}

	if got := repo.refs["refs/heads/dev"]; got != Hash(looseDev) {
		t.Fatalf("refs[refs/heads/dev] = %s, want %s", got, looseDev)
	}
	if got := repo.Head(); got != Hash(looseDev) {
		t.Fatalf("Head() = %s, want %s", got, looseDev)
	}
	if got := repo.HeadRef(); got != "refs/heads/dev" {
		t.Fatalf("HeadRef() = %q, want %q", got, "refs/heads/dev")
	}
}

func TestLoadRefs_LooseRemoteRefOverridesPackedRef(t *testing.T) {
	gitDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs/heads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir refs/remotes/origin: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/dev\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	const headDev = "3333333333333333333333333333333333333333"
	const packedRemote = "1111111111111111111111111111111111111111"
	const looseRemote = "2222222222222222222222222222222222222222"

	packedRefs := "# pack-refs with: peeled fully-peeled sorted\n" +
		headDev + " refs/heads/dev\n" +
		packedRemote + " refs/remotes/origin/dev\n"
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(packedRefs), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "dev"), []byte(headDev+"\n"), 0o644); err != nil {
		t.Fatalf("write refs/heads/dev: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "remotes", "origin", "dev"), []byte(looseRemote+"\n"), 0o644); err != nil {
		t.Fatalf("write refs/remotes/origin/dev: %v", err)
	}

	repo := &Repository{
		gitDir: gitDir,
		refs:   make(map[string]Hash),
	}

	if err := repo.loadRefs(); err != nil {
		t.Fatalf("loadRefs() error: %v", err)
	}

	if got := repo.refs["refs/remotes/origin/dev"]; got != Hash(looseRemote) {
		t.Fatalf("refs[refs/remotes/origin/dev] = %s, want %s", got, looseRemote)
	}
}

func TestLoadRefs_SymbolicRemoteHeadResolvesPackedTarget(t *testing.T) {
	gitDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs/heads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir refs/remotes/origin: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/dev\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	const headDev = "3333333333333333333333333333333333333333"
	const packedMain = "1111111111111111111111111111111111111111"

	packedRefs := "# pack-refs with: peeled fully-peeled sorted\n" +
		headDev + " refs/heads/dev\n" +
		packedMain + " refs/remotes/origin/main\n"
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(packedRefs), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "dev"), []byte(headDev+"\n"), 0o644); err != nil {
		t.Fatalf("write refs/heads/dev: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"), []byte("ref: refs/remotes/origin/main\n"), 0o644); err != nil {
		t.Fatalf("write refs/remotes/origin/HEAD: %v", err)
	}

	repo := &Repository{
		gitDir: gitDir,
		refs:   make(map[string]Hash),
	}

	if err := repo.loadRefs(); err != nil {
		t.Fatalf("loadRefs() error: %v", err)
	}

	if got := repo.refs["refs/remotes/origin/HEAD"]; got != Hash(packedMain) {
		t.Fatalf("refs[refs/remotes/origin/HEAD] = %s, want %s", got, packedMain)
	}
}

func TestResolveRef_RejectsPathTraversal(t *testing.T) {
	gitDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs/heads: %v", err)
	}

	refPath := filepath.Join(gitDir, "refs", "heads", "main")
	if err := os.WriteFile(refPath, []byte("ref: ../outside\n"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	repo := &Repository{gitDir: gitDir}
	if _, err := repo.resolveRef(refPath); err == nil {
		t.Fatal("expected resolveRef to reject ref path traversal")
	}
}
