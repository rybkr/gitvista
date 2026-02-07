package gitcore

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// writeUint32BE writes a uint32 in big-endian to the buffer.
func writeUint32BE(buf *bytes.Buffer, v uint32) {
	binary.Write(buf, binary.BigEndian, v)
}

// writeUint64BE writes a uint64 in big-endian to the buffer.
func writeUint64BE(buf *bytes.Buffer, v uint64) {
	binary.Write(buf, binary.BigEndian, v)
}

// hashFromHex returns a 20-byte array from a 40-char hex string.
func hashFromHex(s string) [20]byte {
	b, _ := hex.DecodeString(s)
	var h [20]byte
	copy(h[:], b)
	return h
}

func TestLoadPackIndexV1(t *testing.T) {
	// Build a V1 index with 2 objects.
	// V1 format: 256 fanout uint32s, then [offset uint32][name 20 bytes] pairs.
	hash1 := hashFromHex("0a0b0c0d0e0f1011121314151617181920212223")
	hash2 := hashFromHex("ff0b0c0d0e0f1011121314151617181920212223")

	var buf bytes.Buffer

	// Fanout table: hash1 starts with 0x0a, hash2 starts with 0xff.
	var fanout [256]uint32
	for i := 0x0a; i < 0xff; i++ {
		fanout[i] = 1
	}
	fanout[0xff] = 2 // total count
	for i := 0; i < 256; i++ {
		writeUint32BE(&buf, fanout[i])
	}

	// Object entries: offset + 20-byte name
	writeUint32BE(&buf, 100) // offset for hash1
	buf.Write(hash1[:])
	writeUint32BE(&buf, 200) // offset for hash2
	buf.Write(hash2[:])

	idx, err := loadPackIndexV1(bytes.NewReader(buf.Bytes()), "test.pack")
	if err != nil {
		t.Fatalf("loadPackIndexV1 failed: %v", err)
	}

	if idx.Version() != 1 {
		t.Errorf("expected version 1, got %d", idx.Version())
	}
	if idx.NumObjects() != 2 {
		t.Errorf("expected 2 objects, got %d", idx.NumObjects())
	}
	if idx.PackFile() != "test.pack" {
		t.Errorf("expected packPath 'test.pack', got %q", idx.PackFile())
	}

	hash1Str, _ := NewHashFromBytes(hash1)
	hash2Str, _ := NewHashFromBytes(hash2)

	off1, ok := idx.FindObject(hash1Str)
	if !ok || off1 != 100 {
		t.Errorf("expected offset 100 for hash1, got %d (found=%v)", off1, ok)
	}
	off2, ok := idx.FindObject(hash2Str)
	if !ok || off2 != 200 {
		t.Errorf("expected offset 200 for hash2, got %d (found=%v)", off2, ok)
	}

	fa := idx.Fanout()
	if fa[0xff] != 2 {
		t.Errorf("expected fanout[255]=2, got %d", fa[0xff])
	}
}

func TestLoadPackIndexV2(t *testing.T) {
	// V2 format (after magic+version already consumed by caller):
	// version uint32 (=2), fanout[256], names, CRCs, offsets
	hash1 := hashFromHex("0a0b0c0d0e0f1011121314151617181920212223")
	hash2 := hashFromHex("ff0b0c0d0e0f1011121314151617181920212223")

	var buf bytes.Buffer

	// Version
	writeUint32BE(&buf, 2)

	// Fanout
	var fanout [256]uint32
	for i := 0x0a; i < 0xff; i++ {
		fanout[i] = 1
	}
	fanout[0xff] = 2
	for i := 0; i < 256; i++ {
		writeUint32BE(&buf, fanout[i])
	}

	// Object names (sorted)
	buf.Write(hash1[:])
	buf.Write(hash2[:])

	// CRCs (4 bytes each, we don't validate them)
	writeUint32BE(&buf, 0xDEADBEEF)
	writeUint32BE(&buf, 0xCAFEBABE)

	// 4-byte offsets (no MSB set = no large offset)
	writeUint32BE(&buf, 300)
	writeUint32BE(&buf, 400)

	idx, err := loadPackIndexV2(bytes.NewReader(buf.Bytes()), "test.pack")
	if err != nil {
		t.Fatalf("loadPackIndexV2 failed: %v", err)
	}

	if idx.Version() != 2 {
		t.Errorf("expected version 2, got %d", idx.Version())
	}
	if idx.NumObjects() != 2 {
		t.Errorf("expected 2 objects, got %d", idx.NumObjects())
	}

	hash1Str, _ := NewHashFromBytes(hash1)
	hash2Str, _ := NewHashFromBytes(hash2)

	off1, ok := idx.FindObject(hash1Str)
	if !ok || off1 != 300 {
		t.Errorf("expected offset 300 for hash1, got %d (found=%v)", off1, ok)
	}
	off2, ok := idx.FindObject(hash2Str)
	if !ok || off2 != 400 {
		t.Errorf("expected offset 400 for hash2, got %d (found=%v)", off2, ok)
	}
}

