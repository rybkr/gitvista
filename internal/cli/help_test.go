package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/termcolor"
)

func TestFormatAppHelp(t *testing.T) {
	app := NewApp("myapp", "2.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	app.Register(&Command{Name: "log", Summary: "Show commit log", Run: func([]string) int { return 0 }})
	app.Register(&Command{Name: "diff", Summary: "Show diff between commits", Run: func([]string) int { return 0 }})

	cw := termcolor.NewWriter(os.Stdout, termcolor.ColorNever)
	FormatAppHelp(app, cw)

	out := buf.String()

	checks := []string{
		"myapp version 2.0.0",
		"Usage:",
		"Commands:",
		"log",
		"Show commit log",
		"diff",
		"Show diff between commits",
		"Global flags:",
		"--color",
		"--no-color",
		"--version",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Errorf("FormatAppHelp output missing %q", s)
		}
	}
}

func TestFormatCommandHelp(t *testing.T) {
	app := NewApp("myapp", "2.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	cmd := &Command{
		Name:     "log",
		Summary:  "Show commit log",
		Usage:    "myapp log [--oneline] [-n <count>]",
		Examples: []string{"myapp log", "myapp log --oneline -n5"},
		Run:      func([]string) int { return 0 },
	}

	cw := termcolor.NewWriter(os.Stdout, termcolor.ColorNever)
	FormatCommandHelp(app, cmd, cw)

	out := buf.String()

	checks := []string{
		"log",
		"Show commit log",
		"Usage:",
		"myapp log [--oneline] [-n <count>]",
		"Examples:",
		"myapp log --oneline -n5",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Errorf("FormatCommandHelp output missing %q", s)
		}
	}
}
