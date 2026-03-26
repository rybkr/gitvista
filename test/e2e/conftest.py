from __future__ import annotations

import subprocess
from pathlib import Path

import pytest


def pytest_addoption(parser):
    parser.addoption(
        "--quick",
        action="store_true",
        default=False,
        help="skip the largest e2e repos (git and cpython)",
    )


def repo_root() -> Path:
    return Path(
        subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
    )


@pytest.fixture(scope="session")
def root_dir() -> Path:
    return repo_root()


@pytest.fixture(scope="session")
def cli_path(tmp_path_factory: pytest.TempPathFactory, root_dir: Path) -> Path:
    build_dir = tmp_path_factory.mktemp("gitvista-e2e")
    cli = build_dir / "cli"
    subprocess.run(
        ["go", "build", "-o", str(cli), "./cmd/cli"],
        cwd=root_dir,
        check=True,
    )
    return cli


def run(cmd: list[str], cwd: Path) -> str:
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
    )
    return completed.stdout


@pytest.fixture
def run_git():
    def _run(repo_dir: Path, *args: str) -> str:
        return run(["git", *args], cwd=repo_dir)

    return _run


@pytest.fixture
def run_cli(cli_path: Path):
    def _run(root_dir: Path, *args: str) -> str:
        return run([str(cli_path), *args], cwd=root_dir)

    return _run
