package gitcore

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Git index file constants.
const (
	// indexMagic is the 4-byte signature that begins every .git/index file.
	indexMagic = "DIRC"

	// indexFixedEntrySize is the number of bytes occupied by the fixed-size
	// fields of each index entry (ctime through flags, inclusive), before the
	// variable-length null-terminated path begins.
	//
	// Breakdown:
	//   ctime_sec   4
	//   ctime_nsec  4
	//   mtime_sec   4
	//   mtime_nsec  4
	//   device      4
	//   inode       4
	//   mode        4
	//   uid         4
	//   gid         4
	//   file_size   4
	//   sha1       20
	//   flags       2
	//   total      62
	indexFixedEntrySize = 62

	// indexEntryAlignment is the boundary to which each entry's total length
	// (fixed fields + path + NUL + padding) must be a multiple of.
	indexEntryAlignment = 8

	// indexFlagStageMask isolates bits 12-13 of the flags field, which encode
	// the merge stage (0=normal, 1=base, 2=ours, 3=theirs).
	indexFlagStageMask = 0x3000

	// indexFlagStageShift is the bit-shift to extract the stage value from flags.
	indexFlagStageShift = 12
)

// IndexEntry represents a single entry in the Git index (staging area).
// The index stores the cached stat information and blob hash for each tracked
// file so that Git can quickly detect which files have changed on disk.
type IndexEntry struct {
	CtimeSec  uint32
	CtimeNsec uint32
	MtimeSec  uint32
	MtimeNsec uint32
	Device    uint32
	Inode     uint32
	// Mode encodes the file type and permissions, e.g. 0100644 (regular),
	// 0100755 (executable), 0120000 (symlink), 0160000 (gitlink/submodule).
	Mode     uint32
	UID      uint32
	GID      uint32
	FileSize uint32
	// Hash is the SHA-1 of the blob object that the index records for this path.
	Hash  Hash
	Flags uint16
	// Stage is the merge conflict stage extracted from flags bits 12-13.
	// 0 = normal (not in a merge conflict), 1 = base, 2 = ours, 3 = theirs.
	Stage int
	// Path is the null-terminated path of the file, relative to the repo root.
	Path string
}

// Index represents the parsed .git/index file (the staging area / cache).
type Index struct {
	Version uint32
	Entries []IndexEntry
	// ByPath provides O(1) lookup by path. Only stage-0 (non-conflicted) entries
	// are stored here; during a merge conflict multiple entries share the same
	// path at stages 1-3, and the stage-0 entry is absent until conflict resolution.
	ByPath map[string]*IndexEntry
}

// ReadIndex parses the .git/index file inside gitDir and returns a structured
// Index. Only version 2 of the index format is fully supported; versions 3 and
// 4 return an error because they introduce extensions that alter the wire layout.
//
// If the index file does not exist (e.g., a freshly initialized repository that
// has never had anything staged), ReadIndex returns an empty Index with no error.
// This matches the semantic of "nothing is staged yet" rather than a hard failure.
func ReadIndex(gitDir string) (*Index, error) {
	indexPath := filepath.Join(gitDir, "index")

	//nolint:gosec // G304: index path is derived from the git directory, which is caller-controlled
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Fresh repository with no staged files — return an empty but valid Index.
			return &Index{ByPath: make(map[string]*IndexEntry)}, nil
		}
		return nil, fmt.Errorf("ReadIndex: reading index file: %w", err)
	}

	idx, err := parseIndex(data)
	if err != nil {
		return nil, fmt.Errorf("ReadIndex: %w", err)
	}
	return idx, nil
}

// parseIndex decodes the raw bytes of a .git/index file into an Index.
// All multi-byte integers are big-endian as per the Git index specification.
func parseIndex(data []byte) (*Index, error) {
	// Header: 4-byte magic + 4-byte version + 4-byte entry count = 12 bytes
	const headerSize = 12
	if len(data) < headerSize {
		return nil, fmt.Errorf("file too short to contain a valid header (%d bytes)", len(data))
	}

	if string(data[:4]) != indexMagic {
		return nil, fmt.Errorf("invalid magic signature: expected %q, got %q", indexMagic, string(data[:4]))
	}

	version := binary.BigEndian.Uint32(data[4:8])
	// Version 3 adds a skip-worktree flag; version 4 uses path-prefix compression.
	// Both formats change how entries are laid out, so we reject them until
	// explicit support is added.
	if version != 2 {
		return nil, fmt.Errorf("unsupported index version %d (only version 2 is supported)", version)
	}

	numEntries := binary.BigEndian.Uint32(data[8:12])

	idx := &Index{
		Version: version,
		Entries: make([]IndexEntry, 0, numEntries),
		ByPath:  make(map[string]*IndexEntry, numEntries),
	}

	// Entries: parse numEntries variable-length records starting at offset 12.
	offset := headerSize
	for i := range numEntries {
		entry, bytesConsumed, err := parseIndexEntry(data, offset)
		if err != nil {
			return nil, fmt.Errorf("entry %d at offset %d: %w", i, offset, err)
		}

		idx.Entries = append(idx.Entries, entry)

		// ByPath only holds the canonical (stage-0) entry for each path.
		// During a merge conflict, stage-0 is absent and stages 1-3 hold the
		// ancestor, ours, and theirs versions respectively — we skip those.
		if entry.Stage == 0 {
			// Use a pointer into the slice so ByPath doesn't copy the struct.
			// We re-slice after the loop to ensure pointer stability.
			idx.ByPath[entry.Path] = &idx.Entries[len(idx.Entries)-1]
		}

		offset += bytesConsumed
	}

	// Re-establish pointer stability: the slice may have been reallocated by
	// append, so we need to re-populate ByPath from the final backing array.
	// This is safe because we're done appending.
	for i := range idx.Entries {
		if idx.Entries[i].Stage == 0 {
			idx.ByPath[idx.Entries[i].Path] = &idx.Entries[i]
		}
	}

	return idx, nil
}

