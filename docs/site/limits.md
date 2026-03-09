GitVista is built to make Git behavior legible, but it still inherits the realities of repository size, remote accessibility, and the difference between hosted and local execution.

- Hosted mode is aimed at public GitHub repositories.
- Very large histories can take longer to prepare before the graph is available.
- Private repositories, local-only branches, and sensitive work should use local mode instead of the hosted path.
- Unknown hosted URLs now fall back to the app shell, but missing static assets and unknown API routes still return `404`.
