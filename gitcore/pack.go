package gitcore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		offsets:  make(map[Hash]int64),
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

	objectNames := make([][20]byte, idx.numObjects)
	for i := uint32(0); i < idx.numObjects; i++ {
		if _, err := io.ReadFull(rs, objectNames[i][:]); err != nil {
			return nil, fmt.Errorf("failed to read object name %d: %w", i, err)
		}
	}

	if _, err := rs.Seek(int64(idx.numObjects*4), io.SeekCurrent); err != nil {
		return nil, fmt.Errorf("failed to skip CRCs: %w", err)
	}

	offsets := make([]uint32, idx.numObjects)
	for i := uint32(0); i < idx.numObjects; i++ {
		if err := binary.Read(rs, binary.BigEndian, &offsets[i]); err != nil {
			return nil, fmt.Errorf("failed to read offset %d: %w", i, err)
		}
	}

	var largeOffsets []uint64
	for _, offset := range offsets {
		if offset&packIndexLargeOffsetFlag != 0 {
			if len(largeOffsets) == 0 {
				for {
					var largeOffset uint64
					err := binary.Read(rs, binary.BigEndian, &largeOffset)
					if err == io.EOF {
						break
					}
					if err != nil {
						return nil, fmt.Errorf("failed to read large offset: %w", err)
					}
					largeOffsets = append(largeOffsets, largeOffset)
				}
			}
		}
	}

	for i := uint32(0); i < idx.numObjects; i++ {
		hash, err := NewHashFromBytes(objectNames[i])
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

// PackReader caches an open pack file and its size so object reads can reuse
// the same file descriptor instead of reopening the pack on every lookup.
type PackReader struct {
	file *os.File
	size int64
}

// PackLocation identifies where a packed object is stored within a pack file.
type PackLocation struct {
	packPath string
	offset   int64
}

const (
	maxDeltaDepth = 50
)

var (
	// ErrDeltaChainTooDeep occurs when the recursive pack delta checker exceeds maxDeltaDepth.
	ErrDeltaChainTooDeep = fmt.Errorf("delta chain exceeds maximum depth of %d", maxDeltaDepth)
)

func (r *Repository) loadPackIndices() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	packDir := filepath.Join(r.gitDir, "objects", "pack")
	if _, err := os.Stat(packDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	entries, err := os.ReadDir(packDir)
	if err != nil {
		return fmt.Errorf("failed to read pack directory: %w", err)
	}

	var loadErrs []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".idx") {
			continue
		}

		idxPath := filepath.Join(packDir, entry.Name())
		idx, err := NewPackIndex(idxPath)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Errorf("loading pack index %s: %w", entry.Name(), err))
			continue
		}

		r.packIndices = append(r.packIndices, idx)
		for hash, offset := range idx.offsets {
			if _, exists := r.packLocations[hash]; !exists {
				r.packLocations[hash] = PackLocation{
					packPath: idx.PackFile(),
					offset:   offset,
				}
			}
		}
	}

	return errors.Join(loadErrs...)
}

func (r *Repository) readPackedObjectData(path string, offset int64, depth int) ([]byte, ObjectType, error) {
	reader, err := r.packReader(path)
	if err != nil {
		return nil, ObjectTypeInvalid, err
	}

	sr := io.NewSectionReader(reader.file, 0, reader.size)
	if _, err := sr.Seek(offset, io.SeekStart); err != nil {
		return nil, ObjectTypeInvalid, err
	}

	return readPackObjectData(sr, r.readObjectData, depth)
}

func (r *Repository) packReader(path string) (*PackReader, error) {
	r.packReadersMu.Lock()
	defer r.packReadersMu.Unlock()

	if reader, ok := r.packReaders[path]; ok {
		return reader, nil
	}

	//nolint:gosec // G304: Pack file paths are controlled by git repository structure
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	reader := &PackReader{file: file, size: info.Size()}
	r.packReaders[path] = reader
	return reader, nil
}

func readPackObjectData(rs io.ReadSeeker, resolve ObjectResolver, depth int) (data []byte, objectType ObjectType, err error) {
	if depth > maxDeltaDepth {
		return nil, 0, ErrDeltaChainTooDeep
	}

	objStart, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}

	objType, size, err := readPackObjectHeader(rs)
	if err != nil {
		return nil, 0, err
	}

	switch objType {
	case ObjectTypeCommit, ObjectTypeTree, ObjectTypeBlob, ObjectTypeTag:
		data, err := readCompressedObject(rs, size)
		return data, objType, err
	case ObjectTypeOfsDelta:
		return readOffsetDelta(rs, size, objStart, resolve, depth)
	case ObjectTypeRefDelta:
		return readRefDelta(rs, size, resolve, depth)
	default:
		return nil, 0, fmt.Errorf("unsupported object type: %d", objType)
	}
}

func readPackObjectHeader(r io.Reader) (objectType ObjectType, size int64, err error) {
	var b [1]byte
	if _, err := r.Read(b[:]); err != nil {
		return ObjectTypeInvalid, 0, err
	}

	objectType = ObjectType((b[0] >> 4) & 0x07)
	size = int64(b[0] & 0x0F)
	shift := 4

	for b[0]&0x80 != 0 {
		if _, err := r.Read(b[:]); err != nil {
			return ObjectTypeInvalid, 0, err
		}
		size |= int64(b[0]&0x7F) << shift
		shift += 7
	}

	return objectType, size, nil
}

