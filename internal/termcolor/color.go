// Package termcolor provides ANSI color output with automatic TTY detection
// and support for the NO_COLOR convention (https://no-color.org/).
package termcolor

import "fmt"

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
