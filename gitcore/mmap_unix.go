//go:build darwin || linux || freebsd || netbsd || openbsd

package gitcore

import (
	"os"
	"syscall"
)

func mapPackFile(file *os.File, size int64) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}

	return syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
}

func unmapPackData(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	return syscall.Munmap(data)
}
