package termcolor

import (
	"io"
	"os"
)

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
