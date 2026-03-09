GitVista is meant to make Git behavior legible, but it still inherits the realities of repository size, remote accessibility, and the difference between hosted and local execution. The product is useful when those boundaries are explicit rather than hidden.

Hosted mode is intentionally scoped. It is aimed at public GitHub repositories, and very large histories can take longer to prepare before the graph is available.

- Private repositories, local-only branches, and sensitive work should use local mode.
- Hosted mode may take longer to prepare repositories with large or complicated histories.
- Unknown hosted URLs still fall back to the app shell, while missing static assets and unknown API routes return `404`.

If the hosted path feels slow, incomplete, or too public for the job, that is usually the signal to switch modes rather than to push harder against the hosted boundary.
