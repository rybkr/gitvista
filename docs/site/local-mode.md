Local mode runs the GitVista binary against a repository on your own machine, so branch movement, staged changes, and diffs reflect what is happening on disk right now. This is the right mode when you are actively working, not just inspecting history.

- Install or build the binary, then run `gitvista -repo /path/to/your/repo`.
- Use local mode when you care about staged changes, immediate refresh, and unpushed work.
- The browser UI stays familiar, but the data source is your local `.git` directory instead of a hosted clone.
- Local mode is the safer choice for private repositories and sensitive work.
