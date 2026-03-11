Local mode points GitVista at a repository on your own machine, so the UI reflects what is actually happening on disk right now. This is the right choice when you are actively working and need staged changes, branch movement, or unpushed work to stay accurate.

The interface stays familiar, but the source of truth changes. Instead of browsing a hosted clone, local mode reads your local `.git` directory directly, which makes it the safer and more faithful option for day-to-day engineering work.

- Install GitVista and run `git vista open --repo /path/to/your/repo`.
- If you built from source, run `./vista --repo /path/to/your/repo`.
- Use local mode when you care about staged changes, immediate refresh, and local-only history.
- Prefer local mode for private repositories, sensitive work, and anything that should never leave your machine.

If hosted mode helps you orient quickly, local mode is the version you keep nearby while you are actually changing code.
