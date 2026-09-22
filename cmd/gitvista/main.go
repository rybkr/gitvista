package main

import (
	"context"
	"os"
	"os/signal"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app := cli.New(cli.Dependencies{
		Build: cli.BuildInfo{
			Version:   version,
			Commit:    commit,
			BuildDate: buildDate,
		},

		OpenRepository: git.Open,
		NewSession:     session.New,
		NewServer:      httpserver.New,
		OpenBrowser:    browser.Open,
		Updater:        selfupdate.New(),
		WebFS:          webassets.FS(),

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: os.Getenv,
	})

	return app.Run(ctx, os.Args[1:])
}
