package main

import (
	"fmt"
	"strings"

	"github.com/rybkr/gitvista/internal/cli"
)

type globalFlags struct {
	colorMode      cli.ColorMode
	repoPath       string
	cpuProfilePath string
	memProfilePath string
}

func parseStringFlag(args []string, index int, name string) (string, int, bool, error) {
	arg := args[index]
	if arg == name {
		if index+1 >= len(args) {
			return "", 0, false, fmt.Errorf("gitvista-cli: missing value for %s", name)
		}
		return args[index+1], 1, true, nil
	}

	if val, ok := strings.CutPrefix(arg, name+"="); ok {
		if val == "" {
			return "", 0, false, fmt.Errorf("gitvista-cli: missing value for %s", name)
		}
		return val, 0, true, nil
	}

	return "", 0, false, nil
}

func parseGlobalFlags(args []string) (globalFlags, []string, error) {
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
				return globalFlags{}, nil, fmt.Errorf("gitvista-cli: %w", err)
			}
			gf.colorMode = mode
			i++
			continue
		}
		if arg == "--color" {
			return globalFlags{}, nil, fmt.Errorf("gitvista-cli: missing value for --color")
		}

		if val, ok := strings.CutPrefix(arg, "--color="); ok {
			mode, err := cli.ParseColorMode(val)
			if err != nil {
				return globalFlags{}, nil, fmt.Errorf("gitvista-cli: %w", err)
			}
			gf.colorMode = mode
			continue
		}

		if val, skip, ok, err := parseStringFlag(args, i, "--repo"); err != nil {
			return globalFlags{}, nil, err
		} else if ok {
			gf.repoPath = val
			i += skip
			continue
		}

		if val, skip, ok, err := parseStringFlag(args, i, "--cpuprofile"); err != nil {
			return globalFlags{}, nil, err
		} else if ok {
			gf.cpuProfilePath = val
			i += skip
			continue
		}

		if val, skip, ok, err := parseStringFlag(args, i, "--memprofile"); err != nil {
			return globalFlags{}, nil, err
		} else if ok {
			gf.memProfilePath = val
			i += skip
			continue
		}

		remaining = append(remaining, arg)
	}

	return gf, remaining, nil
}
