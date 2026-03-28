package gitcore

import (
	"os"
	"path/filepath"
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

func TestLoadRefsWrapsStageErrors(t *testing.T) {
	t.Run("packed refs", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "packed-refs"), "bad line\n")
		if err := repo.loadRefs(); err == nil || !strings.Contains(err.Error(), "failed to load packed refs") {
			t.Fatalf("loadRefs() error = %v, want packed refs wrapper", err)
		}
	})

	t.Run("heads", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "bad"), "nope\n")
		if err := repo.loadRefs(); err == nil || !strings.Contains(err.Error(), "failed to load loose branches") {
			t.Fatalf("loadRefs() error = %v, want heads wrapper", err)
		}
	})

	t.Run("remotes", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "main"), testHash1+"\n")
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "remotes", "origin", "bad"), "nope\n")
		if err := repo.loadRefs(); err == nil || !strings.Contains(err.Error(), "failed to load loose remote-tracking refs") {
			t.Fatalf("loadRefs() error = %v, want remotes wrapper", err)
		}
	})

	t.Run("tags", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "main"), testHash1+"\n")
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "tags", "bad"), "nope\n")
		if err := repo.loadRefs(); err == nil || !strings.Contains(err.Error(), "failed to load loose tags") {
			t.Fatalf("loadRefs() error = %v, want tags wrapper", err)
		}
	})
}

func TestLoadLooseRefsAndPackedRefsIOErrors(t *testing.T) {
	t.Run("loadLooseRefs missing dir", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		if err := repo.loadLooseRefs("missing"); err != nil {
			t.Fatalf("loadLooseRefs(missing) = %v, want nil", err)
		}
	})

	t.Run("loadLooseRefs stat error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		repo.gitDir = filepath.Join(repo.gitDir, "HEAD")
		if err := repo.loadLooseRefs("heads"); err == nil {
			t.Fatal("expected loadLooseRefs stat error")
		}
	})

	t.Run("loadLooseRefs walk error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		blockedDir := filepath.Join(repo.gitDir, "refs", "heads", "blocked")
		if err := os.MkdirAll(filepath.Join(blockedDir, "child"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(blockedDir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(blockedDir, 0o750) })

		if err := repo.loadLooseRefs("heads"); err == nil {
			t.Fatal("expected loadLooseRefs walk error")
		}
	})

	t.Run("loadLooseRefs rel error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "main"), testHash1+"\n")

		originalRel := filepathRel
		filepathRel = func(basepath, targpath string) (string, error) {
			return "", os.ErrInvalid
		}
		t.Cleanup(func() { filepathRel = originalRel })

		if err := repo.loadLooseRefs("heads"); err == nil {
			t.Fatal("expected loadLooseRefs rel error")
		}
	})

	t.Run("loadPackedRefs open error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		repo.gitDir = filepath.Join(repo.gitDir, "HEAD")
		if err := repo.loadPackedRefs(); err == nil {
			t.Fatal("expected loadPackedRefs open error")
		}
	})

	t.Run("loadPackedRefs scanner error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "packed-refs"), strings.Repeat("a", 70_000))
		if err := repo.loadPackedRefs(); err == nil || !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("loadPackedRefs() error = %v, want scanner error", err)
		}
	})
}

func TestLoadHEADReadError(t *testing.T) {
	repo := newRepoSkeleton(t)
	if err := os.Remove(filepath.Join(repo.gitDir, "HEAD")); err != nil {
		t.Fatal(err)
	}
	if err := repo.loadHEAD(); err == nil || !strings.Contains(err.Error(), "failed to read HEAD") {
		t.Fatalf("loadHEAD() error = %v, want read error", err)
	}
}

func TestLoadHEADInvalidDetachedHash(t *testing.T) {
	repo := newRepoSkeleton(t)
	writeTextFile(t, filepath.Join(repo.gitDir, "HEAD"), "bad\n")
	if err := repo.loadHEAD(); err == nil || !strings.Contains(err.Error(), "invalid HEAD") {
		t.Fatalf("loadHEAD() error = %v, want invalid HEAD error", err)
	}
}

