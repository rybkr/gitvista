package cli

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ANSI escape codes.
const (
	reset    = "\033[0m"
	red      = "\033[31m"
	green    = "\033[32m"
	yellow   = "\033[33m"
	cyan     = "\033[36m"
	bold     = "\033[1m"
	boldCyan = "\033[1;36m"
)

// ColorMode controls when color output is used.
type ColorMode int

const (
	// ColorAuto enables color only when writing to a terminal.
	ColorAuto ColorMode = iota
	// ColorAlways forces color output regardless of terminal detection.
	ColorAlways
	// ColorNever disables color output unconditionally.
	ColorNever
)

// ParseColorMode parses a string into a ColorMode.
// Accepted values are "auto", "always", and "never".
func ParseColorMode(s string) (ColorMode, error) {
	switch s {
	case "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	default:
		return ColorAuto, fmt.Errorf("invalid color mode %q: must be auto, always, or never", s)
	}
}

// IsTerminal reports whether the given file descriptor refers to a terminal.
func IsTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd)) // #nosec G115 -- fd comes from os.File.Fd(); safe on all supported platforms
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

// Writer wraps an io.Writer and conditionally applies ANSI color codes
// based on whether color output is enabled.
type Writer struct {
	io.Writer
	enabled bool
}

// NewWriter creates a Writer that resolves the given ColorMode against the
// file's terminal status. In ColorAuto mode, color is enabled only when f
// is a terminal and NO_COLOR is not set.
func NewWriter(f *os.File, mode ColorMode) *Writer {
	var enabled bool
	switch mode {
	case ColorAlways:
		enabled = true
	case ColorNever:
		enabled = false
	default:
		enabled = ShouldColorize(f)
	}
	return &Writer{Writer: f, enabled: enabled}
}

// Enabled reports whether color output is active.
func (w *Writer) Enabled() bool {
	return w.enabled
}

// Red returns s wrapped in red ANSI codes, or s unchanged if color is disabled.
func (w *Writer) Red(s string) string {
	if !w.enabled {
		return s
	}
	return red + s + reset
}

// Green returns s wrapped in green ANSI codes, or s unchanged if color is disabled.
func (w *Writer) Green(s string) string {
	if !w.enabled {
		return s
	}
	return green + s + reset
}

// Yellow returns s wrapped in yellow ANSI codes, or s unchanged if color is disabled.
func (w *Writer) Yellow(s string) string {
	if !w.enabled {
		return s
	}
	return yellow + s + reset
}

// Cyan returns s wrapped in cyan ANSI codes, or s unchanged if color is disabled.
func (w *Writer) Cyan(s string) string {
	if !w.enabled {
		return s
	}
	return cyan + s + reset
}

// Bold returns s wrapped in bold ANSI codes, or s unchanged if color is disabled.
func (w *Writer) Bold(s string) string {
	if !w.enabled {
		return s
	}
	return bold + s + reset
}

// BoldCyan returns s wrapped in bold cyan ANSI codes, or s unchanged if color is disabled.
func (w *Writer) BoldCyan(s string) string {
	if !w.enabled {
		return s
	}
	return boldCyan + s + reset
}
