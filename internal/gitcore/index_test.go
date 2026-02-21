package gitcore

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// "DIRC" magic (4) + version (4, big-endian) + entry count (4, big-endian).

func buildIndexHeader(numEntries uint32) []byte {
	const version uint32 = 2
	var buf bytes.Buffer
	buf.WriteString(indexMagic)
	if err := binary.Write(&buf, binary.BigEndian, version); err != nil {
		panic(err)
	}
	if err := binary.Write(&buf, binary.BigEndian, numEntries); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// buildIndexEntry constructs the binary bytes for a single index entry.
//
// The entry layout is:
//
//	10 × uint32 fixed fields (40 bytes)
//	20 bytes SHA-1 hash
//	2 bytes flags (stage in bits 12-13, path-len in lower 12)
//	path bytes + NUL terminator
//	padding to align the total to a multiple of 8 bytes
//
// The caller may pass any stat values; this helper uses recognizable defaults
// so individual tests can override specific fields by parsing the result.
func buildIndexEntry(path string, hash [20]byte, mode uint32, stage int) []byte {
	var buf bytes.Buffer

	// 10 fixed uint32 fields (ctime_s, ctime_ns, mtime_s, mtime_ns, dev, ino, mode, uid, gid, size).
	// We use the mode parameter for the mode field and zero for everything else so
	// tests can set meaningful mode values while keeping unused fields predictable.
	fields := [10]uint32{
		0,    // ctime_sec
		0,    // ctime_nsec
		0,    // mtime_sec
		0,    // mtime_nsec
		0,    // device
		0,    // inode
		mode, // mode
		0,    // uid
		0,    // gid
		0,    // file_size
	}
	for _, f := range fields {
		if err := binary.Write(&buf, binary.BigEndian, f); err != nil {
			panic(err)
		}
	}

	buf.Write(hash[:])

	// 2-byte flags: bits 12-13 = stage, lower 12 = min(len(path), 0xFFF).
	nameLen := min(len(path), 0xFFF)
	flags := uint16(stage<<indexFlagStageShift) | uint16(nameLen)
	if err := binary.Write(&buf, binary.BigEndian, flags); err != nil {
		panic(err)
	}

	buf.WriteString(path)
	buf.WriteByte(0)

	// Alignment padding: the total length (fixed 62 bytes + path + NUL + padding)
	// must be a multiple of indexEntryAlignment (8). Round up.
	rawLen := indexFixedEntrySize + len(path) + 1
	paddedLen := (rawLen + indexEntryAlignment - 1) &^ (indexEntryAlignment - 1)
	padBytes := paddedLen - rawLen
	for range padBytes {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

// buildIndexEntryWithStats is like buildIndexEntry but exposes all stat fields,
// making it easy for tests that verify specific ctime/mtime/uid/gid/etc. values.
func buildIndexEntryWithStats(path string, hash [20]byte, mode uint32, stage int,
	ctimeSec, ctimeNsec, mtimeSec, mtimeNsec, device, inode, uid, gid, fileSize uint32,
) []byte {
	var buf bytes.Buffer

	fields := [10]uint32{
		ctimeSec, ctimeNsec,
		mtimeSec, mtimeNsec,
		device, inode,
		mode, uid, gid, fileSize,
	}
	for _, f := range fields {
		if err := binary.Write(&buf, binary.BigEndian, f); err != nil {
			panic(err)
		}
	}

	buf.Write(hash[:])

	nameLen := min(len(path), 0xFFF)
	flags := uint16(stage<<indexFlagStageShift) | uint16(nameLen)
	if err := binary.Write(&buf, binary.BigEndian, flags); err != nil {
		panic(err)
	}

	buf.WriteString(path)
	buf.WriteByte(0)

	rawLen := indexFixedEntrySize + len(path) + 1
	paddedLen := (rawLen + indexEntryAlignment - 1) &^ (indexEntryAlignment - 1)
	padBytes := paddedLen - rawLen
	for range padBytes {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

// writeIndexFile writes raw index bytes to <gitDir>/index and returns gitDir.
// The caller supplies an already-created temporary directory via t.TempDir().
func writeIndexFile(t *testing.T, gitDir string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("writeIndexFile: mkdir %s: %v", gitDir, err)
	}
	indexPath := filepath.Join(gitDir, "index")
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		t.Fatalf("writeIndexFile: %v", err)
	}
}

var zeroHash = [20]byte{}

// knownHash returns a deterministic non-zero hash for use in test assertions.
// The byte pattern is 0xAA repeated 20 times, giving the hex string "aa…aa".
var knownHash = [20]byte{
	0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA,
	0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA,
}

// TestReadIndex_NonExistentFile verifies that a gitDir which contains no index
// file is not treated as an error — it returns an empty (but valid) Index.
// This matches the behavior of a freshly-initialized repository where nothing
// has been staged yet.
func TestReadIndex_NonExistentFile(t *testing.T) {
	gitDir := t.TempDir()
	// Deliberately do NOT create an index file.

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: expected no error for missing index, got: %v", err)
	}
	if idx == nil {
		t.Fatal("ReadIndex: got nil Index for missing file")
	}
	if len(idx.Entries) != 0 {
		t.Errorf("Entries: want 0, got %d", len(idx.Entries))
	}
	if idx.ByPath == nil {
		t.Error("ByPath map must be non-nil even for empty index")
	}
	if len(idx.ByPath) != 0 {
		t.Errorf("ByPath: want 0 entries, got %d", len(idx.ByPath))
	}
}

// TestReadIndex_SingleEntry constructs a minimal but complete v2 index with one
// entry and verifies that every field is parsed to its expected value.
func TestReadIndex_SingleEntry(t *testing.T) {
	const (
		wantPath      = "src/main.go"
		wantMode      = uint32(0o100644) // regular file, 644 permissions
		wantCtimeSec  = uint32(1_700_000_000)
		wantCtimeNsec = uint32(123_456)
		wantMtimeSec  = uint32(1_700_000_100)
		wantMtimeNsec = uint32(654_321)
		wantDevice    = uint32(0xDEAD)
		wantInode     = uint32(0xBEEF)
		wantUID       = uint32(1000)
		wantGID       = uint32(1000)
		wantFileSize  = uint32(42)
	)
	// Hash: bytes 0x01 through 0x14 (hex "0102030405060708090a0b0c0d0e0f1011121314").
	var wantHashBytes [20]byte
	for i := range wantHashBytes {
		wantHashBytes[i] = byte(i + 1)
	}

	gitDir := t.TempDir()
	entryData := buildIndexEntryWithStats(
		wantPath, wantHashBytes, wantMode, 0, // stage 0
		wantCtimeSec, wantCtimeNsec, wantMtimeSec, wantMtimeNsec,
		wantDevice, wantInode, wantUID, wantGID, wantFileSize,
	)

	var raw bytes.Buffer
	raw.Write(buildIndexHeader(1))
	raw.Write(entryData)
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}

	if idx.Version != 2 {
		t.Errorf("Version: got %d, want 2", idx.Version)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("Entries: got %d, want 1", len(idx.Entries))
	}

	e := idx.Entries[0]

	if e.Path != wantPath {
		t.Errorf("Path: got %q, want %q", e.Path, wantPath)
	}
	if e.Mode != wantMode {
		t.Errorf("Mode: got %o, want %o", e.Mode, wantMode)
	}
	if e.CtimeSec != wantCtimeSec {
		t.Errorf("CtimeSec: got %d, want %d", e.CtimeSec, wantCtimeSec)
	}
	if e.CtimeNsec != wantCtimeNsec {
		t.Errorf("CtimeNsec: got %d, want %d", e.CtimeNsec, wantCtimeNsec)
	}
	if e.MtimeSec != wantMtimeSec {
		t.Errorf("MtimeSec: got %d, want %d", e.MtimeSec, wantMtimeSec)
	}
	if e.MtimeNsec != wantMtimeNsec {
		t.Errorf("MtimeNsec: got %d, want %d", e.MtimeNsec, wantMtimeNsec)
	}
	if e.Device != wantDevice {
		t.Errorf("Device: got %d, want %d", e.Device, wantDevice)
	}
	if e.Inode != wantInode {
		t.Errorf("Inode: got %d, want %d", e.Inode, wantInode)
	}
	if e.UID != wantUID {
		t.Errorf("UID: got %d, want %d", e.UID, wantUID)
	}
	if e.GID != wantGID {
		t.Errorf("GID: got %d, want %d", e.GID, wantGID)
	}
	if e.FileSize != wantFileSize {
		t.Errorf("FileSize: got %d, want %d", e.FileSize, wantFileSize)
	}
	if e.Stage != 0 {
		t.Errorf("Stage: got %d, want 0", e.Stage)
	}

	wantHashHex := "0102030405060708090a0b0c0d0e0f1011121314"
	if string(e.Hash) != wantHashHex {
		t.Errorf("Hash: got %s, want %s", e.Hash, wantHashHex)
	}

	byPath, ok := idx.ByPath[wantPath]
	if !ok {
		t.Fatalf("ByPath missing entry for %q", wantPath)
	}
	if byPath.Path != wantPath {
		t.Errorf("ByPath[%q].Path = %q, want %q", wantPath, byPath.Path, wantPath)
	}
}

// TestReadIndex_MultipleEntries builds a v2 index with three entries and checks
// that all are parsed and the ByPath map is keyed correctly.
func TestReadIndex_MultipleEntries(t *testing.T) {
	type entry struct {
		path string
		mode uint32
	}
	entries := []entry{
		{"Makefile", 0o100644},
		{"internal/gitcore/index.go", 0o100644},
		{"web/app.js", 0o100755},
	}

	gitDir := t.TempDir()
	var raw bytes.Buffer
	raw.Write(buildIndexHeader(uint32(len(entries))))
	for i, e := range entries {
		var h [20]byte
		h[0] = byte(i + 1)
		raw.Write(buildIndexEntry(e.path, h, e.mode, 0))
	}
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}

	if len(idx.Entries) != 3 {
		t.Fatalf("Entries: got %d, want 3", len(idx.Entries))
	}
	if len(idx.ByPath) != 3 {
		t.Fatalf("ByPath: got %d entries, want 3", len(idx.ByPath))
	}

	for i, want := range entries {
		t.Run(want.path, func(t *testing.T) {
			got := idx.Entries[i]
			if got.Path != want.path {
				t.Errorf("Path: got %q, want %q", got.Path, want.path)
			}
			if got.Mode != want.mode {
				t.Errorf("Mode: got %o, want %o", got.Mode, want.mode)
			}
			if got.Stage != 0 {
				t.Errorf("Stage: got %d, want 0", got.Stage)
			}

			if _, ok := idx.ByPath[want.path]; !ok {
				t.Errorf("ByPath missing %q", want.path)
			}
		})
	}
}