func TestLoadPackIndexV2_LargeOffsets(t *testing.T) {
	hash1 := hashFromHex("0a0b0c0d0e0f1011121314151617181920212223")

	var buf bytes.Buffer

	// Version
	writeUint32BE(&buf, 2)

	// Fanout: 1 object, hash starts with 0x0a
	var fanout [256]uint32
	for i := 0x0a; i <= 0xff; i++ {
		fanout[i] = 1
	}
	for i := 0; i < 256; i++ {
		writeUint32BE(&buf, fanout[i])
	}

	// Object name
	buf.Write(hash1[:])

	// CRC
	writeUint32BE(&buf, 0)

	// 4-byte offset with MSB set → index 0 into large offset table
	writeUint32BE(&buf, 0x80000000)

	// Large offset table: one 8-byte entry
	writeUint64BE(&buf, 5000000000) // 5 GB offset

	idx, err := loadPackIndexV2(bytes.NewReader(buf.Bytes()), "test.pack")
	if err != nil {
		t.Fatalf("loadPackIndexV2 with large offsets failed: %v", err)
	}

	hash1Str, _ := NewHashFromBytes(hash1)
	off, ok := idx.FindObject(hash1Str)
	if !ok || off != 5000000000 {
		t.Errorf("expected large offset 5000000000, got %d (found=%v)", off, ok)
	}
}

func TestReadPackObjectHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantType byte
		wantSize int64
	}{
		{
			name:     "single byte, type=1 (commit), size=5",
			input:    []byte{0x15}, // 0 | 001 | 0101 → type=1, size=5
			wantType: 1,
			wantSize: 5,
		},
		{
			name: "multi byte, type=2 (tree), size=0x124",
			// First byte: 1 | 010 | 0100 → continue=1, type=2, size low 4 bits = 4
			// Second byte: 0 | 0010010 → continue=0, value = 0x12
			// size = 4 | (0x12 << 4) = 4 | 288 = 292 = 0x124
			input:    []byte{0xA4, 0x12},
			wantType: 2,
			wantSize: 0x124,
		},
		{
			name: "three bytes, type=3 (blob), large size",
			// First byte: 1 | 011 | 1111 → continue=1, type=3, low 4 = 15
			// Second byte: 1 | 1111111 → continue=1, value = 127
			// Third byte:  0 | 0000001 → continue=0, value = 1
			// size = 15 | (127 << 4) | (1 << 11) = 15 + 2032 + 2048 = 4095
			input:    []byte{0xBF, 0xFF, 0x01},
			wantType: 3,
			wantSize: 4095,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objType, size, err := readPackObjectHeader(bytes.NewReader(tt.input))
			if err != nil {
				t.Fatalf("readPackObjectHeader failed: %v", err)
			}
			if objType != tt.wantType {
				t.Errorf("type: got %d, want %d", objType, tt.wantType)
			}
			if size != tt.wantSize {
				t.Errorf("size: got %d, want %d", size, tt.wantSize)
			}
		})
	}
}

