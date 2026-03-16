from pathlib import Path

import pytest


@pytest.mark.parametrize("repo_name", ["express", "gitvista", "cpython", "octocat", "git"])
def test_rev_list_all_matches_git_for_prepared_repos(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"

    want = run_git(repo_dir, "rev-list", "--all")
    got = run_cli(root_dir, "--repo", str(repo_dir), "rev-list", "--all")
    assert got == want

    want = run_git(repo_dir, "rev-list", "--all", "--topo-order")
    got = run_cli(root_dir, "--repo", str(repo_dir), "rev-list", "--all", "--topo-order")
    assert got == want
