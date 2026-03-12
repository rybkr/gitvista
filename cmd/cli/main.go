package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/cli"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	gf, args := parseGlobalFlags(os.Args[1:])
	cw := cli.NewWriter(os.Stdout, gf.colorMode)

	for _, a := range args {
		if a == "--version" {
			printVersion(cw)
			os.Exit(0)
		}
	}

	app := cli.NewApp("gitvista-cli", version)
	app.Stderr = os.Stderr
	repoCtx := &repositoryContext{path: gf.repoPath}
	registerCommands(app, repoCtx, cw)

	if len(args) > 0 {
		cmd := app.Lookup(args[0])
		if cmd != nil && cmd.NeedsRepo {
			start := time.Now()
			var err error
			repoCtx.repo, err = gitcore.NewRepository(repoCtx.path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
				os.Exit(128)
			}
			repoCtx.loadDuration = time.Since(start)
			defer func() {
				if err := repoCtx.repo.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "gitvista-cli: close repository: %v\n", err)
				}
			}()
		}
	}

	os.Exit(app.Run(args, cw))
}

func printVersion(cw *cli.Writer) {
	fmt.Printf("%s %s\n", cw.Command("GitVista CLI"), cw.Muted(version))
	fmt.Printf("  %s  %s\n", cw.Cyan("commit:"), commit)
	fmt.Printf("  %s   %s\n", cw.Cyan("built:"), buildDate)
	fmt.Printf("  %s %s\n", cw.Cyan("go version:"), runtime.Version())
	fmt.Printf("  %s %s/%s\n", cw.Cyan("platform:"), runtime.GOOS, runtime.GOARCH)
}