// TestReadIndex_InvalidMagic verifies that a file whose first 4 bytes are not
// "DIRC" is rejected with a descriptive error.
func TestReadIndex_InvalidMagic(t *testing.T) {
	gitDir := t.TempDir()

	var raw bytes.Buffer
	raw.WriteString("XXXX")
	_ = binary.Write(&raw, binary.BigEndian, uint32(2))
	_ = binary.Write(&raw, binary.BigEndian, uint32(0))
	writeIndexFile(t, gitDir, raw.Bytes())

	_, err := ReadIndex(gitDir)
	if err == nil {
		t.Fatal("expected error for invalid magic, got nil")
	}
	if !strings.Contains(err.Error(), "invalid magic") {
		t.Errorf("error %q does not mention 'invalid magic'", err.Error())
	}
}

// TestReadIndex_UnsupportedVersion verifies that index versions other than 2
// (v3 adds skip-worktree flags, v4 uses path compression) are rejected.
func TestReadIndex_UnsupportedVersion(t *testing.T) {
	for _, version := range []uint32{1, 3, 4} {
		t.Run("version", func(t *testing.T) {
			gitDir := t.TempDir()

			var raw bytes.Buffer
			raw.WriteString(indexMagic)
			_ = binary.Write(&raw, binary.BigEndian, version)
			_ = binary.Write(&raw, binary.BigEndian, uint32(0))
			writeIndexFile(t, gitDir, raw.Bytes())

			_, err := ReadIndex(gitDir)
			if err == nil {
				t.Fatalf("version %d: expected error, got nil", version)
			}
			if !strings.Contains(err.Error(), "unsupported") {
				t.Errorf("version %d: error %q does not mention 'unsupported'", version, err.Error())
			}
		})
	}
}

