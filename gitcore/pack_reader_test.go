package gitcore

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type failingReadSeeker struct {
	reader       *bytes.Reader
	failSeekCall map[int]error
	seekCalls    int
}

func (f *failingReadSeeker) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *failingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	f.seekCalls++
	if err, ok := f.failSeekCall[f.seekCalls]; ok {
		return 0, err
	}
	return f.reader.Seek(offset, whence)
}

type shortReader struct {
	data []byte
	pos  int
}

func (s *shortReader) Read(p []byte) (int, error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	n := copy(p, s.data[s.pos:])
	s.pos += n
	return n, nil
}

func packHeader(objectType ObjectType, size int64) []byte {
	firstByte := (packObjectTypeNibble(objectType) << 4) | byte(size&0x0F)
	size >>= 4
	out := []byte{firstByte}
	if size > 0 {
		out[0] |= 0x80
	}
	for size > 0 {
		b := byte(size & 0x7F)
		size >>= 7
		if size > 0 {
			b |= 0x80
		}
		out = append(out, b)
	}
	return out
}

func packObjectTypeNibble(objectType ObjectType) byte {
	if objectType < 0 || objectType > ObjectTypeRefDelta {
		return 0
	}
	//nolint:gosec // Test helper bounds-checks the enum before narrowing to a pack header nibble.
	return byte(objectType)
}

func packObjectBytes(t *testing.T, objectType ObjectType, body []byte) []byte {
	t.Helper()
	out := append([]byte{}, packHeader(objectType, int64(len(body)))...)
	out = append(out, compressBytes(t, body)...)
	return out
}

func packRefDeltaObject(t *testing.T, base Hash, delta []byte) []byte {
	t.Helper()
	rawHash, err := hex.DecodeString(string(base))
	if err != nil {
		t.Fatalf("decode base hash: %v", err)
	}
	out := append([]byte{}, packHeader(ObjectTypeRefDelta, int64(len(delta)))...)
	out = append(out, rawHash...)
	out = append(out, compressBytes(t, delta)...)
	return out
}

