package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
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
	os.Exit(run())
}

func run() int {
	gf, args := parseGlobalFlags(os.Args[1:])
	cw := cli.NewWriter(os.Stdout, gf.colorMode)

	if gf.cpuProfilePath != "" {
		profFile, err := os.Create(gf.cpuProfilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gitvista-cli: create cpu profile: %v\n", err)
			return 1
		}
		if err := pprof.StartCPUProfile(profFile); err != nil {
			_ = profFile.Close()
			fmt.Fprintf(os.Stderr, "gitvista-cli: start cpu profile: %v\n", err)
			return 1
		}
		defer func() {
			pprof.StopCPUProfile()
			if err := profFile.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "gitvista-cli: close cpu profile: %v\n", err)
			}
		}()
	}

	if gf.memProfilePath != "" {
		defer func() {
			profFile, err := os.Create(gf.memProfilePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "gitvista-cli: create memory profile: %v\n", err)
				return
			}
			defer func() {
				if err := profFile.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "gitvista-cli: close memory profile: %v\n", err)
				}
			}()

			runtime.GC()
			debug.FreeOSMemory()
			if err := pprof.WriteHeapProfile(profFile); err != nil {
				fmt.Fprintf(os.Stderr, "gitvista-cli: write memory profile: %v\n", err)
			}
		}()
	}

	for _, a := range args {
		if a == "--version" {
			printVersion(cw)
			return 0
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
				return 128
			}
			repoCtx.loadDuration = time.Since(start)
			defer func() {
				if err := repoCtx.repo.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "gitvista-cli: close repository: %v\n", err)
				}
			}()
		}
	}

	return app.Run(args, cw)
}

func printVersion(cw *cli.Writer) {
	fmt.Printf("%s %s\n", cw.Command("GitVista CLI"), cw.Muted(version))
	fmt.Printf("  %s  %s\n", cw.Cyan("commit:"), commit)
	fmt.Printf("  %s   %s\n", cw.Cyan("built:"), buildDate)
	fmt.Printf("  %s %s\n", cw.Cyan("go version:"), runtime.Version())
	fmt.Printf("  %s %s/%s\n", cw.Cyan("platform:"), runtime.GOOS, runtime.GOARCH)
}