// TestReadIndex_TruncatedHeader verifies that a file shorter than the 12-byte
// header is rejected rather than causing a panic or wrong parse.
func TestReadIndex_TruncatedHeader(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"4 bytes (magic only)", []byte("DIRC")},
		{"8 bytes (magic + version)", append([]byte("DIRC"), 0, 0, 0, 2)},
		{"11 bytes (one short)", append([]byte("DIRC"), 0, 0, 0, 2, 0, 0, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir := t.TempDir()
			writeIndexFile(t, gitDir, tt.data)

			_, err := ReadIndex(gitDir)
			if err == nil {
				t.Fatalf("%s: expected error for truncated header, got nil", tt.name)
			}
		})
	}
}

// TestReadIndex_TruncatedEntry verifies that a header claiming 1 entry but
// providing insufficient bytes for the fixed fields is rejected cleanly.
func TestReadIndex_TruncatedEntry(t *testing.T) {
	gitDir := t.TempDir()

	var raw bytes.Buffer
	// Header says 1 entry, but we write only a partial fixed block (30 bytes
	// instead of the required 62).
	raw.Write(buildIndexHeader(1))
	raw.Write(bytes.Repeat([]byte{0x00}, 30))
	writeIndexFile(t, gitDir, raw.Bytes())

	_, err := ReadIndex(gitDir)
	if err == nil {
		t.Fatal("expected error for truncated entry, got nil")
	}
}

