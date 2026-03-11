GitVista makes Git history easier to read, but the right mode still depends on where the repository lives and what kind of state you need to see.

## Use hosted mode when

- Hosted mode is for public GitHub repositories that GitVista can clone and prepare on the server.

## Switch to local mode when

- Local mode is the right choice for private repositories, sensitive work, local-only branches, staged changes, and working tree diffs.

## Expect preparation time

- Hosted repositories with large or complicated histories can take longer to prepare before the graph is ready.

## Check local prerequisites

- Local mode still depends on a readable repository path and a free local port. `git vista doctor` is the fastest way to check those basics.

## Know the routing boundary

- Unknown app routes fall back to the app shell, while missing static assets and unknown API routes return `404`.

If the hosted path feels slow, incomplete, or too public for the job, that is usually the signal to switch to local mode rather than keep forcing the hosted path.
