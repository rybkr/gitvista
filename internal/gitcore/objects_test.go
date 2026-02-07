package gitcore

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestParseCommitBody_NoParents(t *testing.T) {
	body := []byte("tree aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nauthor Test User <test@example.com> 1700000000 +0000\ncommitter Test User <test@example.com> 1700000000 +0000\n\nInitial commit\n")
	id := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	commit, err := parseCommitBody(body, id)
	if err != nil {
		t.Fatalf("parseCommitBody failed: %v", err)
	}

	if commit.ID != id {
		t.Errorf("ID: got %s, want %s", commit.ID, id)
	}
	if commit.Tree != Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Errorf("Tree: got %s", commit.Tree)
	}
	if len(commit.Parents) != 0 {
		t.Errorf("Parents: expected 0, got %d", len(commit.Parents))
	}
	if commit.Author.Name != "Test User" {
		t.Errorf("Author.Name: got %q", commit.Author.Name)
	}
	if commit.Author.Email != "test@example.com" {
		t.Errorf("Author.Email: got %q", commit.Author.Email)
	}
	if commit.Message != "Initial commit" {
		t.Errorf("Message: got %q", commit.Message)
	}
}

func TestParseCommitBody_OneParent(t *testing.T) {
	body := []byte("tree aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nparent cccccccccccccccccccccccccccccccccccccccc\nauthor Test User <test@example.com> 1700000000 +0000\ncommitter Test User <test@example.com> 1700000000 +0000\n\nSecond commit\n")
	id := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	commit, err := parseCommitBody(body, id)
	if err != nil {
		t.Fatalf("parseCommitBody failed: %v", err)
	}

	if len(commit.Parents) != 1 {
		t.Fatalf("Parents: expected 1, got %d", len(commit.Parents))
	}
	if commit.Parents[0] != Hash("cccccccccccccccccccccccccccccccccccccccc") {
		t.Errorf("Parent[0]: got %s", commit.Parents[0])
	}
}

func TestParseCommitBody_MultipleParents(t *testing.T) {
	body := []byte("tree aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nparent cccccccccccccccccccccccccccccccccccccccc\nparent dddddddddddddddddddddddddddddddddddddddd\nauthor Test User <test@example.com> 1700000000 +0000\ncommitter Test User <test@example.com> 1700000000 +0000\n\nMerge commit\n")
	id := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	commit, err := parseCommitBody(body, id)
	if err != nil {
		t.Fatalf("parseCommitBody failed: %v", err)
	}

	if len(commit.Parents) != 2 {
		t.Fatalf("Parents: expected 2, got %d", len(commit.Parents))
	}
	if commit.Parents[0] != Hash("cccccccccccccccccccccccccccccccccccccccc") {
		t.Errorf("Parent[0]: got %s", commit.Parents[0])
	}
	if commit.Parents[1] != Hash("dddddddddddddddddddddddddddddddddddddddd") {
		t.Errorf("Parent[1]: got %s", commit.Parents[1])
	}
	if commit.Message != "Merge commit" {
		t.Errorf("Message: got %q", commit.Message)
	}
}

func TestParseTagBody(t *testing.T) {
	body := []byte("object aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\ntype commit\ntag v1.0.0\ntagger Test User <test@example.com> 1700000000 +0000\n\nRelease v1.0.0\n")
	id := Hash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	tag, err := parseTagBody(body, id)
	if err != nil {
		t.Fatalf("parseTagBody failed: %v", err)
	}

	if tag.ID != id {
		t.Errorf("ID: got %s, want %s", tag.ID, id)
	}
	if tag.Object != Hash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Errorf("Object: got %s", tag.Object)
	}
	if tag.ObjType != CommitObject {
		t.Errorf("ObjType: got %d, want %d", tag.ObjType, CommitObject)
	}
	if tag.Name != "v1.0.0" {
		t.Errorf("Name: got %q", tag.Name)
	}
	if tag.Tagger.Name != "Test User" {
		t.Errorf("Tagger.Name: got %q", tag.Tagger.Name)
	}
	if tag.Message != "Release v1.0.0" {
		t.Errorf("Message: got %q", tag.Message)
	}
}

func TestParseTreeBody(t *testing.T) {
	// Tree body format: mode<SP>name<NUL>20-byte-hash
	hash1, _ := hex.DecodeString("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hash2, _ := hex.DecodeString("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hash3, _ := hex.DecodeString("cccccccccccccccccccccccccccccccccccccccc")

	var body bytes.Buffer
	// blob entry
	fmt.Fprintf(&body, "100644 file.txt")
	body.WriteByte(0)
	body.Write(hash1)
	// tree entry
	fmt.Fprintf(&body, "040000 subdir")
	body.WriteByte(0)
	body.Write(hash2)
	// submodule entry
	fmt.Fprintf(&body, "160000 vendor")
	body.WriteByte(0)
	body.Write(hash3)

	id := Hash("dddddddddddddddddddddddddddddddddddddddd")
	tree, err := parseTreeBody(body.Bytes(), id)
	if err != nil {
		t.Fatalf("parseTreeBody failed: %v", err)
	}

	if len(tree.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(tree.Entries))
	}

	tests := []struct {
		name     string
		mode     string
		entType  string
		entName  string
		hashHex  string
	}{
		{"blob", "100644", "blob", "file.txt", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"tree", "040000", "tree", "subdir", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{"submodule", "160000", "commit", "vendor", "cccccccccccccccccccccccccccccccccccccccc"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tree.Entries[i]
			if e.Mode != tt.mode {
				t.Errorf("Mode: got %q, want %q", e.Mode, tt.mode)
			}
			if e.Type != tt.entType {
				t.Errorf("Type: got %q, want %q", e.Type, tt.entType)
			}
			if e.Name != tt.entName {
				t.Errorf("Name: got %q, want %q", e.Name, tt.entName)
			}
			if string(e.ID) != tt.hashHex {
				t.Errorf("ID: got %s, want %s", e.ID, tt.hashHex)
			}
		})
	}
}

func TestReadCompressedData(t *testing.T) {
	original := []byte("the quick brown fox jumps over the lazy dog")

	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(original)
	w.Close()

	result, err := readCompressedData(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatalf("readCompressedData failed: %v", err)
	}

	if !bytes.Equal(result, original) {
		t.Errorf("got %q, want %q", result, original)
	}
}
