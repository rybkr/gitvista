package cli

import (
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"golang.org/x/term"
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

var (
	commandStyle  = pterm.NewStyle(pterm.Bold, pterm.FgLightCyan)
	emphasisStyle = pterm.NewStyle(pterm.Bold)
	flagStyle     = pterm.NewStyle(pterm.FgLightCyan)
	successStyle  = pterm.NewStyle(pterm.FgGreen)
	warningStyle  = pterm.NewStyle(pterm.FgYellow)
	errorStyle    = pterm.NewStyle(pterm.FgLightRed)
	accentStyle   = pterm.NewStyle(pterm.FgCyan)
	mutedStyle    = pterm.NewStyle(pterm.FgDarkGray)
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

// Writer applies the GitVista CLI presentation theme to inline strings.
type Writer struct {
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

	configureTheme(enabled)

	return &Writer{enabled: enabled}
}

// Enabled reports whether color output is active.
func (w *Writer) Enabled() bool {
	return w.enabled
}

// Bold returns s wrapped in the emphasis style, or s unchanged if color is disabled.
func (w *Writer) Bold(s string) string {
	return w.apply(emphasisStyle, s)
}

// BoldCyan returns s wrapped in the command/title style, or s unchanged if color is disabled.
func (w *Writer) BoldCyan(s string) string {
	return w.apply(commandStyle, s)
}

// Cyan returns s wrapped in the accent style, or s unchanged if color is disabled.
func (w *Writer) Cyan(s string) string {
	return w.apply(accentStyle, s)
}

// Green returns s wrapped in the success style, or s unchanged if color is disabled.
func (w *Writer) Green(s string) string {
	return w.apply(successStyle, s)
}

// Red returns s wrapped in the error style, or s unchanged if color is disabled.
func (w *Writer) Red(s string) string {
	return w.apply(errorStyle, s)
}

// Yellow returns s wrapped in the warning style, or s unchanged if color is disabled.
func (w *Writer) Yellow(s string) string {
	return w.apply(warningStyle, s)
}

// Command returns s wrapped in the command/title style.
func (w *Writer) Command(s string) string {
	return w.apply(commandStyle, s)
}

// Flag returns s wrapped in the flag style.
func (w *Writer) Flag(s string) string {
	return w.apply(flagStyle, s)
}

// Muted returns s wrapped in the muted style.
func (w *Writer) Muted(s string) string {
	return w.apply(mutedStyle, s)
}

func (w *Writer) apply(style *pterm.Style, s string) string {
	if !w.enabled || style == nil {
		return s
	}
	return style.Sprint(s)
}

func configureTheme(enabled bool) {
	theme := pterm.ThemeDefault
	theme.PrimaryStyle = pterm.Style{pterm.FgLightCyan}
	theme.SecondaryStyle = pterm.Style{pterm.FgDarkGray}
	theme.HighlightStyle = pterm.Style{pterm.Bold, pterm.FgLightCyan}
	theme.InfoMessageStyle = pterm.Style{pterm.FgDefault}
	theme.InfoPrefixStyle = pterm.Style{pterm.Bold, pterm.FgCyan}
	theme.SuccessMessageStyle = pterm.Style{pterm.FgDefault}
	theme.SuccessPrefixStyle = pterm.Style{pterm.Bold, pterm.FgGreen}
	theme.WarningMessageStyle = pterm.Style{pterm.FgDefault}
	theme.WarningPrefixStyle = pterm.Style{pterm.Bold, pterm.FgYellow}
	theme.ErrorMessageStyle = pterm.Style{pterm.FgDefault}
	theme.ErrorPrefixStyle = pterm.Style{pterm.Bold, pterm.FgLightRed}
	theme.FatalMessageStyle = pterm.Style{pterm.FgDefault}
	theme.FatalPrefixStyle = pterm.Style{pterm.Bold, pterm.FgLightRed}
	theme.DescriptionMessageStyle = pterm.Style{pterm.FgDefault}
	theme.DescriptionPrefixStyle = pterm.Style{pterm.FgDarkGray}
	theme.ScopeStyle = pterm.Style{pterm.FgDarkGray}
	theme.ProgressbarBarStyle = pterm.Style{pterm.FgCyan}
	theme.ProgressbarTitleStyle = pterm.Style{pterm.FgLightCyan}
	theme.SpinnerStyle = pterm.Style{pterm.FgCyan}
	theme.SpinnerTextStyle = pterm.Style{pterm.FgDefault}
	theme.TimerStyle = pterm.Style{pterm.FgDarkGray}
	theme.TableStyle = pterm.Style{pterm.FgDefault}
	theme.TableHeaderStyle = pterm.Style{pterm.Bold, pterm.FgLightCyan}
	theme.TableSeparatorStyle = pterm.Style{pterm.FgDarkGray}
	theme.SectionStyle = pterm.Style{pterm.Bold, pterm.FgLightCyan}
	theme.BulletListTextStyle = pterm.Style{pterm.FgDefault}
	theme.BulletListBulletStyle = pterm.Style{pterm.FgDarkGray}
	theme.TreeStyle = pterm.Style{pterm.FgDarkGray}
	theme.TreeTextStyle = pterm.Style{pterm.FgDefault}
	theme.DebugMessageStyle = pterm.Style{pterm.FgDarkGray}
	theme.DebugPrefixStyle = pterm.Style{pterm.FgDarkGray}
	theme.BoxStyle = pterm.Style{pterm.FgDarkGray}
	theme.BoxTextStyle = pterm.Style{pterm.FgDefault}
	theme.BarLabelStyle = pterm.Style{pterm.FgLightCyan}
	theme.BarStyle = pterm.Style{pterm.FgCyan}
	pterm.ThemeDefault = theme

	if enabled {
		pterm.EnableColor()
		return
	}
	pterm.DisableColor()
}
