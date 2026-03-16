package gitcore

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	indexMagic          = "DIRC"
	indexFixedEntrySize = 62
	indexEntryAlignment = 8
	indexFlagStageMask  = 0x3000
	indexFlagStageShift = 12
)

// IndexEntry represents a single entry in the Git index.
type IndexEntry struct {
	CtimeSec  uint32
	CtimeNsec uint32
	MtimeSec  uint32
	MtimeNsec uint32
	Device    uint32
	Inode     uint32
	Mode      uint32
	UID       uint32
	GID       uint32
	FileSize  uint32
	Hash      Hash
	Flags     uint16
	Stage     int
	Path      string
}

// Index represents the parsed .git/index file.
type Index struct {
	Version uint32
	Entries []IndexEntry
	ByPath  map[string]*IndexEntry
}

// ReadIndex parses the .git/index file inside gitDir.
func ReadIndex(gitDir string) (*Index, error) {
	indexPath := filepath.Join(gitDir, "index")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
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

func parseIndex(data []byte) (*Index, error) {
	const headerSize = 12
	if len(data) < headerSize {
		return nil, fmt.Errorf("file too short to contain a valid header (%d bytes)", len(data))
	}
	if string(data[:4]) != indexMagic {
		return nil, fmt.Errorf("invalid magic signature: expected %q, got %q", indexMagic, string(data[:4]))
	}

	version := binary.BigEndian.Uint32(data[4:8])
	if version != 2 {
		return nil, fmt.Errorf("unsupported index version %d (only version 2 is supported)", version)
	}

	numEntries := binary.BigEndian.Uint32(data[8:12])
	idx := &Index{
		Version: version,
		Entries: make([]IndexEntry, 0, numEntries),
		ByPath:  make(map[string]*IndexEntry, numEntries),
	}

	offset := headerSize
	for i := range numEntries {
		entry, bytesConsumed, err := parseIndexEntry(data, offset)
		if err != nil {
			return nil, fmt.Errorf("entry %d at offset %d: %w", i, offset, err)
		}

		idx.Entries = append(idx.Entries, entry)
		if entry.Stage == 0 {
			idx.ByPath[entry.Path] = &idx.Entries[len(idx.Entries)-1]
		}
		offset += bytesConsumed
	}

	for i := range idx.Entries {
		if idx.Entries[i].Stage == 0 {
			idx.ByPath[idx.Entries[i].Path] = &idx.Entries[i]
		}
	}

	return idx, nil
}

func parseIndexEntry(data []byte, startOffset int) (IndexEntry, int, error) {
	if startOffset+indexFixedEntrySize > len(data) {
		return IndexEntry{}, 0, fmt.Errorf(
			"not enough data for fixed entry fields: need %d bytes, have %d",
			indexFixedEntrySize, len(data)-startOffset,
		)
	}

	p := data[startOffset:]
	var entry IndexEntry

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

	hashHex := hex.EncodeToString(p[40:60])
	hash, err := NewHash(hashHex)
	if err != nil {
		return IndexEntry{}, 0, fmt.Errorf("invalid blob hash: %w", err)
	}
	entry.Hash = hash

	entry.Flags = binary.BigEndian.Uint16(p[60:62])
	entry.Stage = int((entry.Flags & indexFlagStageMask) >> indexFlagStageShift)

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
	pathLen := nullIdx - pathStart
	rawLen := indexFixedEntrySize + pathLen + 1
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