// parseIndexEntry decodes one index entry from data starting at startOffset.
// It returns the entry and the total number of bytes consumed (fixed fields +
// path + NUL terminator + alignment padding).
func parseIndexEntry(data []byte, startOffset int) (IndexEntry, int, error) {
	// Ensure there is enough room for the fixed-size portion of the entry.
	if startOffset+indexFixedEntrySize > len(data) {
		return IndexEntry{}, 0, fmt.Errorf(
			"not enough data for fixed entry fields: need %d bytes, have %d",
			indexFixedEntrySize, len(data)-startOffset,
		)
	}

	p := data[startOffset:]

	var entry IndexEntry

	// Read the 10 fixed uint32 fields (40 bytes) in order.
	entry.CtimeSec = binary.BigEndian.Uint32(p[0:4])
	entry.CtimeNsec = binary.BigEndian.Uint32(p[4:8])
	entry.MtimeSec = binary.BigEndian.Uint32(p[8:12])
	entry.MtimeNsec = binary.BigEndian.Uint32(p[12:16])
	entry.Device = binary.BigEndian.Uint32(p[16:20])
	entry.Inode = binary.BigEndian.Uint32(p[20:24])
	entry.Mode = binary.BigEndian.Uint32(p[24:28])
	entry.UID = binary.BigEndian.Uint32(p[28:32])
	entry.GID = binary.BigEndian.Uint32(p[32:36])
	entry.FileSize = binary.BigEndian.Uint32(p[36:40])

	// SHA-1 hash occupies bytes 40-59 (20 bytes).
	hashHex := hex.EncodeToString(p[40:60])
	hash, err := NewHash(hashHex)
	if err != nil {
		// This should never fail since hex.EncodeToString always produces valid hex.
		return IndexEntry{}, 0, fmt.Errorf("invalid blob hash: %w", err)
	}
	entry.Hash = hash

	// Flags field at bytes 60-61 (2 bytes big-endian).
	entry.Flags = binary.BigEndian.Uint16(p[60:62])

	// Extract the merge stage from flags bits 12-13.
	entry.Stage = int((entry.Flags & indexFlagStageMask) >> indexFlagStageShift)

	// Variable-length null-terminated path, starting at byte 62.
	pathStart := startOffset + indexFixedEntrySize
	nullIdx := -1
	for i := pathStart; i < len(data); i++ {
		if data[i] == 0 {
			nullIdx = i
			break
		}
	}
	if nullIdx == -1 {
		return IndexEntry{}, 0, fmt.Errorf("null terminator not found for path starting at offset %d", pathStart)
	}

	entry.Path = string(data[pathStart:nullIdx])

	// Alignment padding: the total entry length (fixed + path + NUL + padding)
	// must be a multiple of indexEntryAlignment (8 bytes). The NUL terminator
	// is 1 byte; at least one NUL must be present, and additional NULs are
	// added so the total is divisible by 8.
	//
	// Git computes the padded end as:
	//   padded_end = startOffset + round_up(fixedSize + pathLen + 1, 8)
	// where pathLen is the number of path bytes (not counting the NUL).
	pathLen := nullIdx - pathStart              // bytes in the path, not counting the NUL
	rawLen := indexFixedEntrySize + pathLen + 1 // +1 for NUL terminator
	// Round up rawLen to the next multiple of indexEntryAlignment.
	// Formula: (rawLen + alignment - 1) & ^(alignment - 1)
	paddedLen := (rawLen + indexEntryAlignment - 1) &^ (indexEntryAlignment - 1)

	totalConsumed := paddedLen

	if startOffset+totalConsumed > len(data) {
		return IndexEntry{}, 0, fmt.Errorf(
			"entry extends beyond end of data: offset %d + paddedLen %d > fileLen %d",
			startOffset, totalConsumed, len(data),
		)
	}

	return entry, totalConsumed, nil
}
