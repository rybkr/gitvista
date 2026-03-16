package gitcore

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func buildIndexHeader(numEntries uint32) []byte {
	buf := &bytes.Buffer{}
	buf.WriteString(indexMagic)
	_ = binary.Write(buf, binary.BigEndian, uint32(2))
	_ = binary.Write(buf, binary.BigEndian, numEntries)
	return buf.Bytes()
}

func buildIndexEntry(path string, hash [20]byte, mode uint32, stage int) []byte {
	return buildIndexEntryWithStats(path, hash, mode, stage, 0, 0, 0, 0)
}

func buildIndexEntryWithStats(path string, hash [20]byte, mode uint32, stage int, mtimeSec, mtimeNsec, ctimeSec, ctimeNsec uint32) []byte {
	buf := &bytes.Buffer{}

	fields := []uint32{
		ctimeSec, ctimeNsec,
		mtimeSec, mtimeNsec,
		0, 0, mode, 0, 0, uint32(len(path)),
	}
	for _, field := range fields {
		_ = binary.Write(buf, binary.BigEndian, field)
	}

	buf.Write(hash[:])

	flags := uint16(len(path)) | (uint16(stage) << indexFlagStageShift)
	_ = binary.Write(buf, binary.BigEndian, flags)

	buf.WriteString(path)
	buf.WriteByte(0)

	for buf.Len()%indexEntryAlignment != 0 {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

func writeIndexFile(t *testing.T, gitDir string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(gitDir, "index"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(index): %v", err)
	}
}

func TestReadIndex_NonExistentFile(t *testing.T) {
	gitDir := t.TempDir()

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if idx == nil {
		t.Fatal("ReadIndex() = nil")
	}
	if len(idx.Entries) != 0 {
		t.Fatalf("len(idx.Entries) = %d, want 0", len(idx.Entries))
	}
	if len(idx.ByPath) != 0 {
		t.Fatalf("len(idx.ByPath) = %d, want 0", len(idx.ByPath))
	}
}

func TestReadIndex_SingleEntry(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	data := append(buildIndexHeader(1), buildIndexEntry("README.md", hash, 0o100644, 0)...)
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if idx.Version != 2 {
		t.Fatalf("idx.Version = %d, want 2", idx.Version)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("len(idx.Entries) = %d, want 1", len(idx.Entries))
	}

	entry := idx.Entries[0]
	if entry.Path != "README.md" {
		t.Fatalf("entry.Path = %q, want README.md", entry.Path)
	}
	if entry.Mode != 0o100644 {
		t.Fatalf("entry.Mode = %o, want 100644", entry.Mode)
	}
	if entry.Stage != 0 {
		t.Fatalf("entry.Stage = %d, want 0", entry.Stage)
	}
	if idx.ByPath["README.md"] != &idx.Entries[0] {
		t.Fatal("ByPath entry does not point at slice entry")
	}
}

func TestReadIndex_InvalidMagic(t *testing.T) {
	gitDir := t.TempDir()
	data := buildIndexHeader(0)
	copy(data[:4], []byte("BADC"))
	writeIndexFile(t, gitDir, data)

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want error")
	}
}

func TestReadIndex_UnsupportedVersion(t *testing.T) {
	gitDir := t.TempDir()
	data := buildIndexHeader(0)
	binary.BigEndian.PutUint32(data[4:8], 3)
	writeIndexFile(t, gitDir, data)

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want error")
	}
}

func TestReadIndex_TruncatedEntry(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	data := append(buildIndexHeader(1), buildIndexEntry("a.txt", hash, 0o100644, 0)...)
	data = data[:len(data)-3]
	writeIndexFile(t, gitDir, data)

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want error")
	}
}

func TestReadIndex_StageExtraction(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("cccccccccccccccccccccccccccccccccccccccc")

	data := buildIndexHeader(3)
	data = append(data, buildIndexEntry("conflict.txt", hash, 0o100644, 1)...)
	data = append(data, buildIndexEntry("conflict.txt", hash, 0o100644, 2)...)
	data = append(data, buildIndexEntry("conflict.txt", hash, 0o100644, 3)...)
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if len(idx.ByPath) != 0 {
		t.Fatalf("len(idx.ByPath) = %d, want 0", len(idx.ByPath))
	}
	for i, entry := range idx.Entries {
		if want := i + 1; entry.Stage != want {
			t.Fatalf("entry[%d].Stage = %d, want %d", i, entry.Stage, want)
		}
	}
}

func TestReadIndex_ByPathOnlyStageZero(t *testing.T) {
	gitDir := t.TempDir()
	hashA := hashFromHex("dddddddddddddddddddddddddddddddddddddddd")
	hashB := hashFromHex("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")

	data := buildIndexHeader(3)
	data = append(data, buildIndexEntry("normal.txt", hashA, 0o100644, 0)...)
	data = append(data, buildIndexEntry("conflict.txt", hashA, 0o100644, 1)...)
	data = append(data, buildIndexEntry("conflict.txt", hashB, 0o100644, 2)...)
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if len(idx.ByPath) != 1 {
		t.Fatalf("len(idx.ByPath) = %d, want 1", len(idx.ByPath))
	}
	if _, ok := idx.ByPath["normal.txt"]; !ok {
		t.Fatal("missing stage-0 path in ByPath")
	}
	if _, ok := idx.ByPath["conflict.txt"]; ok {
		t.Fatal("conflict path should not exist in ByPath")
	}
}

func TestReadIndex_AlignmentAndExecutableMode(t *testing.T) {
	gitDir := t.TempDir()
	hashA := hashFromHex("ffffffffffffffffffffffffffffffffffffffff")
	hashB := hashFromHex("1111111111111111111111111111111111111111")

	data := buildIndexHeader(2)
	data = append(data, buildIndexEntry("bin/tool", hashA, 0o100755, 0)...)
	data = append(data, buildIndexEntry("nested/very/long/path.txt", hashB, 0o100644, 0)...)
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if len(idx.Entries) != 2 {
		t.Fatalf("len(idx.Entries) = %d, want 2", len(idx.Entries))
	}
	if idx.Entries[0].Mode != 0o100755 {
		t.Fatalf("entry[0].Mode = %o, want 100755", idx.Entries[0].Mode)
	}
	if idx.ByPath["bin/tool"] != &idx.Entries[0] {
		t.Fatal("ByPath pointer for executable entry is unstable")
	}
	if idx.ByPath["nested/very/long/path.txt"] != &idx.Entries[1] {
		t.Fatal("ByPath pointer for long path entry is unstable")
	}
}
