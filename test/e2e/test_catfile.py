from pathlib import Path

import pytest

ALL_REPOS = ["express", "gitvista", "cpython", "octocat", "git"]
QUICK_REPOS = ["express", "gitvista", "octocat"]


def pytest_generate_tests(metafunc):
    if "repo_name" not in metafunc.fixturenames:
        return

    repo_names = QUICK_REPOS if metafunc.config.getoption("--quick") else ALL_REPOS
    metafunc.parametrize("repo_name", repo_names)


def test_cat_file_p_head(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"

    want = run_git(repo_dir, "cat-file", "-p", "HEAD")
    got = run_cli(root_dir, "--repo", str(repo_dir), "cat-file", "-p", "HEAD")
    assert got == want


def test_cat_file_t_head(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"

    want = run_git(repo_dir, "cat-file", "-t", "HEAD")
    got = run_cli(root_dir, "--repo", str(repo_dir), "cat-file", "-t", "HEAD")
    assert got == want


def test_cat_file_s_head(
    repo_name: str,
    root_dir: Path,
    run_git,
    run_cli,
) -> None:
    repo_dir = root_dir / "testdata" / repo_name
    git_dir = repo_dir / ".git"

    assert git_dir.exists(), f"prepared repository missing at {repo_dir}; run scripts/prepare_test_repos.py first"

    want = run_git(repo_dir, "cat-file", "-s", "HEAD")
    got = run_cli(root_dir, "--repo", str(repo_dir), "cat-file", "-s", "HEAD")
    assert got == want
