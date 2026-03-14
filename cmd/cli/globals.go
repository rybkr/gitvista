package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rybkr/gitvista/internal/cli"
)

type globalFlags struct {
	colorMode      cli.ColorMode
	repoPath       string
	cpuProfilePath string
	memProfilePath string
}

func parseStringFlag(args []string, index int, name string) (string, int, bool) {
	arg := args[index]
	if arg == name {
		if index+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "gitvista-cli: missing value for %s\n", name)
			os.Exit(1)
		}
		return args[index+1], 1, true
	}

	if val, ok := strings.CutPrefix(arg, name+"="); ok {
		if val == "" {
			fmt.Fprintf(os.Stderr, "gitvista-cli: missing value for %s\n", name)
			os.Exit(1)
		}
		return val, 0, true
	}

	return "", 0, false
}

func parseGlobalFlags(args []string) (globalFlags, []string) {
	gf := globalFlags{
		colorMode: cli.ColorAuto,
		repoPath:  ".",
	}
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
			i++
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

		if val, skip, ok := parseStringFlag(args, i, "--repo"); ok {
			gf.repoPath = val
			i += skip
			continue
		}

		if val, skip, ok := parseStringFlag(args, i, "--cpuprofile"); ok {
			gf.cpuProfilePath = val
			i += skip
			continue
		}

		if val, skip, ok := parseStringFlag(args, i, "--memprofile"); ok {
			gf.memProfilePath = val
			i += skip
			continue
		}

		remaining = append(remaining, arg)
	}

	return gf, remaining
}
