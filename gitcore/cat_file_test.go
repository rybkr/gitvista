package gitcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatFileReturnsRawObjectData(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	treeID := mustHash(t, testHash2)
	commitID := mustHash(t, testHash3)
	tagID := mustHash(t, testHash4)

	treeBody := treeBodyWithEntries(
		treeEntry("100644", "README.md", blobID),
		treeEntry("040000", "dir", mustHash(t, testHash5)),
	)
	commitBody := []byte("tree " + testHash2 + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\ninitial commit\n")
	tagBody := []byte("object " + testHash3 + "\ntype commit\ntag v1.0\ntagger Jane Doe <jane@example.com> 1700000000 +0000\n\nrelease\n")

	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("hello world\n"))
	writeLooseObject(t, repo.gitDir, treeID, "tree", treeBody)
	writeLooseObject(t, repo.gitDir, commitID, "commit", commitBody)
	writeLooseObject(t, repo.gitDir, tagID, "tag", tagBody)

	tests := []struct {
		name     string
		revision string
		wantHash Hash
		wantType ObjectType
		wantSize int
		wantData string
	}{
		{name: "blob", revision: string(blobID), wantHash: blobID, wantType: ObjectTypeBlob, wantSize: len("hello world\n"), wantData: "hello world\n"},
		{name: "commit", revision: string(commitID), wantHash: commitID, wantType: ObjectTypeCommit, wantSize: len(commitBody), wantData: string(commitBody)},
		{name: "tree", revision: string(treeID), wantHash: treeID, wantType: ObjectTypeTree, wantSize: len(treeBody), wantData: string(treeBody)},
		{name: "tag", revision: string(tagID), wantHash: tagID, wantType: ObjectTypeTag, wantSize: len(tagBody), wantData: string(tagBody)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.CatFile(CatFileOptions{Revision: tt.revision})
			if err != nil {
				t.Fatalf("CatFile() error: %v", err)
			}
			if result.Hash != tt.wantHash || result.Type != tt.wantType || result.Size != tt.wantSize {
				t.Fatalf("CatFile() = %+v", result)
			}
			if string(result.Data) != tt.wantData {
				t.Fatalf("CatFile().Data = %q, want %q", string(result.Data), tt.wantData)
			}
		})
	}
}

func TestCatFileAnnotatedTagNameResolvesToTagObject(t *testing.T) {
	repo := newRepoSkeleton(t)

	tagID := mustHash(t, testHash2)
	tagBody := []byte("object " + testHash1 + "\ntype commit\ntag v1.0\ntagger Jane Doe <jane@example.com> 1700000000 +0000\n\nrelease\n")

	writeLooseObject(t, repo.gitDir, tagID, "tag", tagBody)
	repo.refs["refs/tags/v1.0"] = tagID

	result, err := repo.CatFile(CatFileOptions{Revision: "v1.0"})
	if err != nil {
		t.Fatalf("CatFile() error: %v", err)
	}
	if result.Hash != tagID || result.Type != ObjectTypeTag || string(result.Data) != string(tagBody) {
		t.Fatalf("CatFile() = %+v", result)
	}
}

func TestCatFileLightweightTagNameResolvesToTargetObject(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("lightweight\n"))
	repo.refs["refs/tags/v1.0"] = blobID

	result, err := repo.CatFile(CatFileOptions{Revision: "v1.0"})
	if err != nil {
		t.Fatalf("CatFile() error: %v", err)
	}
	if result.Hash != blobID || result.Type != ObjectTypeBlob || result.Size != len("lightweight\n") {
		t.Fatalf("CatFile() = %+v", result)
	}
}

func TestCatFileShortHashResolution(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("short hash\n"))

	result, err := repo.CatFile(CatFileOptions{Revision: string(blobID)[:8]})
	if err != nil {
		t.Fatalf("CatFile() error: %v", err)
	}
	if result.Hash != blobID {
		t.Fatalf("CatFile().Hash = %s, want %s", result.Hash, blobID)
	}
}

func TestCatFileResolvesBranchName(t *testing.T) {
	repo := newRepoSkeleton(t)

	branchID := mustHash(t, testHash1)
	writeLooseObject(t, repo.gitDir, branchID, "blob", []byte("branch object\n"))
	repo.refs["refs/heads/main"] = branchID

	result, err := repo.CatFile(CatFileOptions{Revision: "main"})
	if err != nil {
		t.Fatalf("CatFile(main) error: %v", err)
	}
	if result.Hash != branchID || result.Type != ObjectTypeBlob || string(result.Data) != "branch object\n" {
		t.Fatalf("CatFile(main) = %+v", result)
	}
}

