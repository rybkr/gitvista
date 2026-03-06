package cli

import (
	"os"

	"github.com/pterm/pterm"
)

// Spinner displays an animated braille spinner on stderr while a long-running
// operation is in progress. It is only displayed when stderr is a TTY;
// in non-interactive environments (piped output, CI, E2E tests) it is silent.
type Spinner struct {
	msg     string
	printer *pterm.SpinnerPrinter
}

// NewSpinner creates a Spinner that will display msg alongside the animation.
func NewSpinner(msg string) *Spinner {
	return &Spinner{
		msg: msg,
	}
}

// Start begins the spinner animation in a background goroutine.
// It writes to stderr so it never pollutes stdout.
func (s *Spinner) Start() {
	if !IsTerminal(os.Stderr.Fd()) {
		return
	}
	printer, err := pterm.DefaultSpinner.
		WithWriter(os.Stderr).
		WithShowTimer(false).
		WithRemoveWhenDone(true).
		Start(s.msg)
	if err != nil {
		return
	}
	s.printer = printer
}

// Stop halts the spinner animation and clears the line.
func (s *Spinner) Stop() {
	if s.printer == nil {
		return
	}
	_ = s.printer.Stop()
	s.printer = nil
}
