GitVista works best when you move from broad context to exact evidence instead of jumping straight into a raw diff.

## Start with context

- Use `Graph` to read branch shape, merge flow, and commit order before you focus on any one revision.
- Use `Repository` to confirm the current branch, HEAD, commit counts, tags, remotes, and basic repository metadata.

## Move into file-level evidence

- Use `File Explorer` to browse the tree at the selected commit, inspect blame, and open commit-level or file-level diffs.
- Use `Lifecycle` when you care about what is unstaged, staged, recently committed locally, or ahead and behind the tracked upstream.

## Use focused analysis views

- Use `Analytics` for commit velocity, author activity, and diff-based hotspots over a selected time window.
- Use `Compare` to preview a merge between two branches before you actually attempt it.

## Share hosted investigations

The usual rhythm is simple: orient in `Graph`, confirm the repository state, then move into `File Explorer`, `Lifecycle`, or `Compare` only when you need exact file-level evidence.

In hosted mode, the repository and selected commit stay in the URL so the current view is shareable.
