from pathlib import Path

import pytest


def test_rev_list_all(
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


def test_rev_list_all_topo(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"
    want = run_git(repo_dir, "rev-list", "--all", "--topo-order")
    got = run_cli(root_dir, "--repo", str(repo_dir), "rev-list", "--all", "--topo-order")
    assert got == want


def test_rev_list_all_date(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"
    want = run_git(repo_dir, "rev-list", "--all", "--date-order")
    got = run_cli(root_dir, "--repo", str(repo_dir), "rev-list", "--all", "--date-order")
    assert got == want


def test_rev_list_count_all(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"
    want = run_git(repo_dir, "rev-list", "--count", "--all")
    got = run_cli(root_dir, "--repo", str(repo_dir), "rev-list", "--count", "--all")
    assert got == want


def test_rev_list_no_merges(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"
    want = run_git(repo_dir, "rev-list", "--no-merges", "--all")
    got = run_cli(root_dir, "--repo", str(repo_dir), "rev-list", "--no-merges", "--all")
    assert got == want
