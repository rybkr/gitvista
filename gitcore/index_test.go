package gitcore

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- test helper for Git index checksum
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildIndexHeaderVersion(version, numEntries uint32) []byte {
	buf := &bytes.Buffer{}
	buf.WriteString(indexMagic)
	_ = binary.Write(buf, binary.BigEndian, version)
	_ = binary.Write(buf, binary.BigEndian, numEntries)
	return buf.Bytes()
}

func buildIndexHeader(numEntries uint32) []byte {
	return buildIndexHeaderVersion(2, numEntries)
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

func appendIndexChecksum(data []byte) []byte {
	sum := sha1.Sum(data) // #nosec G401 -- test helper for Git index checksum
	return append(append([]byte{}, data...), sum[:]...)
}

func buildIndexEntryV4(path string, prevPath string, hash [20]byte, mode uint32, stage int) []byte {
	buf := &bytes.Buffer{}

	fields := []uint32{
		0, 0,
		0, 0,
		0, 0, mode, 0, 0, uint32(len(path)),
	}
	for _, field := range fields {
		_ = binary.Write(buf, binary.BigEndian, field)
	}

	buf.Write(hash[:])

	flags := uint16(len(path)) | (uint16(stage) << indexFlagStageShift)
	_ = binary.Write(buf, binary.BigEndian, flags)

	stripCount := sharedPathTrimCount(prevPath, path)
	buf.Write(encodeDeltaOffset(int64(stripCount)))
	buf.WriteString(path[len(prevPath)-stripCount:])
	buf.WriteByte(0)

	return buf.Bytes()
}

func sharedPathTrimCount(prevPath, path string) int {
	maxPrefix := len(prevPath)
	if len(path) < maxPrefix {
		maxPrefix = len(path)
	}
	shared := 0
	for shared < maxPrefix && prevPath[shared] == path[shared] {
		shared++
	}
	return len(prevPath) - shared
}

func writeIndexFile(t *testing.T, gitDir string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(gitDir, "index"), appendIndexChecksum(data), 0o644); err != nil {
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
	data := buildIndexHeaderVersion(5, 0)
	writeIndexFile(t, gitDir, data)

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want error")
	}
}

