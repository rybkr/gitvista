Hosted mode is the fastest way to get oriented in a public repository. Paste a GitHub URL, let GitVista prepare the repository-backed workspace, and you can start from graph shape instead of from raw commit hashes.

It is best when the job is inspection rather than active development. Hosted mode is good for demos, code archaeology, design reviews, and sharing a specific repository or commit with someone who should not have to install anything first.

- Hosted docs live at `/docs`, and section pages use `/docs/:section`.
- Hosted repositories open at `/repo/:id`.
- Selecting a commit updates the URL to `/repo/:id/:commitHash`, which makes the current view shareable.
- If the repository is still preparing, GitVista streams progress until the graph is ready.

The tradeoff is that the data is coming from a hosted clone, not your machine. That means hosted mode is about understanding repository history, not about watching your own working tree change in real time.
