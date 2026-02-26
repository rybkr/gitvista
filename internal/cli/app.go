package cli

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/rybkr/gitvista/internal/termcolor"
)

// Command describes a single CLI subcommand.
type Command struct {
	Name      string
	Summary   string   // one-line description for help listing
	Usage     string   // full usage string for per-command help
	Examples  []string // example invocations
	Run       func(args []string) int
	NeedsRepo bool // whether the command requires a loaded repository
}

// App is a lightweight CLI application with subcommand dispatch.
type App struct {
	Name     string
	Version  string
	Stderr   io.Writer
	commands map[string]*Command
	order    []string // insertion order preserved for help
}

// NewApp creates a new App with the given name and version.
func NewApp(name, version string) *App {
	return &App{
		Name:     name,
		Version:  version,
		Stderr:   os.Stderr,
		commands: make(map[string]*Command),
	}
}

// Register adds a command to the app. It panics if a command with the
// same name has already been registered.
func (a *App) Register(cmd *Command) {
	if _, exists := a.commands[cmd.Name]; exists {
		panic(fmt.Sprintf("cli: duplicate command %q", cmd.Name))
	}
	a.commands[cmd.Name] = cmd
	a.order = append(a.order, cmd.Name)
}

// Lookup returns the named command, or nil if not found.
func (a *App) Lookup(name string) *Command {
	return a.commands[name]
}

// CommandNames returns all registered command names in sorted order.
func (a *App) CommandNames() []string {
	names := make([]string, len(a.order))
	copy(names, a.order)
	sort.Strings(names)
	return names
}

// Run dispatches args to the appropriate command. It returns an exit code.
//
// Dispatch rules:
//  1. Empty args → print app help to stderr, return 1
//  2. "help" / "-h" / "--help" → print app or per-command help, return 0
//  3. Known command → intercept -h/--help in sub-args, else call cmd.Run
//  4. Unknown command → error + suggestion + hint, return 1
func (a *App) Run(args []string, cw *termcolor.Writer) int {
	if len(args) == 0 {
		FormatAppHelp(a, cw)
		return 1
	}

	name := args[0]
	subArgs := args[1:]

	// Global help triggers.
	if name == "help" || name == "-h" || name == "--help" {
		if len(subArgs) > 0 {
			return a.showCommandHelp(subArgs[0], cw)
		}
		FormatAppHelp(a, cw)
		return 0
	}

	// Known command.
	if cmd := a.Lookup(name); cmd != nil {
		// Intercept -h / --help on any subcommand.
		for _, arg := range subArgs {
			if arg == "-h" || arg == "--help" {
				FormatCommandHelp(a, cmd, cw)
				return 0
			}
		}
		return cmd.Run(subArgs)
	}

	// Unknown command.
	fpf(a.Stderr, "%s: %q is not a command\n", a.Name, name)
	if suggestion := Suggest(name, a.CommandNames()); suggestion != "" {
		fpf(a.Stderr, "\n\tDid you mean %q?\n", suggestion)
	}
	fpf(a.Stderr, "\nRun '%s help' for a list of commands.\n", a.Name)
	return 1
}

func (a *App) showCommandHelp(name string, cw *termcolor.Writer) int {
	cmd := a.Lookup(name)
	if cmd == nil {
		fpf(a.Stderr, "%s help: unknown command %q\n", a.Name, name)
		return 1
	}
	FormatCommandHelp(a, cmd, cw)
	return 0
}
