//go:build unix

package gitcore

import (
	"fmt"
	"math"
	"os"
	"syscall"
)

func mapPackFile(file *os.File, size int64) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}
	if size < 0 || size > math.MaxInt {
		return nil, fmt.Errorf("pack file too large to map: %d", size)
	}
	fd := file.Fd()
	if fd > math.MaxInt {
		return nil, fmt.Errorf("file descriptor out of range: %d", fd)
	}

	return syscall.Mmap(int(fd), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
}

func unmapPackData(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	return syscall.Munmap(data)
}
