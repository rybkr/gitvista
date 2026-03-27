package main

import (
	"strings"
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestParseCatFileArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantMode catFileMode
		wantRev  string
		wantCode int
		wantErr  string
	}{
		{name: "type", args: []string{"-t", "HEAD"}, wantMode: catFileModeType, wantRev: "HEAD"},
		{name: "size", args: []string{"-s", "abc123"}, wantMode: catFileModeSize, wantRev: "abc123"},
		{name: "pretty", args: []string{"-p", "main"}, wantMode: catFileModePretty, wantRev: "main"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli cat-file"},
		{name: "unsupported flag", args: []string{"--bad", "HEAD"}, wantCode: 1, wantErr: "unsupported argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseCatFileArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseCatFileArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.mode != tt.wantMode || opts.revision != tt.wantRev {
				t.Fatalf("parseCatFileArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestFormatCatFileOutput(t *testing.T) {
	blobData, err := formatCatFileOutput(&gitcore.CatFileResult{
		Type: gitcore.ObjectTypeBlob,
		Data: []byte("blob\n"),
	})
	if err != nil || string(blobData) != "blob\n" {
		t.Fatalf("formatCatFileOutput(blob) = %q, %v", string(blobData), err)
	}

	treeHash1 := mustCLIHash(t, "1111111111111111111111111111111111111111")
	treeHash2 := mustCLIHash(t, "2222222222222222222222222222222222222222")
	treeData := treeBody(
		treeEntryBytes("100644", "README.md", treeHash1),
		treeEntryBytes("120000", "link", treeHash2),
	)
	formatted, err := formatCatFileOutput(&gitcore.CatFileResult{
		Type: gitcore.ObjectTypeTree,
		Data: treeData,
	})
	if err != nil {
		t.Fatalf("formatCatFileOutput(tree) error: %v", err)
	}
	want := strings.Join([]string{
		"100644 blob 1111111111111111111111111111111111111111\tREADME.md",
		"120000 blob 2222222222222222222222222222222222222222\tlink",
		"",
	}, "\n")
	if string(formatted) != want {
		t.Fatalf("formatCatFileOutput(tree) = %q, want %q", string(formatted), want)
	}

	if _, err := formatCatFileOutput(&gitcore.CatFileResult{Type: gitcore.ObjectTypeInvalid, Data: []byte("x")}); err == nil {
		t.Fatal("expected invalid type error")
	}
	if _, err := formatTreeObject(append([]byte("100644 bad"), 0)); err == nil {
		t.Fatal("expected malformed tree error")
	}
}

func TestFormatTreeObjectNormalizesTreeMode(t *testing.T) {
	treeHash := mustCLIHash(t, "1111111111111111111111111111111111111111")
	treeData := treeBody(treeEntryBytes("40000", "docs", treeHash))

	formatted, err := formatTreeObject(treeData)
	if err != nil {
		t.Fatalf("formatTreeObject(tree) error: %v", err)
	}

	want := "040000 tree 1111111111111111111111111111111111111111\tdocs\n"
	if string(formatted) != want {
		t.Fatalf("formatTreeObject(tree) = %q, want %q", string(formatted), want)
	}
}

func treeEntryBytes(mode, name string, hash gitcore.Hash) []byte {
	body := append([]byte(mode+" "+name), 0)
	raw := hashFromHex(hash)
	return append(body, raw[:]...)
}

func treeBody(entries ...[]byte) []byte {
	var body []byte
	for _, entry := range entries {
		body = append(body, entry...)
	}
	return body
}

func mustCLIHash(t *testing.T, s string) gitcore.Hash {
	t.Helper()
	h, err := gitcore.NewHash(s)
	if err != nil {
		t.Fatalf("NewHash(%q): %v", s, err)
	}
	return h
}

func hashFromHex(h gitcore.Hash) [20]byte {
	var out [20]byte
	for i := 0; i < 20; i++ {
		out[i] = byte((fromHex(string(h)[i*2]) << 4) | fromHex(string(h)[i*2+1]))
	}
	return out
}

func fromHex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	default:
		return int(b-'A') + 10
	}
}