func TestLoadStashesErrorPaths(t *testing.T) {
	t.Run("missing stash", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		if err := repo.loadStashes(); err != nil {
			t.Fatalf("loadStashes() = %v, want nil", err)
		}
	})

	t.Run("stat error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		repo.gitDir = filepath.Join(repo.gitDir, "HEAD")
		if err := repo.loadStashes(); err == nil {
			t.Fatal("expected loadStashes stat error")
		}
	})

	t.Run("fallback read error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		stashRefPath := filepath.Join(repo.gitDir, "refs", "stash")
		if err := os.MkdirAll(stashRefPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := repo.loadStashes(); err == nil || !strings.Contains(err.Error(), "reading stash ref fallback") {
			t.Fatalf("loadStashes() error = %v, want fallback read error", err)
		}
	})

	t.Run("fallback parse error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "stash"), "bad\n")
		if err := repo.loadStashes(); err == nil || !strings.Contains(err.Error(), "parsing stash ref fallback") {
			t.Fatalf("loadStashes() error = %v, want fallback parse error", err)
		}
	})

	t.Run("fallback success", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "stash"), testHash2+"\n")
		if err := repo.loadStashes(); err != nil {
			t.Fatalf("loadStashes() = %v, want nil", err)
		}
		if len(repo.stashes) != 1 || repo.stashes[0].Hash != mustHash(t, testHash2) || repo.stashes[0].Message != "stash@{0}" {
			t.Fatalf("unexpected fallback stashes: %+v", repo.stashes)
		}
	})

	t.Run("scanner error", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "stash"), testHash1+"\n")
		writeTextFile(t, filepath.Join(repo.gitDir, "logs", "refs", "stash"), strings.Repeat("a", 70_000))
		if err := repo.loadStashes(); err == nil || !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("loadStashes() error = %v, want scanner error", err)
		}
	})

	t.Run("invalid reflog hash and blank lines", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "stash"), testHash4+"\n")
		writeTextFile(t, filepath.Join(repo.gitDir, "logs", "refs", "stash"), strings.Join([]string{
			"",
			"broken",
			testHash1 + " bad a <a@b> 1 +0000\tbad",
			testHash2 + " " + testHash3 + " a <a@b> 1 +0000",
		}, "\n"))
		err := repo.loadStashes()
		if err == nil || !strings.Contains(err.Error(), "invalid stash reflog line") || !strings.Contains(err.Error(), "invalid stash reflog hash") {
			t.Fatalf("loadStashes() error = %v, want invalid line and hash errors", err)
		}
		if len(repo.stashes) != 1 || repo.stashes[0].Hash != mustHash(t, testHash3) || repo.stashes[0].Message != "stash@{0}" {
			t.Fatalf("unexpected parsed stashes: %+v", repo.stashes)
		}
	})

	t.Run("reflog tab message", func(t *testing.T) {
		repo := newRepoSkeleton(t)
		writeTextFile(t, filepath.Join(repo.gitDir, "refs", "stash"), testHash4+"\n")
		writeTextFile(t, filepath.Join(repo.gitDir, "logs", "refs", "stash"), testHash1+" "+testHash3+" a <a@b> 1 +0000\tmessage from tab\n")
		if err := repo.loadStashes(); err != nil {
			t.Fatalf("loadStashes() = %v, want nil", err)
		}
		if len(repo.stashes) != 1 || repo.stashes[0].Message != "message from tab" {
			t.Fatalf("unexpected stash messages: %+v", repo.stashes)
		}
	})
}

func TestResolveRefDepthReadAndParseErrors(t *testing.T) {
	repo := newRepoSkeleton(t)

	if _, err := repo.resolveRef(filepath.Join(repo.gitDir, "refs", "heads", "missing")); err == nil {
		t.Fatal("expected missing ref read error")
	}

	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "bad"), "nope\n")
	if _, err := repo.resolveRef(filepath.Join(repo.gitDir, "refs", "heads", "bad")); err == nil || !strings.Contains(err.Error(), "invalid hash in ref file") {
		t.Fatalf("resolveRef() error = %v, want invalid hash error", err)
	}

	target := filepath.Join(repo.gitDir, "refs", "heads", "main")
	writeTextFile(t, target, testHash2+"\n")
	repo.refs["refs/heads/main"] = mustHash(t, testHash2)
	writeTextFile(t, filepath.Join(repo.gitDir, "refs", "heads", "alias"), "ref: refs/heads/main\n")
	resolved, err := repo.resolveRef(filepath.Join(repo.gitDir, "refs", "heads", "alias"))
	if err != nil || resolved != mustHash(t, testHash2) {
		t.Fatalf("resolveRef(alias) = %v, %v", resolved, err)
	}
}

func TestLoadRefsWrapsHEADFailure(t *testing.T) {
	repo := newRepoSkeleton(t)
	writeTextFile(t, filepath.Join(repo.gitDir, "HEAD"), "bad\n")
	if err := repo.loadRefs(); err == nil || !strings.Contains(err.Error(), "failed to load head") {
		t.Fatalf("loadRefs() error = %v, want HEAD wrapper", err)
	}
}

func TestEnsurePathWithinBaseErrorPaths(t *testing.T) {
	originalAbs := filepathAbs
	originalRel := filepathRel
	t.Cleanup(func() {
		filepathAbs = originalAbs
		filepathRel = originalRel
	})

	filepathAbs = func(string) (string, error) {
		return "", os.ErrInvalid
	}
	if err := ensurePathWithinBase("base", "candidate"); err == nil {
		t.Fatal("expected abs base error")
	}

	callCount := 0
	filepathAbs = func(s string) (string, error) {
		callCount++
		if callCount == 2 {
			return "", os.ErrInvalid
		}
		return s, nil
	}
	if err := ensurePathWithinBase("base", "candidate"); err == nil {
		t.Fatal("expected abs candidate error")
	}

	filepathAbs = func(s string) (string, error) { return s, nil }
	filepathRel = func(string, string) (string, error) {
		return "", os.ErrInvalid
	}
	if err := ensurePathWithinBase("base", "candidate"); err == nil {
		t.Fatal("expected rel error")
	}
}
