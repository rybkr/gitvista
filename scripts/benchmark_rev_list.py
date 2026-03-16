#!/usr/bin/env python3

from __future__ import annotations

import argparse
import shutil
import statistics
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class RepoBenchmark:
    name: str
    commit_count: int
    git_seconds: float
    gitvista_seconds: float

    @property
    def ratio(self) -> float:
        return self.gitvista_seconds / self.git_seconds

    @property
    def percent_slower(self) -> float:
        return (self.ratio - 1.0) * 100.0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Benchmark gitvista rev-list --all against git rev-list --all.",
    )
    parser.add_argument(
        "repos",
        nargs="*",
        help="Repository names under testdata/ to benchmark. Defaults to all prepared repos.",
    )
    parser.add_argument(
        "--runs",
        type=int,
        default=5,
        help="Measured runs per command after warmup. Default: 5.",
    )
    parser.add_argument(
        "--warmup",
        type=int,
        default=1,
        help="Warmup runs per command before measuring. Default: 1.",
    )
    parser.add_argument(
        "--cli",
        type=Path,
        help="Path to a prebuilt gitvista CLI. If omitted, the script builds ./cmd/cli once.",
    )
    parser.add_argument(
        "--testdata-root",
        type=Path,
        help="Path to the testdata root. Defaults to <repo-root>/testdata.",
    )
    return parser.parse_args()


def repo_root() -> Path:
    completed = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        check=True,
        capture_output=True,
        text=True,
    )
    return Path(completed.stdout.strip())


def discover_repositories(testdata_root: Path) -> list[str]:
    repositories = []
    for child in sorted(testdata_root.iterdir()):
        if child.is_dir() and (child / ".git").exists():
            repositories.append(child.name)
    return repositories


def build_cli(root_dir: Path) -> Path:
    build_dir = Path(tempfile.mkdtemp(prefix="gitvista-bench-"))
    cli_path = build_dir / "cli"
    try:
        subprocess.run(
            ["go", "build", "-o", str(cli_path), "./cmd/cli"],
            cwd=root_dir,
            check=True,
        )
    except Exception:
        shutil.rmtree(build_dir, ignore_errors=True)
        raise
    return cli_path


def run_stdout(cmd: list[str], cwd: Path) -> bytes:
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        check=True,
        capture_output=True,
    )
    return completed.stdout


def progress_bar(completed: int, total: int, width: int = 24) -> str:
    if total <= 0:
        return "[no runs]"
    filled = int(width * completed / total)
    if completed > 0 and filled == 0:
        filled = 1
    if filled > width:
        filled = width
    return "[" + ("#" * filled) + ("-" * (width - filled)) + f"] {completed}/{total}"


def print_progress(label: str, completed: int, total: int) -> None:
    print(f"\r{label:<12} {progress_bar(completed, total)}", end="", file=sys.stderr, flush=True)
    if completed == total:
        print(file=sys.stderr, flush=True)


def measure_command(label: str, cmd: list[str], cwd: Path, warmup: int, runs: int) -> float:
    for _ in range(warmup):
        subprocess.run(
            cmd,
            cwd=cwd,
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    samples = []
    print_progress(label, 0, runs)
    for index in range(runs):
        start = time.perf_counter()
        subprocess.run(
            cmd,
            cwd=cwd,
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        samples.append(time.perf_counter() - start)
        print_progress(label, index + 1, runs)

    return statistics.median(samples)


def commit_count(repo_dir: Path) -> int:
    output = run_stdout(["git", "rev-list", "--all"], cwd=repo_dir)
    return sum(1 for line in output.splitlines() if line)


def benchmark_repo(
    root_dir: Path,
    repo_dir: Path,
    cli_path: Path,
    warmup: int,
    runs: int,
) -> RepoBenchmark:
    revisions = commit_count(repo_dir)

    git_seconds = measure_command(
        f"{repo_dir.name}:git",
        ["git", "rev-list", "--all"],
        cwd=repo_dir,
        warmup=warmup,
        runs=runs,
    )
    gitvista_seconds = measure_command(
        f"{repo_dir.name}:vista",
        [str(cli_path), "--repo", str(repo_dir), "rev-list", "--all"],
        cwd=root_dir,
        warmup=warmup,
        runs=runs,
    )

    return RepoBenchmark(
        name=repo_dir.name,
        commit_count=revisions,
        git_seconds=git_seconds,
        gitvista_seconds=gitvista_seconds,
    )


def format_percent(percent: float) -> str:
    if percent >= 0:
        return f"{percent:.1f}% slower"
    return f"{abs(percent):.1f}% faster"


def format_seconds(value: float) -> str:
    return f"{value:.6f}"


def format_ratio(value: float) -> str:
    return f"{value:.2f}x"


def main() -> int:
    args = parse_args()
    root_dir = repo_root()
    testdata_root = args.testdata_root or (root_dir / "testdata")
    if not testdata_root.exists():
        print(f"testdata root does not exist: {testdata_root}", file=sys.stderr)
        return 1

    repo_names = args.repos or discover_repositories(testdata_root)
    if not repo_names:
        print(
            "no prepared repositories found under testdata; run scripts/prepare_test_repos.py first",
            file=sys.stderr,
        )
        return 1

    missing = [name for name in repo_names if not (testdata_root / name / ".git").exists()]
    if missing:
        print(
            "missing prepared repositories: "
            + ", ".join(missing)
            + "; run scripts/prepare_test_repos.py first",
            file=sys.stderr,
        )
        return 1

    cleanup_dir: Path | None = None
    cli_path = args.cli
    if cli_path is None:
        cli_path = build_cli(root_dir)
        cleanup_dir = cli_path.parent

    print(
        f"Benchmarking {len(repo_names)} repos with runs={args.runs}, warmup={args.warmup}",
        file=sys.stderr,
    )

    results: list[RepoBenchmark] = []
    try:
        for name in repo_names:
            repo_dir = testdata_root / name
            print(f"benchmarking {name}", file=sys.stderr)
            results.append(
                benchmark_repo(
                    root_dir=root_dir,
                    repo_dir=repo_dir,
                    cli_path=cli_path,
                    warmup=args.warmup,
                    runs=args.runs,
                )
            )
    finally:
        if cleanup_dir is not None:
            shutil.rmtree(cleanup_dir, ignore_errors=True)

    name_width = max(len("repo"), *(len(result.name) for result in results))
    commits_width = max(len("commits"), *(len(str(result.commit_count)) for result in results))
    git_width = max(len("git_s"), *(len(format_seconds(result.git_seconds)) for result in results))
    vista_width = max(len("gitvista_s"), *(len(format_seconds(result.gitvista_seconds)) for result in results))
    ratio_width = max(len("ratio"), *(len(format_ratio(result.ratio)) for result in results))
    result_width = max(len("result"), *(len(format_percent(result.percent_slower)) for result in results))

    print(
        f"{'repo':<{name_width}}  "
        f"{'commits':>{commits_width}}  "
        f"{'git_s':>{git_width}}  "
        f"{'gitvista_s':>{vista_width}}  "
        f"{'ratio':>{ratio_width}}  "
        f"{'result':<{result_width}}"
    )
    for result in results:
        print(
            f"{result.name:<{name_width}}  "
            f"{result.commit_count:>{commits_width}}  "
            f"{format_seconds(result.git_seconds):>{git_width}}  "
            f"{format_seconds(result.gitvista_seconds):>{vista_width}}  "
            f"{format_ratio(result.ratio):>{ratio_width}}  "
            f"{format_percent(result.percent_slower):<{result_width}}"
        )

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
