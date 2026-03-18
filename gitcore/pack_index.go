package gitcore

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// See: https://git-scm.com/docs/pack-format#_version_2_pack_idx_files_support_packs_larger_than_4_gib_and
const (
	packIndexV2Magic0 byte = 0xFF
	packIndexV2Magic1 byte = 0x74 // 't'
	packIndexV2Magic2 byte = 0x4F // 'O'
	packIndexV2Magic3 byte = 0x63 // 'c'
)

// In version 2 pack indices, a 32-bit offset with the high bit set indicates
// that the actual offset is >= 4 GiB and must be looked up in the large offset table.
// See: https://git-scm.com/docs/pack-format#_version_2_pack_idx_files_support_packs_larger_than_4_gib_and
const (
	packIndexLargeOffsetFlag uint32 = 0x80000000 // High bit set = large offset
	packIndexLargeOffsetMask uint32 = 0x7FFFFFFF // Mask to extract large offset table index
	maxPackObjectOffset      uint64 = ^uint64(0) >> 1
)

// PackIndex maps object hashes to their byte offsets within a pack file.
type PackIndex struct {
	path       string
	packPath   string
	version    uint32
	numObjects uint32
	fanout     [256]uint32
	offsets    map[Hash]int64
}

// NewPackIndex loads a single .idx file into a PackIndex, auto-detecting pack format.
// See: https://git-scm.com/book/en/v2/Git-Internals-Packfiles
func NewPackIndex(path string) (*PackIndex, error) {
	//nolint:gosec // G304: Pack index paths are controlled by git repository structure
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	var header [4]byte
	if _, _err := io.ReadFull(file, header[:]); _err != nil {
		return nil, fmt.Errorf("failed to read index header: %w", _err)
	}

	packPath := strings.Replace(path, ".idx", ".pack", 1)
	var idx *PackIndex

	if header[0] == packIndexV2Magic0 &&
		header[1] == packIndexV2Magic1 &&
		header[2] == packIndexV2Magic2 &&
		header[3] == packIndexV2Magic3 {
		idx, err = loadPackIndexV2(file, packPath)
	} else {
		if _, _err := file.Seek(0, io.SeekStart); _err != nil {
			return nil, fmt.Errorf("failed to seek to beginning: %w", _err)
		}
		idx, err = loadPackIndexV1(file, packPath)
	}
	if err != nil {
		return nil, err
	}

	idx.path = path
	return idx, nil
}

// FindObject looks up the byte offset of an object by its hash.
func (p *PackIndex) FindObject(id Hash) (int64, bool) {
	offset, found := p.offsets[id]
	return offset, found
}

// PackFile returns the path to the pack file associated with this index.
func (p *PackIndex) PackFile() string {
	return p.packPath
}

// Version returns the pack index format version.
func (p *PackIndex) Version() uint32 {
	return p.version
}

// NumObjects returns the number of objects stored in the pack file.
func (p *PackIndex) NumObjects() uint32 {
	return p.numObjects
}

// Fanout returns the 256-entry fanout table used for binary search within the index.
func (p *PackIndex) Fanout() [256]uint32 {
	return p.fanout
}

func loadPackIndexV1(r io.ReadSeeker, packPath string) (*PackIndex, error) {
	idx := &PackIndex{
		packPath: packPath,
		version:  1,
		offsets:  make(map[Hash]int64),
	}

	for i := 0; i < 256; i++ {
		if err := binary.Read(r, binary.BigEndian, &idx.fanout[i]); err != nil {
			return nil, fmt.Errorf("failed to read fanout[%d]: %w", i, err)
		}
	}
	idx.numObjects = idx.fanout[255]

	for i := uint32(0); i < idx.numObjects; i++ {
		var offset uint32
		if err := binary.Read(r, binary.BigEndian, &offset); err != nil {
			return nil, fmt.Errorf("failed to read offset %d: %w", i, err)
		}

		var name [20]byte
		if _, err := io.ReadFull(r, name[:]); err != nil {
			return nil, fmt.Errorf("failed to read object name %d: %w", i, err)
		}

		id, err := NewHashFromBytes(name)
		if err != nil {
			return nil, err
		}
		idx.offsets[id] = int64(offset)
	}

	return idx, nil
}

func loadPackIndexV2(rs io.ReadSeeker, packPath string) (*PackIndex, error) {
	idx := &PackIndex{
		packPath: packPath,
		version:  2,
	}

	var version uint32
	if err := binary.Read(rs, binary.BigEndian, &version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if version != 2 {
		return nil, fmt.Errorf("expected version 2, got %d", version)
	}

	for i := 0; i < 256; i++ {
		if err := binary.Read(rs, binary.BigEndian, &idx.fanout[i]); err != nil {
			return nil, fmt.Errorf("failed to read fanout[%d]: %w", i, err)
		}
	}
	idx.numObjects = idx.fanout[255]
	idx.offsets = make(map[Hash]int64, idx.numObjects)

	objectNames := make([]byte, int(idx.numObjects)*20)
	if _, err := io.ReadFull(rs, objectNames); err != nil {
		return nil, fmt.Errorf("failed to read object names: %w", err)
	}

	if _, err := rs.Seek(int64(idx.numObjects*4), io.SeekCurrent); err != nil {
		return nil, fmt.Errorf("failed to skip CRCs: %w", err)
	}

	offsets := make([]uint32, idx.numObjects)
	offsetBytes := make([]byte, int(idx.numObjects)*4)
	if _, err := io.ReadFull(rs, offsetBytes); err != nil {
		return nil, fmt.Errorf("failed to read offsets: %w", err)
	}

	largeOffsetCount := 0
	for i := uint32(0); i < idx.numObjects; i++ {
		offset := binary.BigEndian.Uint32(offsetBytes[i*4 : (i+1)*4])
		offsets[i] = offset
		if offset&packIndexLargeOffsetFlag != 0 {
			largeOffsetCount++
		}
	}

	var largeOffsets []uint64
	if largeOffsetCount > 0 {
		largeOffsets = make([]uint64, largeOffsetCount)
		largeOffsetBytes := make([]byte, largeOffsetCount*8)
		if _, err := io.ReadFull(rs, largeOffsetBytes); err != nil {
			return nil, fmt.Errorf("failed to read large offsets: %w", err)
		}
		for i := range largeOffsetCount {
			start := i * 8
			largeOffsets[i] = binary.BigEndian.Uint64(largeOffsetBytes[start : start+8])
		}
	}

	for i := uint32(0); i < idx.numObjects; i++ {
		var name [20]byte
		copy(name[:], objectNames[i*20:(i+1)*20])

		hash, err := NewHashFromBytes(name)
		if err != nil {
			return nil, err
		}

		offset := offsets[i]
		if offset&packIndexLargeOffsetFlag != 0 {
			largeOffsetIdx := offset & packIndexLargeOffsetMask
			// #nosec G115 -- largeOffsets length is bounded by pack index format (max 2^31 entries)
			if largeOffsetIdx >= uint32(len(largeOffsets)) {
				continue
			}
			largeOffset := largeOffsets[largeOffsetIdx]
			if largeOffset > maxPackObjectOffset {
				continue
			}
			idx.offsets[hash] = int64(largeOffset)
		} else {
			idx.offsets[hash] = int64(offset)
		}
	}

	return idx, nil
}
