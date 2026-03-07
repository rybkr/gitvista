package gitcore

// Pack index v2 magic number bytes: "\377tOc" (\377 = 0xFF in octal)
// See: https://git-scm.com/docs/pack-format#_version_2_pack_idx_files_support_packs_larger_than_4_gib_and
const (
	packIndexV2Magic0 byte = 0xFF
	packIndexV2Magic1 byte = 0x74 // 't'
	packIndexV2Magic2 byte = 0x4F // 'O'
	packIndexV2Magic3 byte = 0x63 // 'c'
)

// Pack index v2 large offset constants.
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

// Offsets returns a defensive copy of the offset map.
func (p *PackIndex) Offsets() map[Hash]int64 {
	cp := make(map[Hash]int64, len(p.offsets))
	for k, v := range p.offsets {
		cp[k] = v
	}
	return cp
}
