//go:build !(darwin || linux || freebsd || netbsd || openbsd)

package gitcore

import "os"

func mapPackFile(_ *os.File, _ int64) ([]byte, error) {
	return nil, nil
}

func unmapPackData(_ []byte) error {
	return nil
}
