The `git vista` command is the fastest way to point GitVista at a repository you already have. The install also gives you the `gitvista` binary, but the Git subcommand form is the standard entrypoint in these docs.

## Commands

- `git vista open` starts GitVista and launches the browser. It is the default command.
- `git vista serve` starts the server without launching the browser.
- `git vista url` prints the exact URL GitVista would open.
- `git vista doctor` checks repository loading, port binding, and browser launcher readiness.

## Common tasks

```bash
git vista open
git vista open --repo /path/to/repo
git vista open --branch main
git vista open --commit HEAD~1
git vista open --path internal/server
git vista serve --port 3000
git vista url --commit HEAD~1
git vista doctor
```

## Command behavior

- `git vista open` uses the current directory when you do not pass `--repo`.
- `git vista open HEAD~1` is shorthand for `git vista open --commit HEAD~1`.
- `--branch` and `--commit` are mutually exclusive.
- `--path` opens the file explorer focused on a repository path and falls back to `HEAD` if you did not also pick a commit.
- `--no-browser` and `--print-url` apply to `git vista open`.

## Configuration

Environment-variable defaults live in the `Config` reference so command behavior and deployment settings stay separate. Use that page for flag precedence, local defaults, and hosted-mode variables.
