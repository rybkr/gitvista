package server

import (
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestTranslateWorkingTreeStatus_MapsAndSortsStatuses(t *testing.T) {
	wts := &gitcore.WorkingTreeStatus{
		Files: []gitcore.FileStatus{
			{
				Path:        "zeta.txt",
				IndexStatus: gitcore.StatusModified,
				IndexHash:   gitcore.Hash("index-zeta"),
				WorkStatus:  gitcore.StatusDeleted,
				WorkHash:    gitcore.Hash("work-zeta"),
			},
			{
				Path:        "alpha.txt",
				IndexStatus: gitcore.StatusAdded,
				IndexHash:   gitcore.Hash("index-alpha"),
			},
			{
				Path:        "copied.txt",
				IndexStatus: gitcore.StatusCopied,
				IndexHash:   gitcore.Hash("index-copied"),
			},
			{
				Path:        "renamed.txt",
				IndexStatus: gitcore.StatusRenamed,
				IndexHash:   gitcore.Hash("index-renamed"),
			},
			{
				Path:       "beta.txt",
				WorkStatus: gitcore.StatusModified,
				WorkHash:   gitcore.Hash("work-beta"),
			},
			{
				Path:        "untracked.txt",
				IsUntracked: true,
			},
			{
				Path:        "ignored-by-server.txt",
				IndexStatus: "unknown",
				WorkStatus:  "unknown",
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
	indexCases := map[string]string{
		gitcore.StatusAdded:    "A",
		gitcore.StatusModified: "M",
		gitcore.StatusDeleted:  "D",
		gitcore.StatusRenamed:  "R",
		gitcore.StatusCopied:   "C",
		"":                     "",
		"unknown":              "",
	}
	for input, want := range indexCases {
		if got := indexStatusCode(input); got != want {
			t.Fatalf("indexStatusCode(%q) = %q, want %q", input, got, want)
		}
	}

	workCases := map[string]string{
		gitcore.StatusModified: "M",
		gitcore.StatusDeleted:  "D",
		"":                     "",
		"unknown":              "",
	}
	for input, want := range workCases {
		if got := workStatusCode(input); got != want {
			t.Fatalf("workStatusCode(%q) = %q, want %q", input, got, want)
		}
	}
}