func readCompressedObject(r io.Reader, expectedSize int64) ([]byte, error) {
	content, err := readCompressedData(r)
	if err != nil {
		return nil, fmt.Errorf("invalid compressed data: %w", err)
	}

	if int64(len(content)) != expectedSize {
		return nil, fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, len(content))
	}
	return content, nil
}

func readOffsetDelta(rs io.ReadSeeker, size, objStart int64, resolve ObjectResolver, depth int) ([]byte, ObjectType, error) {
	var b [1]byte

	if _, err := rs.Read(b[:]); err != nil {
		return nil, 0, err
	}
	offset := int64(b[0] & 0x7F)
	for b[0]&0x80 != 0 {
		if _, err := rs.Read(b[:]); err != nil {
			return nil, 0, err
		}
		offset = ((offset + 1) << 7) | int64(b[0]&0x7F)
	}

	beforeDelta, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}
	deltaData, err := readCompressedObject(rs, size)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read offset delta data at %d: %w", beforeDelta, err)
	}

	afterDelta, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}

	basePos := objStart - offset
	if _, _err := rs.Seek(basePos, io.SeekStart); _err != nil {
		return nil, 0, fmt.Errorf("failed to seek to base object at %d: %w", basePos, _err)
	}
	baseData, baseType, err := readPackObjectData(rs, resolve, depth+1)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read base object at %d (type %d): %w", basePos, baseType, err)
	}
	if _, _err := rs.Seek(afterDelta, io.SeekStart); _err != nil {
		return nil, 0, _err
	}

	result, err := applyDelta(baseData, deltaData)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to apply offset delta: %w", err)
	}

	return result, baseType, nil
}

func readRefDelta(rs io.ReadSeeker, size int64, resolve ObjectResolver, depth int) ([]byte, ObjectType, error) {
	var baseHash [20]byte
	if _, err := io.ReadFull(rs, baseHash[:]); err != nil {
		return nil, 0, fmt.Errorf("failed to read base hash: %w", err)
	}
	baseHashStr, err := NewHashFromBytes(baseHash)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid hash: %w", err)
	}

	beforeDelta, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}
	deltaData, err := readCompressedObject(rs, size)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read ref delta data at %d: %w", beforeDelta, err)
	}

	baseData, baseType, err := resolve(baseHashStr, depth+1)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read base object %s: %w", baseHashStr.Short(), err)
	}

	result, err := applyDelta(baseData, deltaData)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to apply ref delta: %w", err)
	}

	return result, baseType, nil
}

// See: https://git-scm.com/docs/pack-format#_deltified_representation
func applyDelta(base []byte, delta []byte) ([]byte, error) {
	src := bytes.NewReader(delta)

	srcSize, err := readVarInt(src)
	if err != nil {
		return nil, err
	}
	if srcSize != int64(len(base)) {
		return nil, fmt.Errorf("base size mismatch: expected %d, got %d", srcSize, len(base))
	}

	targetSize, err := readVarInt(src)
	if err != nil {
		return nil, err
	}

	result := make([]byte, 0, targetSize)

	for {
		var cmd [1]byte
		_, err := src.Read(cmd[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if cmd[0]&0x80 != 0 {
			var offset, size int64

			for i := 0; i < 4; i++ {
				if cmd[0]&(0x01<<i) != 0 {
					var b [1]byte
					if _, err := src.Read(b[:]); err != nil {
						return nil, err
					}
					offset |= int64(b[0]) << (8 * i)
				}
			}

			for i := 0; i < 3; i++ {
				if cmd[0]&(0x10<<i) != 0 {
					var b [1]byte
					if _, err := src.Read(b[:]); err != nil {
						return nil, err
					}
					size |= int64(b[0]) << (8 * i)
				}
			}

			// "Size zero is automatically converted to 0x10000."
			if size == 0 {
				size = 0x10000
			}
			if offset+size > int64(len(base)) {
				return nil, fmt.Errorf("copy of %d exceeds base size of %d", offset+size, int64(len(base)))
			}
			result = append(result, base[offset:offset+size]...)

		} else if cmd[0] != 0 {
			size := int(cmd[0] & 0x7F)
			if size == 0 {
				return nil, fmt.Errorf("copy of size zero is illegal")
			}
			data := make([]byte, size)
			if _, err := io.ReadFull(src, data); err != nil {
				return nil, err
			}
			result = append(result, data...)

		} else {
			return nil, fmt.Errorf("invalid delta command: 0")
		}
	}

	if int64(len(result)) != targetSize {
		return nil, fmt.Errorf("result size mismatch: expected %d, got %d", targetSize, len(result))
	}

	return result, nil
}

func readVarInt(src *bytes.Reader) (int64, error) {
	var result int64
	var shift uint

	for {
		var b [1]byte
		if _, err := src.Read(b[:]); err != nil {
			return 0, err
		}
		result |= int64(b[0]&0x7F) << shift
		shift += 7
		if b[0]&0x80 == 0 {
			break
		}
	}

	return result, nil
}
