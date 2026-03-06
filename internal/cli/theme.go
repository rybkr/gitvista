package cli

import (
	"io"

	"github.com/pterm/pterm"
)

func newSectionPrinter(w io.Writer) *pterm.SectionPrinter {
	return pterm.DefaultSection.
		WithWriter(w).
		WithLevel(0).
		WithTopPadding(0).
		WithBottomPadding(0)
}
