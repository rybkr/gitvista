The CLI is the fastest way to point GitVista at a repository you already have. If you run `git vista open` with no repo flag, GitVista uses the current directory as the repository path.

- Use `git vista open` to start GitVista and launch the browser.
- Use `git vista serve` to start the server without launching the browser.
- Use `git vista url` to print the exact URL GitVista would open.
- Use `git vista doctor` to verify repository, listener, and browser readiness.

Common commands:

```bash
git vista open
git vista open -repo /path/to/repo
git vista open --branch main
git vista open --commit HEAD~1
git vista open --path internal/server
git vista serve --port 3000
git vista url --commit HEAD~1
git vista doctor
```

Useful flags:

- `-repo /path/to/repo` opens a repository other than the current directory.
- `--branch main` opens the graph focused on a branch tip.
- `--commit HEAD~1` opens the graph focused on a specific revision.
- `--path internal/server` opens the file explorer focused on a repository path.
- `--no-browser` starts the server without launching a browser window.
- `--print-url` prints the resolved launch URL.

Environment defaults:

- `GITVISTA_REPO` sets the default repository path.
- `GITVISTA_PORT` sets the default port.
- `GITVISTA_HOST` sets the default bind host.
