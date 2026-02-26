package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/termcolor"
)

func noColorWriter() *termcolor.Writer {
	return termcolor.NewWriter(os.Stdout, termcolor.ColorNever)
}

func TestRunDispatchesToCorrectCommand(t *testing.T) {
	app := NewApp("test", "1.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	called := ""
	app.Register(&Command{
		Name:    "log",
		Summary: "Show commit log",
		Run:     func(args []string) int { called = "log"; return 0 },
	})
	app.Register(&Command{
		Name:    "diff",
		Summary: "Show diff",
		Run:     func(args []string) int { called = "diff"; return 0 },
	})

	code := app.Run([]string{"diff", "--stat"}, noColorWriter())
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if called != "diff" {
		t.Fatalf("expected 'diff' command to be called, got %q", called)
	}
}

func TestRunPassesSubArgs(t *testing.T) {
	app := NewApp("test", "1.0.0")
	app.Stderr = &bytes.Buffer{}

	var got []string
	app.Register(&Command{
		Name:    "log",
		Summary: "Show log",
		Run:     func(args []string) int { got = args; return 0 },
	})

	app.Run([]string{"log", "--oneline", "-n5"}, noColorWriter())
	if len(got) != 2 || got[0] != "--oneline" || got[1] != "-n5" {
		t.Fatalf("expected [--oneline -n5], got %v", got)
	}
}

func TestRunEmptyArgs(t *testing.T) {
	app := NewApp("test", "1.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	app.Register(&Command{Name: "log", Summary: "Show log", Run: func([]string) int { return 0 }})

	code := app.Run(nil, noColorWriter())
	if code != 1 {
		t.Fatalf("expected exit code 1 for empty args, got %d", code)
	}
	if !strings.Contains(buf.String(), "Commands:") {
		t.Fatal("expected help output on stderr for empty args")
	}
}

func TestRunHelp(t *testing.T) {
	for _, trigger := range []string{"help", "-h", "--help"} {
		t.Run(trigger, func(t *testing.T) {
			app := NewApp("test", "1.0.0")
			var buf bytes.Buffer
			app.Stderr = &buf

			app.Register(&Command{Name: "log", Summary: "Show log", Run: func([]string) int { return 0 }})

			code := app.Run([]string{trigger}, noColorWriter())
			if code != 0 {
				t.Fatalf("expected exit code 0 for %q, got %d", trigger, code)
			}
			if !strings.Contains(buf.String(), "Commands:") {
				t.Fatalf("expected help output for %q", trigger)
			}
		})
	}
}

func TestRunHelpSubcommand(t *testing.T) {
	app := NewApp("test", "1.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	app.Register(&Command{
		Name:    "log",
		Summary: "Show commit log",
		Usage:   "test log [--oneline]",
		Run:     func([]string) int { return 0 },
	})

	code := app.Run([]string{"help", "log"}, noColorWriter())
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(buf.String(), "Show commit log") {
		t.Fatal("expected per-command help with summary")
	}
}

func TestRunSubcommandHFlag(t *testing.T) {
	app := NewApp("test", "1.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	app.Register(&Command{
		Name:    "log",
		Summary: "Show commit log",
		Usage:   "test log [--oneline]",
		Run:     func([]string) int { return 99 },
	})

	code := app.Run([]string{"log", "-h"}, noColorWriter())
	if code != 0 {
		t.Fatalf("expected exit code 0 for sub -h, got %d", code)
	}
	if !strings.Contains(buf.String(), "Show commit log") {
		t.Fatal("expected per-command help for -h flag")
	}
}

func TestRunUnknownCommandWithSuggestion(t *testing.T) {
	app := NewApp("test", "1.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	app.Register(&Command{Name: "log", Summary: "Show log", Run: func([]string) int { return 0 }})
	app.Register(&Command{Name: "diff", Summary: "Show diff", Run: func([]string) int { return 0 }})

	code := app.Run([]string{"lgo"}, noColorWriter())
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, `"lgo" is not a command`) {
		t.Fatal("expected unknown command error")
	}
	if !strings.Contains(out, `Did you mean "log"`) {
		t.Fatal("expected suggestion")
	}
}

func TestRunUnknownCommandNoSuggestion(t *testing.T) {
	app := NewApp("test", "1.0.0")
	var buf bytes.Buffer
	app.Stderr = &buf

	app.Register(&Command{Name: "log", Summary: "Show log", Run: func([]string) int { return 0 }})

	code := app.Run([]string{"xxxxxxx"}, noColorWriter())
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := buf.String()
	if strings.Contains(out, "Did you mean") {
		t.Fatal("expected no suggestion for very different input")
	}
	if !strings.Contains(out, "Run 'test help'") {
		t.Fatal("expected help hint")
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	app := NewApp("test", "1.0.0")
	app.Register(&Command{Name: "log", Summary: "s", Run: func([]string) int { return 0 }})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
	}()
	app.Register(&Command{Name: "log", Summary: "s2", Run: func([]string) int { return 0 }})
}

func TestCommandNames(t *testing.T) {
	app := NewApp("test", "1.0.0")
	app.Register(&Command{Name: "status", Summary: "s", Run: func([]string) int { return 0 }})
	app.Register(&Command{Name: "diff", Summary: "s", Run: func([]string) int { return 0 }})
	app.Register(&Command{Name: "log", Summary: "s", Run: func([]string) int { return 0 }})

	names := app.CommandNames()
	expected := []string{"diff", "log", "status"}
	if len(names) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, names)
	}
	for i, n := range names {
		if n != expected[i] {
			t.Fatalf("expected %v, got %v", expected, names)
		}
	}
}
