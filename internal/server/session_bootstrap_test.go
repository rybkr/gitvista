package server

import (
	"testing"
	"time"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repositoryview"
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

func TestBuildBootstrapMessages_AppendsCompletionPayload(t *testing.T) {
	when := time.Now()
	hash := gitcore.Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	delta := &repositoryview.RepositoryDelta{
		AddedCommits: []*gitcore.Commit{{
			ID:        hash,
			Author:    gitcore.Signature{When: when},
			Committer: gitcore.Signature{When: when},
		}},
		AddedBranches: map[string]gitcore.Hash{"main": hash},
		HeadHash:      string(hash),
		Tags:          map[string]string{"v1.0.0": string(hash)},
	}

	messages := buildBootstrapMessages(delta)
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].Type != messageTypeGraphBootstrapChunk {
		t.Fatalf("first type = %q, want %q", messages[0].Type, messageTypeGraphBootstrapChunk)
	}
	if messages[0].Bootstrap == nil || len(messages[0].Bootstrap.Commits) != 1 {
		t.Fatal("first message missing bootstrap commit payload")
	}
	if messages[1].Type != messageTypeBootstrapComplete {
		t.Fatalf("second type = %q, want %q", messages[1].Type, messageTypeBootstrapComplete)
	}
	if messages[1].BootstrapComplete == nil || messages[1].BootstrapComplete.Tags["v1.0.0"] != string(hash) {
		t.Fatal("completion payload missing tags")
	}
}
