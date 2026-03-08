package server

import (
	"testing"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func TestMakeBootstrapCommit_PreservesBranchLabel(t *testing.T) {
	when := time.Now()
	commit := &gitcore.Commit{
		ID:                gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Parents:           []gitcore.Hash{gitcore.Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
		Author:            gitcore.Signature{When: when},
		Committer:         gitcore.Signature{When: when},
		Message:           "message",
		BranchLabel:       "feature/security",
		BranchLabelSource: "merge_message",
	}

	lightweight := makeBootstrapCommit(commit, true)
	if lightweight.BranchLabel != "feature/security" {
		t.Fatalf("lightweight BranchLabel = %q, want %q", lightweight.BranchLabel, "feature/security")
	}
	if lightweight.BranchLabelSource != "merge_message" {
		t.Fatalf("lightweight BranchLabelSource = %q, want %q", lightweight.BranchLabelSource, "merge_message")
	}

	full := makeBootstrapCommit(commit, false)
	if full.BranchLabel != "feature/security" {
		t.Fatalf("full BranchLabel = %q, want %q", full.BranchLabel, "feature/security")
	}
	if full.BranchLabelSource != "merge_message" {
		t.Fatalf("full BranchLabelSource = %q, want %q", full.BranchLabelSource, "merge_message")
	}
}
