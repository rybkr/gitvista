package gitcore

import (
	"strings"
	"testing"
)

func TestLoadPackedRefsPreservesExistingRefsAndReportsParseErrors(t *testing.T) {
	repo := newRepoSkeleton(t)
	repo.refs["refs/heads/main"] = mustHash(t, testHash1)

	writeTextFile(t, repo.gitDir+"/packed-refs", strings.Join([]string{
		"# pack-refs with: peeled fully-peeled sorted",
		testHash2 + " refs/heads/main",
		testHash3 + " refs/tags/v1.0",
		"^" + testHash4,
		"invalid-line",
		"not-a-hash refs/heads/bad",
		"",
	}, "\n"))

	err := repo.loadPackedRefs()
	if err == nil {
		t.Fatal("expected parse errors from malformed packed-refs entries")
	}
	if repo.refs["refs/heads/main"] != mustHash(t, testHash1) {
		t.Fatal("expected packed refs to preserve existing loose ref value")
	}
	if repo.refs["refs/tags/v1.0"] != mustHash(t, testHash3) {
		t.Fatal("expected packed tag ref to load")
	}
	if !strings.Contains(err.Error(), "invalid packed-refs line") {
		t.Fatalf("expected malformed line error, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid packed ref hash") {
		t.Fatalf("expected malformed hash error, got %v", err)
	}
}

func TestLoadRefsLooseRefsOverridePackedRefs(t *testing.T) {
	repo := newRepoSkeleton(t)

	writeTextFile(t, repo.gitDir+"/packed-refs", testHash1+" refs/heads/main\n")
	writeTextFile(t, repo.gitDir+"/refs/heads/main", testHash2+"\n")
	writeTextFile(t, repo.gitDir+"/HEAD", "ref: refs/heads/main\n")

	if err := repo.loadRefs(); err != nil {
		t.Fatalf("loadRefs: %v", err)
	}

	if repo.refs["refs/heads/main"] != mustHash(t, testHash2) {
		t.Fatalf("expected loose ref to win, got %s", repo.refs["refs/heads/main"])
	}
	if repo.head != mustHash(t, testHash2) {
		t.Fatalf("expected HEAD to resolve to loose ref, got %s", repo.head)
	}
	if repo.headRef != "refs/heads/main" || repo.headDetached {
		t.Fatalf("unexpected HEAD state: ref=%q detached=%v", repo.headRef, repo.headDetached)
	}
}

func TestResolveRefDepthRejectsEscapesAndCycles(t *testing.T) {
	repo := newRepoSkeleton(t)

	writeTextFile(t, repo.gitDir+"/refs/heads/escape", "ref: ../outside\n")
	if _, err := repo.resolveRef(repo.gitDir + "/refs/heads/escape"); err == nil {
		t.Fatal("expected escaping symbolic ref to fail")
	} else if !strings.Contains(err.Error(), "path escapes base directory") {
		t.Fatalf("expected path escape error, got %v", err)
	}

	writeTextFile(t, repo.gitDir+"/refs/heads/a", "ref: refs/heads/b\n")
	writeTextFile(t, repo.gitDir+"/refs/heads/b", "ref: refs/heads/a\n")
	if _, err := repo.resolveRef(repo.gitDir + "/refs/heads/a"); err == nil {
		t.Fatal("expected symbolic ref cycle to fail")
	} else if !strings.Contains(err.Error(), "symbolic ref chain too deep") {
		t.Fatalf("expected cycle depth error, got %v", err)
	}
}

func TestLoadHEADDetachedAndUnbornBranch(t *testing.T) {
	repo := newRepoSkeleton(t)

	writeTextFile(t, repo.gitDir+"/HEAD", testHash4+"\n")
	if err := repo.loadHEAD(); err != nil {
		t.Fatalf("loadHEAD detached: %v", err)
	}
	if !repo.headDetached || repo.headRef != "" || repo.head != mustHash(t, testHash4) {
		t.Fatalf("unexpected detached HEAD state: detached=%v ref=%q head=%s", repo.headDetached, repo.headRef, repo.head)
	}

	writeTextFile(t, repo.gitDir+"/HEAD", "ref: refs/heads/new-branch\n")
	if err := repo.loadHEAD(); err != nil {
		t.Fatalf("loadHEAD unborn branch: %v", err)
	}
	if repo.headDetached || repo.headRef != "refs/heads/new-branch" || repo.head != "" {
		t.Fatalf("unexpected unborn HEAD state: detached=%v ref=%q head=%q", repo.headDetached, repo.headRef, repo.head)
	}
}