func TestCatFileReturnsErrorWhenMatchingObjectHashesFails(t *testing.T) {
	repo := newRepoSkeleton(t)

	if err := os.WriteFile(filepath.Join(repo.gitDir, "objects", "ab"), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write bad loose object prefix path: %v", err)
	}

	if _, err := repo.CatFile(CatFileOptions{Revision: "ab"}); err == nil {
		t.Fatal("expected matchingObjectHashes error")
	} else if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCatFilePackedObject(t *testing.T) {
	packPath := filepath.Join(t.TempDir(), "blob.pack")
	if err := os.WriteFile(packPath, packObjectBytes(t, ObjectTypeBlob, []byte("packed hello\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := NewEmptyRepository()
	blobID := mustHash(t, testHash1)
	repo.packLocations[blobID] = PackLocation{packPath: packPath, offset: 0}

	result, err := repo.CatFile(CatFileOptions{Revision: string(blobID)})
	if err != nil {
		t.Fatalf("CatFile() error: %v", err)
	}
	if result.Hash != blobID || result.Type != ObjectTypeBlob || result.Size != len("packed hello\n") || string(result.Data) != "packed hello\n" {
		t.Fatalf("CatFile() = %+v", result)
	}
}

func TestCatFileResolvesHEADAndRemoteBranch(t *testing.T) {
	repo := newRepoSkeleton(t)

	headID := mustHash(t, testHash1)
	remoteID := mustHash(t, testHash2)
	writeLooseObject(t, repo.gitDir, headID, "blob", []byte("head object\n"))
	writeLooseObject(t, repo.gitDir, remoteID, "blob", []byte("remote object\n"))

	repo.head = headID
	repo.refs["refs/remotes/origin/main"] = remoteID

	headResult, err := repo.CatFile(CatFileOptions{Revision: "HEAD"})
	if err != nil {
		t.Fatalf("CatFile(HEAD) error: %v", err)
	}
	if headResult.Hash != headID || headResult.Type != ObjectTypeBlob {
		t.Fatalf("CatFile(HEAD) = %+v", headResult)
	}

	remoteResult, err := repo.CatFile(CatFileOptions{Revision: "refs/remotes/origin/main"})
	if err != nil {
		t.Fatalf("CatFile(remote) error: %v", err)
	}
	if remoteResult.Hash != remoteID || remoteResult.Type != ObjectTypeBlob {
		t.Fatalf("CatFile(remote) = %+v", remoteResult)
	}
}

func TestCatFileRejectsUnbornHEADAndMissingObjectForValidHash(t *testing.T) {
	repo := newRepoSkeleton(t)

	if _, err := repo.CatFile(CatFileOptions{Revision: "HEAD"}); err == nil {
		t.Fatal("expected unborn HEAD error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected HEAD error: %v", err)
	}

	missingHash := mustHash(t, testHash1)
	if _, err := repo.CatFile(CatFileOptions{Revision: string(missingHash)}); err == nil {
		t.Fatal("expected missing object error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected missing hash error: %v", err)
	}

	repo.refs["refs/tags/v1.0"] = missingHash
	if _, err := repo.CatFile(CatFileOptions{Revision: "v1.0"}); err == nil {
		t.Fatal("expected missing object behind ref error")
	} else if !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("unexpected missing ref target error: %v", err)
	}
}

func TestCatFileRejectsMissingAndAmbiguousRevisions(t *testing.T) {
	repo := newRepoSkeleton(t)

	blob1 := mustHash(t, "abcdef0000000000000000000000000000000000")
	blob2 := mustHash(t, "abcdef1111111111111111111111111111111111")
	writeLooseObject(t, repo.gitDir, blob1, "blob", []byte("one"))
	writeLooseObject(t, repo.gitDir, blob2, "blob", []byte("two"))

	if _, err := repo.CatFile(CatFileOptions{Revision: "missing"}); err == nil {
		t.Fatal("expected missing revision error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected missing revision error: %v", err)
	}

	if _, err := repo.CatFile(CatFileOptions{Revision: ""}); err == nil {
		t.Fatal("expected empty revision error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected empty revision error: %v", err)
	}

	ambiguousPrefix := string(blob1)[:6]
	if _, err := repo.CatFile(CatFileOptions{Revision: ambiguousPrefix}); err == nil {
		t.Fatal("expected ambiguous short hash error")
	} else if !strings.Contains(err.Error(), "ambiguous argument") {
		t.Fatalf("unexpected ambiguous short hash error: %v", err)
	}
}

func TestCatFileHelperCoverage(t *testing.T) {
	repo := newRepoSkeleton(t)

	blobID := mustHash(t, testHash1)
	tagID := mustHash(t, testHash2)
	commitID := mustHash(t, testHash3)
	writeLooseObject(t, repo.gitDir, blobID, "blob", []byte("helper\n"))
	repo.head = blobID
	repo.commitMap[commitID] = &Commit{ID: commitID}
	repo.refs["refs/heads/main"] = blobID
	repo.refs["refs/tags/v1.0"] = tagID
	repo.tags = []*Tag{{ID: tagID, Object: commitID}}
	repo.packLocations[mustHash(t, testHash4)] = PackLocation{packPath: "x.pack", offset: 1}

	hashes := repo.knownObjectHashes()
	if len(hashes) < 4 {
		t.Fatalf("knownObjectHashes() = %#v", hashes)
	}

	shortMatches, err := repo.matchingObjectHashes(string(blobID)[:8])
	if err != nil {
		t.Fatalf("matchingObjectHashes() error: %v", err)
	}
	if len(shortMatches) == 0 {
		t.Fatalf("matchingObjectHashes() = %#v, want at least one match", shortMatches)
	}

	noMatches, err := repo.matchingObjectHashes("")
	if err != nil {
		t.Fatalf("matchingObjectHashes(empty) error: %v", err)
	}
	if noMatches != nil {
		t.Fatalf("matchingObjectHashes(empty) = %#v, want nil", noMatches)
	}

	fullMatches, err := repo.matchingObjectHashes(string(blobID))
	if err != nil {
		t.Fatalf("matchingObjectHashes(full) error: %v", err)
	}
	if fullMatches != nil {
		t.Fatalf("matchingObjectHashes(full) = %#v, want nil", fullMatches)
	}

	tooShort, err := repo.matchLooseObjectHashes("a")
	if err != nil {
		t.Fatalf("matchLooseObjectHashes(short) error: %v", err)
	}
	if tooShort != nil {
		t.Fatalf("matchLooseObjectHashes(short) = %#v, want nil", tooShort)
	}

	matches, err := repo.matchLooseObjectHashes(string(blobID)[:8])
	if err != nil {
		t.Fatalf("matchLooseObjectHashes() error: %v", err)
	}
	if len(matches) != 1 || matches[0] != blobID {
		t.Fatalf("matchLooseObjectHashes() = %#v, want [%s]", matches, blobID)
	}

	if mkErr := os.MkdirAll(filepath.Join(repo.gitDir, "objects", string(blobID)[:2], "nested"), 0o750); mkErr != nil {
		t.Fatalf("mkdir nested object dir: %v", mkErr)
	}
	if writeErr := os.WriteFile(filepath.Join(repo.gitDir, "objects", string(blobID)[:2], "badname"), []byte("x"), 0o600); writeErr != nil {
		t.Fatalf("write bad loose object name: %v", writeErr)
	}

	matches, err = repo.matchLooseObjectHashes(string(blobID)[:2])
	if err != nil {
		t.Fatalf("matchLooseObjectHashes(short prefix with junk entries) error: %v", err)
	}
	if len(matches) != 1 || matches[0] != blobID {
		t.Fatalf("matchLooseObjectHashes(short prefix with junk entries) = %#v, want [%s]", matches, blobID)
	}

	badRepo := newRepoSkeleton(t)
	if err := os.WriteFile(filepath.Join(badRepo.gitDir, "objects", "ab"), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write bad loose object prefix path: %v", err)
	}
	if _, err := badRepo.matchLooseObjectHashes("ab"); err == nil {
		t.Fatal("expected non-directory object prefix path error")
	}
}

func treeEntry(mode, name string, hash Hash) []byte {
	body := append([]byte(mode+" "+name), 0)
	raw := hashFromHex(string(hash))
	return append(body, raw[:]...)
}

func treeBodyWithEntries(entries ...[]byte) []byte {
	var body []byte
	for _, entry := range entries {
		body = append(body, entry...)
	}
	return body
}
