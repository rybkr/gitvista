package gitcore

import (
	"encoding/hex"
	"fmt"
)

// Hash represents a 40-character hex-encoded SHA-1 Git object identifier.
// See: https://git-scm.com/docs/git-hash-object
type Hash string

// NewHash creates a Hash from a 40-character hex string, returning an error if invalid.
func NewHash(s string) (Hash, error) {
	if len(s) != 40 {
		return "", fmt.Errorf("invalid hash length: %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("invalid hash %q: %w", s, err)
	}
	return Hash(s), nil
}

// NewHashFromBytes creates a Hash from a 20-byte array.
func NewHashFromBytes(b [20]byte) (Hash, error) {
	return Hash(hex.EncodeToString(b[:])), nil
}

// Short returns the first 7 characters of the hash, or the full hash if shorter.
func (h Hash) Short() string {
	if len(h) < 7 {
		return string(h)
	}
	return string(h)[:7]
}
