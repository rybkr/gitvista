from pathlib import Path

import pytest


def test_status(
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / "status-traps"
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"

    want = run_git(repo_dir, "status", "--short")
    got = run_cli(repo_dir, "status", "--short")
    assert want == got
