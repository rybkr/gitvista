package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rybkr/gitvista/internal/cli"
)

type globalFlags struct {
	colorMode cli.ColorMode
}

// parseGlobalFlags extracts --color and --no-color from anywhere in args,
// returning the parsed flags and the remaining (filtered) arguments.
func parseGlobalFlags(args []string) (globalFlags, []string) {
	gf := globalFlags{colorMode: cli.ColorAuto}
	var remaining []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--no-color" {
			gf.colorMode = cli.ColorNever
			continue
		}

		if arg == "--color" && i+1 < len(args) {
			mode, err := cli.ParseColorMode(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "gitvista-cli: %v\n", err)
				os.Exit(1)
			}
			gf.colorMode = mode
			i++ // skip the value
			continue
		}

		if val, ok := strings.CutPrefix(arg, "--color="); ok {
			mode, err := cli.ParseColorMode(val)
			if err != nil {
				fmt.Fprintf(os.Stderr, "gitvista-cli: %v\n", err)
				os.Exit(1)
			}
			gf.colorMode = mode
			continue
		}

		remaining = append(remaining, arg)
	}

	return gf, remaining
}
