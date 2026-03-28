package gitcore

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	// Objects larger than this size are rejected to prevent zip-bomb style attacks.
	maxDecompressedSize   = 256 * 1024 * 1024
	decompressScratchSize = 32 * 1024
)

var (
	maxDecompressedObjectSize = maxDecompressedSize
	zlibReaderPool            = sync.Pool{}
	decompressScratchPool     = sync.Pool{
		New: func() any {
			return make([]byte, decompressScratchSize)
		},
	}
)

func (r *Repository) readObject(id Hash) (Object, error) {
	if location, found := r.packLocations[id]; found {
		objectData, objectType, err := r.readPackedObjectData(location.packPath, location.offset, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to read pack object: %w", err)
		}
		return parseObject(id, objectType, objectData)
	}

	header, content, err := r.readLooseObjectRaw(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("object not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read loose object %s: %w", id, err)
	}

	objectType, err := objectTypeFromHeader(header)
	if err != nil {
		return nil, fmt.Errorf("unrecognized loose object type: %q for %s", header, id)
	}
	return parseObject(id, objectType, content)
}

func (r *Repository) readObjectData(id Hash, depth int) ([]byte, ObjectType, error) {
	if location, found := r.packLocations[id]; found {
		return r.readPackedObjectData(location.packPath, location.offset, depth)
	}

	header, content, err := r.readLooseObjectRaw(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ObjectTypeInvalid, fmt.Errorf("object not found: %s", id)
		}
		return nil, ObjectTypeInvalid, fmt.Errorf("failed to read loose object %s: %w", id, err)
	}

	objectType, err := objectTypeFromHeader(header)
	if err != nil {
		return nil, ObjectTypeInvalid, err
	}
	return content, objectType, nil
}

func (r *Repository) readLooseObjectRaw(id Hash) (header string, content []byte, err error) {
	if _, err := NewHash(string(id)); err != nil {
		return "", nil, fmt.Errorf("invalid object hash %q: %w", id, err)
	}

	path := filepath.Join(r.gitDir, "objects", string(id)[:2], string(id)[2:])

	//nolint:gosec // G304: Object paths are controlled by git repository structure
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := readCompressedData(file)
	if err != nil {
		return "", nil, fmt.Errorf("invalid compressed data: %w", err)
	}

	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx == -1 {
		return "", nil, fmt.Errorf("invalid object format")
	}

	header, content = string(data[:nullIdx]), data[nullIdx+1:]
	return header, content, nil
}

func readCompressedData(r io.Reader) ([]byte, error) {
	zr, err := getZlibReader(r)
	if err != nil {
		return nil, err
	}
	defer putZlibReader(zr)

	var buf bytes.Buffer
	scratch := decompressScratchPool.Get().([]byte)
	defer decompressScratchPool.Put(scratch)

	if _, err := io.CopyBuffer(&buf, io.LimitReader(zr, int64(maxDecompressedObjectSize)+1), scratch); err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}
	if buf.Len() > maxDecompressedObjectSize {
		return nil, fmt.Errorf("decompressed object exceeds maximum allowed size (%d bytes)", maxDecompressedObjectSize)
	}

	return buf.Bytes(), nil
}

func getZlibReader(r io.Reader) (io.ReadCloser, error) {
	if pooled := zlibReaderPool.Get(); pooled != nil {
		zr := pooled.(io.ReadCloser)
		resetter, ok := zr.(zlib.Resetter)
		if !ok {
			_ = zr.Close()
			return nil, fmt.Errorf("pooled zlib reader does not support reset")
		}
		if err := resetter.Reset(r, nil); err != nil {
			_ = zr.Close()
			return nil, fmt.Errorf("failed to reset zlib reader: %w", err)
		}
		return zr, nil
	}

	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	return zr, nil
}

func putZlibReader(zr io.ReadCloser) {
	if zr == nil {
		return
	}

	_ = zr.Close()
	zlibReaderPool.Put(zr)
}
