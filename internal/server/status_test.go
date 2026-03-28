package server

import (
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestTranslateWorkingTreeStatus_MapsAndSortsStatuses(t *testing.T) {
	wts := &gitcore.WorkingTreeStatus{
		Files: []gitcore.FileState{
			{
				Path:           "zeta.txt",
				StagedChange:   gitcore.ChangeTypeModified,
				StagedHash:     gitcore.Hash("index-zeta"),
				UnstagedChange: gitcore.ChangeTypeDeleted,
				WorktreeHash:   gitcore.Hash("work-zeta"),
			},
			{
				Path:         "alpha.txt",
				StagedChange: gitcore.ChangeTypeAdded,
				StagedHash:   gitcore.Hash("index-alpha"),
			},
			{
				Path:         "copied.txt",
				StagedChange: gitcore.ChangeTypeCopied,
				StagedHash:   gitcore.Hash("index-copied"),
			},
			{
				Path:         "renamed.txt",
				StagedChange: gitcore.ChangeTypeRenamed,
				StagedHash:   gitcore.Hash("index-renamed"),
			},
			{
				Path:           "beta.txt",
				UnstagedChange: gitcore.ChangeTypeModified,
				WorktreeHash:   gitcore.Hash("work-beta"),
			},
			{
				Path:        "untracked.txt",
				IsUntracked: true,
			},
			{
				Path:           "ignored-by-server.txt",
				StagedChange:   gitcore.ChangeType(-1),
				UnstagedChange: gitcore.ChangeType(-1),
			},
		},
	}

	got := translateWorkingTreeStatus(wts)

	if len(got.Staged) != 4 {
		t.Fatalf("len(Staged) = %d, want 4", len(got.Staged))
	}
	if got.Staged[0] != (FileStatus{Path: "alpha.txt", StatusCode: "A", BlobHash: "index-alpha"}) {
		t.Fatalf("first staged status = %+v", got.Staged[0])
	}
	if got.Staged[1] != (FileStatus{Path: "copied.txt", StatusCode: "C", BlobHash: "index-copied"}) {
		t.Fatalf("second staged status = %+v", got.Staged[1])
	}
	if got.Staged[2] != (FileStatus{Path: "renamed.txt", StatusCode: "R", BlobHash: "index-renamed"}) {
		t.Fatalf("third staged status = %+v", got.Staged[2])
	}
	if got.Staged[3] != (FileStatus{Path: "zeta.txt", StatusCode: "M", BlobHash: "index-zeta"}) {
		t.Fatalf("fourth staged status = %+v", got.Staged[3])
	}

	if len(got.Modified) != 2 {
		t.Fatalf("len(Modified) = %d, want 2", len(got.Modified))
	}
	if got.Modified[0] != (FileStatus{Path: "beta.txt", StatusCode: "M", BlobHash: "work-beta"}) {
		t.Fatalf("first modified status = %+v", got.Modified[0])
	}
	if got.Modified[1] != (FileStatus{Path: "zeta.txt", StatusCode: "D", BlobHash: "work-zeta"}) {
		t.Fatalf("second modified status = %+v", got.Modified[1])
	}

	if len(got.Untracked) != 1 {
		t.Fatalf("len(Untracked) = %d, want 1", len(got.Untracked))
	}
	if got.Untracked[0] != (FileStatus{Path: "untracked.txt", StatusCode: "?"}) {
		t.Fatalf("untracked status = %+v", got.Untracked[0])
	}
}

func TestStatusCodeHelpers(t *testing.T) {
	indexCases := map[gitcore.ChangeType]string{
		gitcore.ChangeTypeAdded:    "A",
		gitcore.ChangeTypeModified: "M",
		gitcore.ChangeTypeDeleted:  "D",
		gitcore.ChangeTypeRenamed:  "R",
		gitcore.ChangeTypeCopied:   "C",
		0:                          "",
		gitcore.ChangeType(-1):     "",
	}
	for input, want := range indexCases {
		if got := indexStatusCode(input); got != want {
			t.Fatalf("indexStatusCode(%v) = %q, want %q", input, got, want)
		}
	}

	workCases := map[gitcore.ChangeType]string{
		gitcore.ChangeTypeModified: "M",
		gitcore.ChangeTypeDeleted:  "D",
		0:                          "",
		gitcore.ChangeType(-1):     "",
	}
	for input, want := range workCases {
		if got := workStatusCode(input); got != want {
			t.Fatalf("workStatusCode(%v) = %q, want %q", input, got, want)
		}
	}
}
