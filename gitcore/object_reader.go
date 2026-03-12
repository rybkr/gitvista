package gitcore

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// Objects larger than this size are rejected to prevent zip-bomb style attacks.
	maxDecompressedSize = 256 * 1024 * 1024
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
	if err == nil {
		objectType, err := objectTypeFromHeader(header)
		if err != nil {
			return nil, fmt.Errorf("unrecognized loose object type: %q for %s", header, id)
		}
		return parseObject(id, objectType, content)
	}

	return nil, fmt.Errorf("object not found: %s", id)
}

func (r *Repository) readObjectData(id Hash, depth int) ([]byte, ObjectType, error) {
	if location, found := r.packLocations[id]; found {
		return r.readPackedObjectData(location.packPath, location.offset, depth)
	}

	header, content, err := r.readLooseObjectRaw(id)
	if err == nil {
		objectType, err := objectTypeFromHeader(header)
		if err != nil {
			return nil, ObjectTypeInvalid, err
		}
		return content, objectType, nil
	}

	return nil, ObjectTypeInvalid, fmt.Errorf("object not found: %s", id)
}

func (r *Repository) readLooseObjectRaw(id Hash) (header string, content []byte, err error) {
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
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer func() { _ = zr.Close() }()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(zr, maxDecompressedSize+1)); err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}
	if buf.Len() > maxDecompressedSize {
		return nil, fmt.Errorf("decompressed object exceeds maximum allowed size (%d bytes)", maxDecompressedSize)
	}

	return buf.Bytes(), nil
}