// TestReadIndex_StageExtraction verifies that merge-conflict stage bits (stored
// in flags bits 12-13) are decoded correctly.  Stage values 1, 2, and 3
// represent the base, ours, and theirs sides of a conflict respectively.
func TestReadIndex_StageExtraction(t *testing.T) {
	tests := []struct {
		name  string
		stage int
	}{
		{"stage 1 (base)", 1},
		{"stage 2 (ours)", 2},
		{"stage 3 (theirs)", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir := t.TempDir()
			path := "conflict.txt"

			var raw bytes.Buffer
			raw.Write(buildIndexHeader(1))
			raw.Write(buildIndexEntry(path, zeroHash, 0o100644, tt.stage))
			writeIndexFile(t, gitDir, raw.Bytes())

			idx, err := ReadIndex(gitDir)
			if err != nil {
				t.Fatalf("ReadIndex: %v", err)
			}
			if len(idx.Entries) != 1 {
				t.Fatalf("Entries: got %d, want 1", len(idx.Entries))
			}

			e := idx.Entries[0]
			if e.Stage != tt.stage {
				t.Errorf("Stage: got %d, want %d", e.Stage, tt.stage)
			}

			// Non-zero stage entries must NOT appear in ByPath.
			if _, ok := idx.ByPath[path]; ok {
				t.Errorf("ByPath must not contain stage-%d entry for %q", tt.stage, path)
			}
		})
	}
}

// TestReadIndex_LongPath verifies that paths longer than 0xFFF (4095) bytes are
// handled correctly: the flags name-length field is capped at 0xFFF per the Git
// spec, but the actual path bytes in the entry data are used verbatim.
func TestReadIndex_LongPath(t *testing.T) {
	// Create a path that exceeds the 12-bit cap in the flags field.
	longPath := strings.Repeat("a", 4100) // 4100 > 0xFFF (4095)

	gitDir := t.TempDir()
	var raw bytes.Buffer
	raw.Write(buildIndexHeader(1))
	raw.Write(buildIndexEntry(longPath, zeroHash, 0o100644, 0))
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("Entries: got %d, want 1", len(idx.Entries))
	}

	// The parser must recover the full path from the NUL terminator, not from
	// the truncated length in the flags field.
	if idx.Entries[0].Path != longPath {
		t.Errorf("Path length: got %d, want %d", len(idx.Entries[0].Path), len(longPath))
	}

	// The entry is stage-0, so it must appear in ByPath.
	if _, ok := idx.ByPath[longPath]; !ok {
		t.Error("ByPath missing long-path entry")
	}
}

