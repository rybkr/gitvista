Install GitVista locally when you want to view a repository you already have on disk in real time.

- Requires a local Git repository checkout, shell access, and a browser on your machine.
- The install script places both a `gitvista` executable and a `git-vista` shim on your `PATH`, effectively registering `vista` as a `git` subcommand.

1. Install GitVista.

   ```bash
   curl -fsSL https://gitvista.io/install.sh | sh
   ```

2. Verify the command is available.

   ```bash
   git vista --help
   ```

3. Open the current repository or point GitVista at a different checkout.

   ```bash
   git vista open --repo /path/to/repo
   ```

---

To learn more about `git vista` cli options:

[Open the CLI docs](/docs/cli)
