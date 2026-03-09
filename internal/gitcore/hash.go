package gitcore

import (
	"fmt"
)

// Hash represents a 40-character hex-encoded SHA-1 Git object identifier.
type Hash string

// NewHash creates a Hash from a 40-character hex string, returning an error if invalid.
func NewHash(s string) (Hash, error) {
	if len(s) != 40 {
		return "", fmt.Errorf("invalid hash length: %d", len(s))
	}
	for i := range 40 {
		if fromHexChar(s[i]) < 0 {
			return "", fmt.Errorf("invalid hash: %q", s)
		}
	}
	return Hash(s), nil
}

// NewHashFromBytes creates a Hash from a 20-byte array.
func NewHashFromBytes(b [20]byte) (Hash, error) {
	var encoded [40]byte
	for i, v := range b {
		encoded[i*2] = toHexChar(v >> 4)
		encoded[i*2+1] = toHexChar(v & 0x0f)
	}
	return Hash(encoded[:]), nil
}

// Short returns the first 7 characters of the hash, or the full hash if shorter.
func (h Hash) Short() string {
	if len(h) < 7 {
		return string(h)
	}
	return string(h)[:7]
}

func fromHexChar(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	default:
		return -1
	}
}

func toHexChar(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'a' + (n - 10)
}