// TestReadIndex_ByPathOnlyStageZero builds an index that mixes stage-0 and
// non-zero entries (as would appear during an active merge conflict) and
// confirms that ByPath contains only stage-0 entries.
func TestReadIndex_ByPathOnlyStageZero(t *testing.T) {
	gitDir := t.TempDir()

	const conflictPath = "conflict.go"
	const normalPath = "normal.go"

	// Stage-2 and stage-3 entries for the conflicted file (stage-0 absent).
	entryStage2 := buildIndexEntry(conflictPath, zeroHash, 0o100644, 2)
	entryStage3 := buildIndexEntry(conflictPath, knownHash, 0o100644, 3)

	// A normal (stage-0) entry for an unrelated file.
	entryStage0 := buildIndexEntry(normalPath, zeroHash, 0o100644, 0)

	var raw bytes.Buffer
	raw.Write(buildIndexHeader(3))
	raw.Write(entryStage2)
	raw.Write(entryStage3)
	raw.Write(entryStage0)
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}

	if len(idx.Entries) != 3 {
		t.Fatalf("Entries: got %d, want 3", len(idx.Entries))
	}

	// ByPath must contain the normal file but not the conflict path.
	if _, ok := idx.ByPath[conflictPath]; ok {
		t.Errorf("ByPath must not contain non-zero stage entry for %q", conflictPath)
	}
	if _, ok := idx.ByPath[normalPath]; !ok {
		t.Errorf("ByPath must contain stage-0 entry for %q", normalPath)
	}

	// Exactly one entry in ByPath (the stage-0 normal.go entry).
	if len(idx.ByPath) != 1 {
		t.Errorf("ByPath: got %d entries, want 1", len(idx.ByPath))
	}
}

// TestReadIndex_Alignment tests paths of varying lengths to exercise the
// 8-byte alignment padding logic across several boundary conditions.
//
// Given fixed-entry size = 62 bytes:
//
//	rawLen = 62 + len(path) + 1 (NUL)
//	paddedLen = round_up(rawLen, 8)
//
// Path lengths and their expected paddedLen values:
//
//	len=1:  rawLen=64, paddedLen=64  (already aligned)
//	len=2:  rawLen=65, paddedLen=72  (needs 7 padding bytes)
//	len=7:  rawLen=70, paddedLen=72  (needs 2 padding bytes)
//	len=9:  rawLen=72, paddedLen=72  (already aligned after 1 NUL)
//	len=10: rawLen=73, paddedLen=80  (needs 7 padding bytes)
func TestReadIndex_Alignment(t *testing.T) {
	tests := []struct {
		path       string
		wantPadded int // expected total bytes consumed per entry
	}{
		// len=1: rawLen = 62+1+1 = 64 → already a multiple of 8 → paddedLen = 64
		{path: "x", wantPadded: 64},
		// len=2: rawLen = 62+2+1 = 65 → round up to 72
		{path: "ab", wantPadded: 72},
		// len=7: rawLen = 62+7+1 = 70 → round up to 72
		{path: "foo.txt", wantPadded: 72},
		// len=9: rawLen = 62+9+1 = 72 → already aligned
		{path: "README.md", wantPadded: 72},
		// len=10: rawLen = 62+10+1 = 73 → round up to 80
		{path: "go.mod.bak", wantPadded: 80},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gitDir := t.TempDir()

			var raw bytes.Buffer
			raw.Write(buildIndexHeader(1))
			entryBytes := buildIndexEntry(tt.path, zeroHash, 0o100644, 0)
			raw.Write(entryBytes)
			writeIndexFile(t, gitDir, raw.Bytes())

			// Verify the helper itself produces the correct entry size.
			if len(entryBytes) != tt.wantPadded {
				t.Errorf("buildIndexEntry(%q): got %d bytes, want %d", tt.path, len(entryBytes), tt.wantPadded)
			}

			// Verify the parser accepts and correctly decodes the entry.
			idx, err := ReadIndex(gitDir)
			if err != nil {
				t.Fatalf("ReadIndex(%q): %v", tt.path, err)
			}
			if len(idx.Entries) != 1 {
				t.Fatalf("Entries: got %d, want 1", len(idx.Entries))
			}
			if idx.Entries[0].Path != tt.path {
				t.Errorf("Path: got %q, want %q", idx.Entries[0].Path, tt.path)
			}
		})
	}
}