func TestReadIndex_Version3(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("abababababababababababababababababababab")

	data := append(buildIndexHeaderVersion(3, 1), buildIndexEntry("README.md", hash, 0o100644, 0)...)
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if idx.Version != 3 {
		t.Fatalf("idx.Version = %d, want 3", idx.Version)
	}
	if len(idx.Entries) != 1 || idx.Entries[0].Path != "README.md" {
		t.Fatalf("unexpected entries: %#v", idx.Entries)
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

func TestReadIndex_Version4PrefixCompression(t *testing.T) {
	gitDir := t.TempDir()
	hashA := hashFromHex("1212121212121212121212121212121212121212")
	hashB := hashFromHex("3434343434343434343434343434343434343434")
	hashC := hashFromHex("5656565656565656565656565656565656565656")

	data := buildIndexHeaderVersion(4, 3)
	prev := ""
	for _, tc := range []struct {
		path string
		hash [20]byte
	}{
		{path: "app/models/item.go", hash: hashA},
		{path: "app/models/item_test.go", hash: hashB},
		{path: "cmd/gitvista/main.go", hash: hashC},
	} {
		data = append(data, buildIndexEntryV4(tc.path, prev, tc.hash, 0o100644, 0)...)
		prev = tc.path
	}
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if idx.Version != 4 {
		t.Fatalf("idx.Version = %d, want 4", idx.Version)
	}
	paths := []string{
		"app/models/item.go",
		"app/models/item_test.go",
		"cmd/gitvista/main.go",
	}
	if len(idx.Entries) != len(paths) {
		t.Fatalf("len(idx.Entries) = %d, want %d", len(idx.Entries), len(paths))
	}
	for i, path := range paths {
		if idx.Entries[i].Path != path {
			t.Fatalf("entry[%d].Path = %q, want %q", i, idx.Entries[i].Path, path)
		}
	}
}

func TestReadIndex_IgnoresOptionalExtensions(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("7878787878787878787878787878787878787878")

	data := append(buildIndexHeader(1), buildIndexEntry("README.md", hash, 0o100644, 0)...)
	data = append(data, []byte("TREE")...)
	data = binary.BigEndian.AppendUint32(data, 4)
	data = append(data, []byte("test")...)
	writeIndexFile(t, gitDir, data)

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if len(idx.Entries) != 1 || idx.Entries[0].Path != "README.md" {
		t.Fatalf("unexpected entries: %#v", idx.Entries)
	}
}

func TestReadIndex_ChecksumMismatch(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("9999999999999999999999999999999999999999")

	data := appendIndexChecksum(append(buildIndexHeader(1), buildIndexEntry("README.md", hash, 0o100644, 0)...))
	data[len(data)-1] ^= 0xFF
	if err := os.WriteFile(filepath.Join(gitDir, "index"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(index): %v", err)
	}

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want checksum mismatch")
	}
}

func TestReadIndex_TruncatedChecksum(t *testing.T) {
	gitDir := t.TempDir()
	hash := hashFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	data := append(buildIndexHeader(1), buildIndexEntry("README.md", hash, 0o100644, 0)...)
	if err := os.WriteFile(filepath.Join(gitDir, "index"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(index): %v", err)
	}

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want truncated checksum error")
	}
}

func TestReadIndex_Version4RejectsInvalidPrefixLength(t *testing.T) {
	gitDir := t.TempDir()
	hashA := hashFromHex("0101010101010101010101010101010101010101")
	hashB := hashFromHex("0202020202020202020202020202020202020202")

	data := buildIndexHeaderVersion(4, 2)
	firstPath := "a.txt"
	data = append(data, buildIndexEntryV4(firstPath, "", hashA, 0o100644, 0)...)
	second := &bytes.Buffer{}
	second.Write(buildIndexEntryWithStats("ignored", hashB, 0o100644, 0, 0, 0, 0, 0)[:indexFixedEntrySize])
	second.Write(encodeDeltaOffset(99))
	second.WriteString("suffix")
	second.WriteByte(0)
	data = append(data, second.Bytes()...)
	writeIndexFile(t, gitDir, data)

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want invalid prefix length error")
	}
}

func TestReadIndex_ReadFileError(t *testing.T) {
	gitDir := t.TempDir()
	indexPath := filepath.Join(gitDir, "index")
	if err := os.Mkdir(indexPath, 0o755); err != nil {
		t.Fatalf("Mkdir(index): %v", err)
	}

	if _, err := ReadIndex(gitDir); err == nil {
		t.Fatal("ReadIndex() error = nil, want read failure")
	}
}

func TestParseIndexEntryFixedFields_Truncated(t *testing.T) {
	t.Parallel()

	if _, err := parseIndexEntryFixedFields(make([]byte, indexFixedEntrySize-1), 0); err == nil {
		t.Fatal("parseIndexEntryFixedFields() error = nil, want truncated entry error")
	}
}

func TestParseIndex_FileTooShort(t *testing.T) {
	t.Parallel()

	if _, err := parseIndex(make([]byte, 12)); err == nil {
		t.Fatal("parseIndex() error = nil, want file-too-short error")
	}
}

func TestParseIndexEntryV2V3_MissingNullTerminator(t *testing.T) {
	t.Parallel()

	hash := hashFromHex("abababababababababababababababababababab")
	entry := buildIndexEntry("missing-null", hash, 0o100644, 0)
	entry = bytes.TrimRight(entry, "\x00")

	if _, _, err := parseIndexEntryV2V3(entry, 0); err == nil {
		t.Fatal("parseIndexEntryV2V3() error = nil, want missing terminator error")
	}
}

func TestParseIndexEntryV2V3_TruncatedFixedFields(t *testing.T) {
	t.Parallel()

	if _, _, err := parseIndexEntryV2V3(make([]byte, indexFixedEntrySize-1), 0); err == nil {
		t.Fatal("parseIndexEntryV2V3() error = nil, want truncated fixed-fields error")
	}
}

func TestParseIndexEntryV2V3_DetectsMissingPadding(t *testing.T) {
	t.Parallel()

	hash := hashFromHex("cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd")
	entry := buildIndexEntry("pad", hash, 0o100644, 0)
	entry = entry[:len(entry)-1]

	if _, _, err := parseIndexEntryV2V3(entry, 0); err == nil {
		t.Fatal("parseIndexEntryV2V3() error = nil, want padding bounds error")
	}
}

func TestParseIndexEntryV4_MissingNullTerminator(t *testing.T) {
	t.Parallel()

	hash := hashFromHex("4545454545454545454545454545454545454545")
	entry := buildIndexEntryV4("src/main.go", "src/file.go", hash, 0o100644, 0)
	entry = entry[:len(entry)-1]

	if _, _, err := parseIndexEntryV4(entry, 0, "src/file.go"); err == nil {
		t.Fatal("parseIndexEntryV4() error = nil, want missing terminator error")
	}
}

func TestParseIndexEntryV4_TruncatedFixedFields(t *testing.T) {
	t.Parallel()

	if _, _, err := parseIndexEntryV4(make([]byte, indexFixedEntrySize-1), 0, "prev"); err == nil {
		t.Fatal("parseIndexEntryV4() error = nil, want truncated fixed-fields error")
	}
}

func TestParseIndexEntryV4_InvalidPrefixVarInt(t *testing.T) {
	t.Parallel()

	hash := hashFromHex("4646464646464646464646464646464646464646")
	buf := &bytes.Buffer{}
	buf.Write(buildIndexEntryWithStats("ignored", hash, 0o100644, 0, 0, 0, 0, 0)[:indexFixedEntrySize])

	if _, _, err := parseIndexEntryV4(buf.Bytes(), 0, "prev/path.go"); err == nil {
		t.Fatal("parseIndexEntryV4() error = nil, want invalid prefix varint error")
	}
}

func TestParseIndexEntryV4_RejectsEmptyReconstructedPath(t *testing.T) {
	t.Parallel()

	hash := hashFromHex("5656565656565656565656565656565656565656")
	buf := &bytes.Buffer{}
	buf.Write(buildIndexEntryWithStats("ignored", hash, 0o100644, 0, 0, 0, 0, 0)[:indexFixedEntrySize])
	buf.Write(encodeDeltaOffset(int64(len("abc"))))
	buf.WriteByte(0)

	if _, _, err := parseIndexEntryV4(buf.Bytes(), 0, "abc"); err == nil {
		t.Fatal("parseIndexEntryV4() error = nil, want empty path error")
	}
}

func TestParseIndexVarInt(t *testing.T) {
	t.Parallel()

	t.Run("single-byte", func(t *testing.T) {
		value, consumed, err := parseIndexVarInt([]byte{0x2A}, 0)
		if err != nil {
			t.Fatalf("parseIndexVarInt() error = %v", err)
		}
		if value != 42 || consumed != 1 {
			t.Fatalf("parseIndexVarInt() = (%d, %d), want (42, 1)", value, consumed)
		}
	})

	t.Run("multi-byte-with-offset", func(t *testing.T) {
		value, consumed, err := parseIndexVarInt([]byte{0xFF, 0xAC, 0x02}, 1)
		if err != nil {
			t.Fatalf("parseIndexVarInt() error = %v", err)
		}
		if value != 300 || consumed != 2 {
			t.Fatalf("parseIndexVarInt() = (%d, %d), want (300, 2)", value, consumed)
		}
	})

	t.Run("missing-data", func(t *testing.T) {
		if _, _, err := parseIndexVarInt([]byte{0x01}, 1); err == nil {
			t.Fatal("parseIndexVarInt() error = nil, want missing data error")
		}
	})

	t.Run("unterminated", func(t *testing.T) {
		if _, _, err := parseIndexVarInt([]byte{0x80}, 0); err == nil {
			t.Fatal("parseIndexVarInt() error = nil, want unterminated varint error")
		}
	})

	t.Run("too-large", func(t *testing.T) {
		if _, _, err := parseIndexVarInt(bytes.Repeat([]byte{0x80}, 10), 0); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("parseIndexVarInt() error = %v, want too large", err)
		}
	})
}
