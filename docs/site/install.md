Install GitVista locally when you want to inspect a repository on your own machine.

- Requires a local Git checkout, shell access, and a browser on the same machine.
- Install once, then open whichever repository you want from the CLI.

1. Install GitVista.

```bash
curl -fsSL https://gitvista.io/install.sh | sh
```

2. Open a repository directly.

```bash
git vista open -repo /path/to/repo
```

3. Or run it from inside the repository you want to inspect.

```bash
git vista open
```

Useful follow-up commands:

```bash
git vista open --branch main
```

```bash
git vista open --commit HEAD~1
```

```bash
git vista open --path internal/server
```

```bash
git vista serve --port 3000
```

If you run `git vista open` inside a repository, GitVista uses the current directory by default.