func encodeDeltaOffset(distance int64) []byte {
	out := []byte{byte(distance & 0x7F)}
	distance >>= 7
	for distance > 0 {
		distance--
		out = append(out, byte(distance&0x7F)|0x80)
		distance >>= 7
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func encodeVarInt(n int64) []byte {
	var out []byte
	for {
		b := byte(n & 0x7F)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}

func deltaCopyThenInsert(insert []byte) []byte {
	const baseSize int64 = 5
	out := append(encodeVarInt(baseSize), encodeVarInt(baseSize+int64(len(insert)))...)
	//nolint:gosec // Test data keeps baseSize and insert length within a single-byte delta instruction.
	out = append(out, 0x90, byte(baseSize), byte(len(insert)))
	out = append(out, insert...)
	return out
}

func TestPackIndexFilesAndRepositoryPackLoading(t *testing.T) {
	hash1 := hashFromHex(testHash1)
	hash2 := hashFromHex(testHash2)

	var v1 bytes.Buffer
	var fanout [256]uint32
	for i := 0x11; i <= 0x22; i++ {
		fanout[i] = 1
	}
	fanout[0xff] = 2
	for i := 0; i < 256; i++ {
		writeUint32BE(&v1, fanout[i])
	}
	writeUint32BE(&v1, 12)
	v1.Write(hash1[:])
	writeUint32BE(&v1, 34)
	v1.Write(hash2[:])

	packDir := filepath.Join(t.TempDir(), "objects", "pack")
	if err := os.MkdirAll(packDir, 0o750); err != nil {
		t.Fatal(err)
	}
	v1Path := filepath.Join(packDir, "one.idx")
	if err := os.WriteFile(v1Path, v1.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPackIndex(filepath.Join(packDir, "missing.idx")); err == nil {
		t.Fatal("expected missing index error")
	}
	idx, err := NewPackIndex(v1Path)
	if err != nil || idx.path != v1Path {
		t.Fatalf("NewPackIndex v1: %+v %v", idx, err)
	}
	truncatedPath := filepath.Join(packDir, "truncated.idx")
	if err := os.WriteFile(truncatedPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPackIndex(truncatedPath); err == nil {
		t.Fatal("expected truncated header error")
	}

	var badV2 bytes.Buffer
	badV2.Write([]byte{packIndexV2Magic0, packIndexV2Magic1, packIndexV2Magic2, packIndexV2Magic3})
	writeUint32BE(&badV2, 3)
	badV2Path := filepath.Join(packDir, "bad-v2.idx")
	if err := os.WriteFile(badV2Path, badV2.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPackIndex(badV2Path); err == nil {
		t.Fatal("expected invalid v2 version error")
	}

	repo := NewEmptyRepository()
	repo.gitDir = filepath.Dir(filepath.Dir(packDir))
	if err := os.Mkdir(filepath.Join(packDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.loadPackIndices(); err == nil {
		t.Fatal("expected joined pack index loading error")
	}
	if len(repo.packIndices) != 1 {
		t.Fatalf("expected one valid pack index, got %d", len(repo.packIndices))
	}

	noPackRepo := NewEmptyRepository()
	noPackRepo.gitDir = t.TempDir()
	if err := noPackRepo.loadPackIndices(); err != nil {
		t.Fatalf("missing pack dir should be ignored: %v", err)
	}

	badPackRepo := NewEmptyRepository()
	badPackRepo.gitDir = t.TempDir()
	writeTextFile(t, filepath.Join(badPackRepo.gitDir, "objects", "pack"), "not a directory")
	if err := badPackRepo.loadPackIndices(); err == nil {
		t.Fatal("expected non-directory pack path error")
	}
}

func TestPackReaderAndPackedObjectAccess(t *testing.T) {
	packPath := filepath.Join(t.TempDir(), "blob.pack")
	if err := os.WriteFile(packPath, packObjectBytes(t, ObjectTypeBlob, []byte("hello")), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := NewEmptyRepository()
	reader1, err := repo.packReader(packPath)
	if err != nil {
		t.Fatalf("packReader: %v", err)
	}
	reader2, err := repo.packReader(packPath)
	if err != nil {
		t.Fatalf("packReader cached: %v", err)
	}
	if reader1 != reader2 {
		t.Fatal("expected packReader to reuse cached reader")
	}
	if len(reader1.data) == 0 {
		t.Fatal("expected packReader to map pack file data")
	}
	if _, packReaderErr := repo.packReader(filepath.Join(t.TempDir(), "missing.pack")); packReaderErr == nil {
		t.Fatal("expected missing pack file error")
	}

	data, typ, err := repo.readPackedObjectData(packPath, 0, 0)
	if err != nil || typ != ObjectTypeBlob || string(data) != "hello" {
		t.Fatalf("readPackedObjectData: %q %v %v", string(data), typ, err)
	}
	if _, _, readErr := repo.readPackedObjectData(packPath, -1, 0); readErr == nil {
		t.Fatal("expected invalid pack seek error")
	}

	id := mustHash(t, testHash1)
	repo.packLocations[id] = PackLocation{packPath: packPath, offset: 0}
	obj, err := repo.readObject(id)
	if err != nil || obj.Type() != ObjectTypeBlob {
		t.Fatalf("readObject packed: %v %v", obj, err)
	}
	data, typ, err = repo.readObjectData(id, 0)
	if err != nil || typ != ObjectTypeBlob || string(data) != "hello" {
		t.Fatalf("readObjectData packed: %q %v %v", string(data), typ, err)
	}

	repo.packReaders = map[string]*PackReader{
		packPath: {file: reader1.file, size: reader1.size},
	}
	data, typ, err = repo.readPackedObjectData(packPath, 0, 0)
	if err != nil || typ != ObjectTypeBlob || string(data) != "hello" {
		t.Fatalf("readPackedObjectData section reader path: %q %v %v", string(data), typ, err)
	}
	if _, _, err := repo.readPackedObjectData(packPath, -1, 0); err == nil {
		t.Fatal("expected invalid section-reader seek error")
	}
}

func TestReadPackObjectDataAndDeltaBranches(t *testing.T) {
	if _, _, err := readPackObjectData(&failingReadSeeker{
		reader:       bytes.NewReader(packObjectBytes(t, ObjectTypeBlob, []byte("x"))),
		failSeekCall: map[int]error{1: errors.New("boom")},
	}, nil, 0); err == nil {
		t.Fatal("expected initial seek error")
	}

	for _, tc := range []struct {
		name string
		typ  ObjectType
		body []byte
	}{
		{name: "commit", typ: ObjectTypeCommit, body: []byte("tree " + testHash1 + "\nauthor Jane Doe <jane@example.com> 1700000000 +0000\ncommitter Jane Doe <jane@example.com> 1700000000 +0000\n\nmsg")},
		{name: "tree", typ: ObjectTypeTree, body: func() []byte {
			hash := hashFromHex(testHash1)
			return append([]byte("100644 file"), append([]byte{0}, hash[:]...)...)
		}()},
		{name: "blob", typ: ObjectTypeBlob, body: []byte("blob")},
		{name: "tag", typ: ObjectTypeTag, body: []byte("object " + testHash1 + "\ntype blob\ntag x\ntagger Jane Doe <jane@example.com> 1700000000 +0000\n\nmsg")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, typ, err := readPackObjectData(bytes.NewReader(packObjectBytes(t, tc.typ, tc.body)), nil, 0)
			if err != nil || typ != tc.typ || len(data) == 0 {
				t.Fatalf("readPackObjectData %s: len=%d typ=%v err=%v", tc.name, len(data), typ, err)
			}
		})
	}

	if _, _, err := readPackObjectData(bytes.NewReader(packHeader(ObjectTypeReserved, 0)), nil, 0); err == nil {
		t.Fatal("expected unsupported pack object type")
	}
	if _, _, err := readPackObjectData(bytes.NewReader(packRefDeltaObject(t, mustHash(t, testHash1), deltaCopyThenInsert([]byte("!")))), func(id Hash, depth int) ([]byte, ObjectType, error) {
		if id != Hash(testHash1) || depth != 1 {
			t.Fatalf("unexpected resolver inputs: %v depth=%d", id, depth)
		}
		return []byte("hello"), ObjectTypeBlob, nil
	}, 0); err != nil {
		t.Fatalf("readPackObjectData ref delta: %v", err)
	}
}

func TestReadOffsetDeltaAndReadRefDeltaErrors(t *testing.T) {
	if _, _, err := readOffsetDelta(bytes.NewReader(nil), 1, 0, nil, 0); err == nil {
		t.Fatal("expected offset delta read error")
	}
	if _, _, err := readOffsetDelta(bytes.NewReader([]byte{0, 'b', 'a', 'd'}), 1, 1, nil, 0); err == nil {
		t.Fatal("expected offset delta compressed data error")
	}

	base := packObjectBytes(t, ObjectTypeBlob, []byte("hello"))
	badDeltaPayload := append([]byte{6, 1, 1, '!'}, []byte{}...)
	var pack bytes.Buffer
	pack.Write(base)
	deltaStart := pack.Len()
	pack.Write(packHeader(ObjectTypeOfsDelta, int64(len(badDeltaPayload))))
	pack.Write(encodeDeltaOffset(int64(deltaStart)))
	pack.Write(compressBytes(t, badDeltaPayload))
	if _, _, err := readPackObjectData(bytes.NewReader(pack.Bytes()[deltaStart:]), nil, 0); err == nil {
		t.Fatal("expected offset delta base read/apply error")
	}

	goodDelta := deltaCopyThenInsert([]byte(" Git!"))
	stream := packRefDeltaObject(t, mustHash(t, testHash1), goodDelta)
	data, typ, err := readRefDelta(bytes.NewReader(stream[len(packHeader(ObjectTypeRefDelta, int64(len(goodDelta)))):]), int64(len(goodDelta)), func(id Hash, depth int) ([]byte, ObjectType, error) {
		return []byte("hello"), ObjectTypeBlob, nil
	}, 0)
	if err != nil || typ != ObjectTypeBlob || string(data) != "hello Git!" {
		t.Fatalf("readRefDelta success: %q %v %v", string(data), typ, err)
	}

	if _, _, err := readRefDelta(bytes.NewReader([]byte{1, 2, 3}), 1, nil, 0); err == nil {
		t.Fatal("expected base hash read error")
	}
	shortHash := make([]byte, 20)
	copy(shortHash, bytes.Repeat([]byte{0x11}, 20))
	if _, _, err := readRefDelta(bytes.NewReader(append(shortHash, []byte("bad")...)), 1, nil, 0); err == nil {
		t.Fatal("expected ref delta compressed data error")
	}
	refDelta := deltaCopyThenInsert([]byte("!"))
	if _, _, err := readRefDelta(bytes.NewReader(append(shortHash, compressBytes(t, refDelta)...)), int64(len(refDelta)), func(id Hash, depth int) ([]byte, ObjectType, error) {
		return nil, 0, errors.New("resolve failed")
	}, 0); err == nil {
		t.Fatal("expected ref delta resolve error")
	}
	if _, _, err := readRefDelta(bytes.NewReader(append(shortHash, compressBytes(t, []byte{4, 6, 0x90, 0x04, 0x01, '!'})...)), 6, func(id Hash, depth int) ([]byte, ObjectType, error) {
		return []byte("hello"), ObjectTypeBlob, nil
	}, 0); err == nil {
		t.Fatal("expected ref delta apply error")
	}
}

func TestReadOffsetDelta_SeekFailures(t *testing.T) {
	deltaPayload := deltaCopyThenInsert([]byte("!"))
	deltaBody := append([]byte{1}, compressBytes(t, deltaPayload)...)

	t.Run("before-delta-seek-fails", func(t *testing.T) {
		reader := &failingReadSeeker{
			reader:       bytes.NewReader(deltaBody),
			failSeekCall: map[int]error{1: errors.New("seek-current failed")},
		}
		if _, _, err := readOffsetDelta(reader, int64(len(deltaPayload)), 1, nil, 0); err == nil {
			t.Fatal("expected before-delta seek error")
		}
	})

	t.Run("seek-back-after-base-read-fails", func(t *testing.T) {
		reader := &failingReadSeeker{
			reader:       bytes.NewReader(deltaBody),
			failSeekCall: map[int]error{2: errors.New("seek-back failed")},
		}
		if _, _, err := readOffsetDelta(reader, int64(len(deltaPayload)), 1, func(id Hash, depth int) ([]byte, ObjectType, error) {
			return nil, 0, nil
		}, 0); err == nil {
			t.Fatal("expected after-delta seek error")
		}
	})
}

func TestReadRefDelta_CurrentPositionSeekFailure(t *testing.T) {
	base := mustHash(t, testHash1)
	delta := deltaCopyThenInsert([]byte("!"))
	stream := packRefDeltaObject(t, base, delta)
	reader := &failingReadSeeker{
		reader:       bytes.NewReader(stream[len(packHeader(ObjectTypeRefDelta, int64(len(delta)))):]),
		failSeekCall: map[int]error{1: errors.New("seek-current failed")},
	}
	if _, _, err := readRefDelta(reader, int64(len(delta)), func(id Hash, depth int) ([]byte, ObjectType, error) {
		return []byte("hello"), ObjectTypeBlob, nil
	}, 0); err == nil {
		t.Fatal("expected before-delta seek error")
	}
}

func TestReadOffsetDelta_RemainingErrorBranches(t *testing.T) {
	t.Run("offset continuation read error", func(t *testing.T) {
		if _, _, err := readOffsetDelta(bytes.NewReader([]byte{0x80}), 1, 1, nil, 0); err == nil {
			t.Fatal("expected offset continuation read error")
		}
	})

	t.Run("base read error", func(t *testing.T) {
		deltaPayload := deltaCopyThenInsert([]byte("!"))
		stream := append([]byte{0x80}, byte(1))
		stream = append(stream, compressBytes(t, deltaPayload)...)
		reader := bytes.NewReader(stream)
		if _, err := reader.Seek(1, io.SeekStart); err != nil {
			t.Fatalf("Seek(): %v", err)
		}
		if _, _, err := readOffsetDelta(reader, int64(len(deltaPayload)), 1, nil, 0); err == nil {
			t.Fatal("expected base read error")
		}
	})

	t.Run("apply delta error", func(t *testing.T) {
		base := packObjectBytes(t, ObjectTypeBlob, []byte("hello"))
		badDelta := []byte{5, 2, 1, '!'}
		stream := append(append([]byte{}, base...), byte(len(base)))
		stream = append(stream, compressBytes(t, badDelta)...)
		reader := bytes.NewReader(stream)
		if _, err := reader.Seek(int64(len(base)), io.SeekStart); err != nil {
			t.Fatalf("Seek(): %v", err)
		}
		if _, _, err := readOffsetDelta(reader, int64(len(badDelta)), int64(len(base)), nil, 0); err == nil {
			t.Fatal("expected delta apply error")
		}
	})
}

func TestReadHelpersAndApplyDeltaBranches(t *testing.T) {
	if _, _, err := readPackObjectHeader(bytes.NewReader(nil)); err == nil {
		t.Fatal("expected pack header EOF error")
	}
	if _, err := readVarInt(bytes.NewReader(nil)); err == nil {
		t.Fatal("expected varint EOF error")
	}

	base := bytes.Repeat([]byte{'x'}, 0x10000)
	delta := append(encodeVarInt(int64(len(base))), encodeVarInt(int64(len(base)))...)
	delta = append(delta, 0x80)
	result, err := applyDelta(base, delta)
	if err != nil || len(result) != len(base) {
		t.Fatalf("applyDelta zero-size copy: len=%d err=%v", len(result), err)
	}

	badTarget := []byte{5, 10, 5, 'h', 'e', 'l', 'l', 'o'}
	if _, err := applyDelta([]byte("hello"), badTarget); err == nil {
		t.Fatal("expected result size mismatch")
	}
	if _, err := applyDelta([]byte("hello"), []byte{5, 1, 1}); err == nil {
		t.Fatal("expected delta insert read error")
	}
}
