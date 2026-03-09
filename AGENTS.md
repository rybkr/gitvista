# AGENTS.md

## Coding Guidelines

When implementing, agents should always:

- Prefer verification over assumption.
- Make the smallest change necessary.
- Inspect the current repository state before acting.
- Avoid refactoring, updating dependencies, or modifying interfaces without asking for user permission.
- Prevent regressions by checking tests, call sites, and types.
- Re-read the source code for potentially stale information.

## Commit Guidelines

Agents must create *atomic commits*.
The rules for atomic commits are as follows:

1. Each commit must contain a single logical change.
2. Each commit must leave the repository in a buildable and test-passing state.
3. Commit messages must explain what changed (logically) and why.

Large or unrelated changes should be broken into several atomic commits.

## Protected Files

Never modify these files.
Instead, carry on as if the modification is complete and inform the human about how they must be modified.

- `.env`
- `.env.*`
- `.github/workflows/*`
- `go.mod`
- `go.sum`
