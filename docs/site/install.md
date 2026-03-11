Install GitVista locally when you want to view a repository you already have on disk in real time.

- Requires a local Git repository checkout, shell access, and a browser on your machine.
- The install script adds a `git-vista` executable to your `PATH`, effectively registering `vista` as a `git` subcommand.

1. Install GitVista.

```
curl -fsSL https://gitvista.io/install.sh | sh
```

2. Verify the command is available.

```
git vista --help
```

3. Open the current repository or point GitVista at a different checkout.

```
git vista open --repo /path/to/repo
```
