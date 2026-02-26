package termcolor

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether the given file descriptor refers to a terminal.
func IsTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd)) //nolint:gosec // G115: fd comes from os.File.Fd(); safe on all supported platforms
}

// ShouldColorize reports whether color output should be enabled for f.
// It returns true when f is a terminal and the NO_COLOR environment variable
// is not set. See https://no-color.org/.
func ShouldColorize(f *os.File) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return IsTerminal(f.Fd())
}