func TestApplyDelta(t *testing.T) {
	base := []byte("Hello, World!")

	// Build a delta that copies "Hello" from base and adds " Git!" as new data.
	var delta bytes.Buffer

	// Source size varint (13)
	delta.WriteByte(13)
	// Target size varint (10 = len("Hello Git!"))
	delta.WriteByte(10)

	// Copy instruction: copy 5 bytes from offset 0
	// cmd byte: 1 | 000 | 0 | 001 | 0 | 1 = 0x91
	// bit 0 set → offset byte follows (0)
	// bit 4 set → size byte follows (5)
	delta.WriteByte(0x91)
	delta.WriteByte(0x00) // offset = 0
	delta.WriteByte(0x05) // size = 5

	// Add instruction: add 5 bytes " Git!"
	delta.WriteByte(0x05)
	delta.Write([]byte(" Git!"))

	result, err := applyDelta(base, delta.Bytes())
	if err != nil {
		t.Fatalf("applyDelta failed: %v", err)
	}

	expected := "Hello Git!"
	if string(result) != expected {
		t.Errorf("got %q, want %q", string(result), expected)
	}
}

func TestApplyDelta_BaseSizeMismatch(t *testing.T) {
	base := []byte("short")

	// Delta claims source size is 100
	var delta bytes.Buffer
	delta.WriteByte(100) // srcSize = 100 (but base is 5)
	delta.WriteByte(5)   // targetSize = 5

	_, err := applyDelta(base, delta.Bytes())
	if err == nil {
		t.Fatal("expected error for base size mismatch")
	}
}

func TestApplyDelta_InvalidCommand0(t *testing.T) {
	base := []byte("test")

	var delta bytes.Buffer
	delta.WriteByte(4) // srcSize = 4
	delta.WriteByte(4) // targetSize = 4
	delta.WriteByte(0) // invalid command

	_, err := applyDelta(base, delta.Bytes())
	if err == nil {
		t.Fatal("expected error for invalid command 0")
	}
}

func TestApplyDelta_CopyExceedsBase(t *testing.T) {
	base := []byte("ab")

	var delta bytes.Buffer
	delta.WriteByte(2)  // srcSize = 2
	delta.WriteByte(10) // targetSize = 10

	// Copy 10 bytes from offset 0 (but base is only 2 bytes)
	delta.WriteByte(0x91) // copy, offset byte + size byte
	delta.WriteByte(0x00) // offset = 0
	delta.WriteByte(0x0A) // size = 10

	_, err := applyDelta(base, delta.Bytes())
	if err == nil {
		t.Fatal("expected error for copy exceeding base size")
	}
}

func TestReadVarInt(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  int64
	}{
		{
			name:  "single byte, value 50",
			input: []byte{50},
			want:  50,
		},
		{
			name:  "single byte, value 0",
			input: []byte{0},
			want:  0,
		},
		{
			name:  "single byte, max (127)",
			input: []byte{0x7F},
			want:  127,
		},
		{
			name: "two bytes, value 128",
			// 128 = 0 | (1 << 7) → first byte: 0x80 (continue, value=0), second: 0x01 (stop, value=1)
			input: []byte{0x80, 0x01},
			want:  128,
		},
		{
			name: "two bytes, value 300",
			// 300 = 0b100101100 = 0b0101100 | (0b10 << 7)
			// first byte: 1 | 0101100 = 0xAC, second byte: 0 | 0000010 = 0x02
			input: []byte{0xAC, 0x02},
			want:  300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.input)
			got, err := readVarInt(reader)
			if err != nil {
				t.Fatalf("readVarInt failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReadCompressedObject(t *testing.T) {
	data := []byte("hello compressed world")

	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(data)
	w.Close()

	result, err := readCompressedObject(bytes.NewReader(compressed.Bytes()), int64(len(data)))
	if err != nil {
		t.Fatalf("readCompressedObject failed: %v", err)
	}
	if !bytes.Equal(result, data) {
		t.Errorf("got %q, want %q", result, data)
	}
}

func TestReadCompressedObject_SizeMismatch(t *testing.T) {
	data := []byte("hello")

	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(data)
	w.Close()

	_, err := readCompressedObject(bytes.NewReader(compressed.Bytes()), 999)
	if err == nil {
		t.Fatal("expected error for size mismatch")
	}
}
