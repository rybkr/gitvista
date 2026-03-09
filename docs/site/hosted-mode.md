Hosted mode is the fastest path into GitVista. Paste a public GitHub URL on the landing page and GitVista prepares a repository-backed workspace with the commit graph, repository overview, and diff views already wired up.

- Best for quick inspection, demos, and sharing a repository view without asking someone to install anything.
- Hosted docs now live at `/docs`, and hosted repositories open at `/repo/:id`.
- When a commit is selected in hosted mode, the URL updates to `/repo/:id/:commitHash` so the current view can be shared directly.
- If a repository is still being prepared, GitVista streams progress until the graph is ready.
