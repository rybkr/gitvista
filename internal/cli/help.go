package cli

import (
	"fmt"
	"io"

	"github.com/rybkr/gitvista/internal/termcolor"
)

// fpf is a shorthand for fmt.Fprintf that discards the error, used for
// writing help text to stderr where write failures are non-actionable.
func fpf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...) //nolint:gosec // CLI stderr, not web output
}

// FormatAppHelp writes the top-level help text to app.Stderr.
func FormatAppHelp(app *App, cw *termcolor.Writer) {
	w := app.Stderr

	fpf(w, "%s version %s\n\n", app.Name, app.Version)
	fpf(w, "%s\n", cw.Bold("Usage:"))
	fpf(w, "  %s [global flags] <command> [<args>]\n\n", app.Name)

	fpf(w, "%s\n", cw.Bold("Global flags:"))
	fpf(w, "  %s   Color output: auto, always, never\n", cw.Yellow("--color=<mode>"))
	fpf(w, "  %s        Disable color output\n", cw.Yellow("--no-color"))
	fpf(w, "  %s         Show version and exit\n\n", cw.Yellow("--version"))

	fpf(w, "%s\n", cw.Bold("Commands:"))

	names := app.CommandNames()

	// Find max name length for alignment.
	maxLen := 0
	for _, n := range names {
		if len(n) > maxLen {
			maxLen = len(n)
		}
	}

	for _, n := range names {
		cmd := app.Lookup(n)
		fpf(w, "  %s  %s\n", cw.BoldCyan(fmt.Sprintf("%-*s", maxLen, n)), cmd.Summary)
	}

	fpf(w, "\nRun '%s help <command>' for more information on a command.\n", app.Name)
}

// FormatCommandHelp writes per-command help text to app.Stderr.
func FormatCommandHelp(app *App, cmd *Command, cw *termcolor.Writer) {
	w := app.Stderr

	fpf(w, "%s — %s\n\n", cw.BoldCyan(cmd.Name), cmd.Summary)

	if cmd.Usage != "" {
		fpf(w, "%s\n", cw.Bold("Usage:"))
		fpf(w, "  %s\n", cmd.Usage)
	}

	if len(cmd.Examples) > 0 {
		fpf(w, "\n%s\n", cw.Bold("Examples:"))
		for _, ex := range cmd.Examples {
			fpf(w, "  %s\n", ex)
		}
	}
}
