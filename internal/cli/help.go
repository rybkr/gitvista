package cli

import (
	"fmt"
	"io"
)

// fpf is a shorthand for fmt.Fprintf that discards the error, used for
// writing help text to stderr where write failures are non-actionable.
func fpf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...) //nolint:gosec // CLI stderr, not web output
}

// FormatAppHelp writes the top-level help text to app.Stderr.
func FormatAppHelp(app *App, cw *Writer) {
	w := app.Stderr

	fpf(w, "%s version %s\n\n", app.Name, app.Version)
	newSectionPrinter(w).Println(cw.Bold("Usage:"))
	fpf(w, "  %s [global flags] <command> [<args>]\n\n", app.Name)

	newSectionPrinter(w).Println(cw.Bold("Global flags:"))
	fpf(w, "  %s   Color output: auto, always, never\n", cw.Flag("--color=<mode>"))
	fpf(w, "  %s        Disable color output\n", cw.Flag("--no-color"))
	fpf(w, "  %s         Show version and exit\n\n", cw.Flag("--version"))

	newSectionPrinter(w).Println(cw.Bold("Commands:"))

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
		fpf(w, "  %s  %s\n", cw.Command(fmt.Sprintf("%-*s", maxLen, n)), cmd.Summary)
	}

	fpf(w, "\nRun '%s help <command>' for more information on a command.\n", app.Name)
}

// FormatCommandHelp writes per-command help text to app.Stderr.
func FormatCommandHelp(app *App, cmd *Command, cw *Writer) {
	w := app.Stderr

	fpf(w, "%s — %s\n\n", cw.Command(cmd.Name), cmd.Summary)

	if cmd.Usage != "" {
		newSectionPrinter(w).Println(cw.Bold("Usage:"))
		fpf(w, "  %s\n", cmd.Usage)
	}

	if len(cmd.Examples) > 0 {
		fpf(w, "\n")
		newSectionPrinter(w).Println(cw.Bold("Examples:"))
		for _, ex := range cmd.Examples {
			fpf(w, "  %s\n", ex)
		}
	}
}
