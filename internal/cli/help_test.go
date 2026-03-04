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

	if strings.Count(out, "Show commit log") != 1 {
		t.Errorf("expected exactly one 'Show commit log' entry, got %d", strings.Count(out, "Show commit log"))
	}
	if strings.Count(out, "Show diff between commits") != 1 {
		t.Errorf("expected exactly one 'Show diff between commits' entry, got %d", strings.Count(out, "Show diff between commits"))
	}

	usageIdx := strings.Index(out, "Usage:")
	commandsIdx := strings.Index(out, "Commands:")
	flagsIdx := strings.Index(out, "Global flags:")
	if usageIdx == -1 || commandsIdx == -1 || flagsIdx == -1 {
		t.Fatalf("help output missing required section header(s)")
	}
	// Usage should appear first; the order of commands and global flags is
	// intentionally presentation-specific.
	if usageIdx > commandsIdx || usageIdx > flagsIdx {
		t.Errorf("unexpected section ordering: Usage=%d Commands=%d Global flags=%d", usageIdx, commandsIdx, flagsIdx)
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

	if strings.Count(out, "Examples:") != 1 {
		t.Errorf("expected exactly one Examples section, got %d", strings.Count(out, "Examples:"))
	}
	if strings.Count(out, "\n  myapp log\n") != 1 {
		t.Errorf("expected exactly one indented base example line")
	}
	if strings.Count(out, "\n  myapp log --oneline -n5\n") != 1 {
		t.Errorf("expected exactly one indented detailed example line")
	}

	usageIdx := strings.Index(out, "Usage:")
	examplesIdx := strings.Index(out, "Examples:")
	if usageIdx == -1 || examplesIdx == -1 {
		t.Fatalf("command help missing Usage or Examples section")
	}
	if usageIdx > examplesIdx {
		t.Errorf("unexpected section ordering: Usage appears after Examples")
	}
}