// TestReadIndex_MultipleEntriesCorrectOrder verifies that entries are returned
// in the same order they appear in the binary file, which Git guarantees to be
// lexicographically sorted by path.
func TestReadIndex_MultipleEntriesCorrectOrder(t *testing.T) {
	// Paths in the order they would appear in a real Git index (lexicographic).
	paths := []string{"Makefile", "README.md", "go.mod", "go.sum", "main.go"}

	gitDir := t.TempDir()
	var raw bytes.Buffer
	raw.Write(buildIndexHeader(uint32(len(paths))))
	for _, p := range paths {
		raw.Write(buildIndexEntry(p, zeroHash, 0o100644, 0))
	}
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(idx.Entries) != len(paths) {
		t.Fatalf("Entries: got %d, want %d", len(idx.Entries), len(paths))
	}

	for i, wantPath := range paths {
		if idx.Entries[i].Path != wantPath {
			t.Errorf("Entries[%d].Path = %q, want %q", i, idx.Entries[i].Path, wantPath)
		}
	}
}

// TestReadIndex_ByPathPointerStability verifies that the pointers stored in
// ByPath actually point into the Entries slice (not stale copies from before a
// slice reallocation). This exercises the re-population loop in parseIndex.
func TestReadIndex_ByPathPointerStability(t *testing.T) {
	// Use enough entries to force at least one slice growth (capacity starts at
	// numEntries per make(), but the re-population loop must still be correct).
	paths := []string{
		"alpha.go", "beta.go", "gamma.go", "delta.go", "epsilon.go",
	}

	gitDir := t.TempDir()
	var raw bytes.Buffer
	raw.Write(buildIndexHeader(uint32(len(paths))))
	for i, p := range paths {
		var h [20]byte
		h[0] = byte(i + 10)
		raw.Write(buildIndexEntry(p, h, 0o100644, 0))
	}
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}

	// Each ByPath pointer must reference the exact same IndexEntry as Entries[i].
	for i, p := range paths {
		ptr, ok := idx.ByPath[p]
		if !ok {
			t.Errorf("ByPath missing %q", p)
			continue
		}
		// Compare the pointer address to &idx.Entries[i].
		// If ByPath was populated before the final re-stabilization, the pointer
		// would be stale and this comparison would fail.
		want := &idx.Entries[i]
		if ptr != want {
			t.Errorf("ByPath[%q] points to a stale copy; want &Entries[%d]", p, i)
		}
	}
}

// TestReadIndex_ExecutableModeFlag verifies that the mode field round-trips
// correctly for executable blobs (0o100755) versus regular files (0o100644).
func TestReadIndex_ExecutableModeFlag(t *testing.T) {
	const regularMode = uint32(0o100644)
	const executableMode = uint32(0o100755)

	gitDir := t.TempDir()
	var raw bytes.Buffer
	raw.Write(buildIndexHeader(2))
	raw.Write(buildIndexEntry("regular.sh", zeroHash, regularMode, 0))
	raw.Write(buildIndexEntry("exec.sh", knownHash, executableMode, 0))
	writeIndexFile(t, gitDir, raw.Bytes())

	idx, err := ReadIndex(gitDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(idx.Entries) != 2 {
		t.Fatalf("Entries: got %d, want 2", len(idx.Entries))
	}

	if idx.Entries[0].Mode != regularMode {
		t.Errorf("regular.sh Mode: got %o, want %o", idx.Entries[0].Mode, regularMode)
	}
	if idx.Entries[1].Mode != executableMode {
		t.Errorf("exec.sh Mode: got %o, want %o", idx.Entries[1].Mode, executableMode)
	}
}
