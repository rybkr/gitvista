from pathlib import Path

import pytest


def test_merge_base(
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / "repos" / "merge-base-traps"
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"
    commits = run_git(repo_dir, "rev-list", "--all").strip().split("\n")

    for i, a in enumerate(commits):
        for j, b in enumerate(commits[i + 1 :], i + 1):
            want = run_git(repo_dir, "merge-base", a, b).strip()
            got = run_cli(repo_dir, "merge-base", a, b).strip()
            assert want == got
