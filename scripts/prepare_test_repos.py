#!/usr/bin/env python3

import argparse
import os
import subprocess
from datetime import datetime
from pathlib import Path

USE_SSH_TEST_REPOS = os.environ.get("USE_SSH_TEST_REPOS") == "1"
SCRIPT_DIR = Path(__file__).resolve().parent

REPOSITORIES = (
    ("express", "git@github.com:expressjs/express.git" if USE_SSH_TEST_REPOS else "https://github.com/expressjs/express.git"),
    ("gitvista", "git@github.com:rybkr/gitvista.git" if USE_SSH_TEST_REPOS else "https://github.com/rybkr/gitvista.git"),
    ("cpython", "git@github.com:python/cpython.git" if USE_SSH_TEST_REPOS else "https://github.com/python/cpython.git"),
    ("octocat", "git@github.com:octocat/Hello-World.git" if USE_SSH_TEST_REPOS else "https://github.com/octocat/Hello-World.git"),
    ("git", "git@github.com:git/git.git" if USE_SSH_TEST_REPOS else "https://github.com/git/git.git"),
)
LOCAL_REPOSITORIES = (
    ("merge-base-traps", SCRIPT_DIR / "create_merge_base_trap_repo.sh"),
)

timestamp_format: str = "%Y-%m-%d %H:%M:%S"


def log(channel: str, message: str) -> None:
    timestamp = datetime.now().strftime("%H:%M:%S")
    print(f"[{timestamp}] [{channel}] {message}", flush=True)


def split_progress_lines(buffer: str) -> tuple[list[str], str]:
    lines: list[str] = []
    current = []

    for char in buffer:
        if char in ("\r", "\n"):
            line = "".join(current).strip()
            if line:
                lines.append(line)
            current = []
            continue
        current.append(char)

    return lines, "".join(current)


def format_git_progress(line: str) -> str:
    if "%" not in line:
        return line
    return " ".join(part.strip() for part in line.split(","))


def run_git(
    repo_dir: Path | None,
    *args: str,
    stream_progress: bool = False,
    progress_label: str = "git",
) -> str:
    cmd = ["git"]
    if repo_dir is not None:
        cmd.extend(["-C", str(repo_dir)])
    cmd.extend(args)
    location = str(repo_dir) if repo_dir is not None else str(Path.cwd())
    log("git", f"{location}$ {' '.join(cmd)}")

    if not stream_progress:
        completed = subprocess.run(
            cmd,
            check=True,
            text=True,
            capture_output=True,
        )
        return completed.stdout

    process = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,
    )
    assert process.stdout is not None
    assert process.stderr is not None

    stderr_buffer = ""
    while True:
        chunk = process.stderr.read(1)
        if chunk == "":
            break
        stderr_buffer += chunk
        lines, stderr_buffer = split_progress_lines(stderr_buffer)
        for line in lines:
            log(progress_label, format_git_progress(line))

    stdout = process.stdout.read()
    return_code = process.wait()

    if stderr_buffer.strip():
        log(progress_label, format_git_progress(stderr_buffer.strip()))

    if return_code != 0:
        raise subprocess.CalledProcessError(return_code, cmd, output=stdout, stderr="")

    return stdout


def run_command(*args: str, cwd: Path | None = None) -> None:
    location = str(cwd) if cwd is not None else str(Path.cwd())
    log("exec", f"{location}$ {' '.join(args)}")
    subprocess.run(
        args,
        cwd=cwd,
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        text=True,
    )


def update_repository(root_dir: Path, name: str, remote: str) -> None:
    repo_dir = root_dir / name
    log(name, f"source: {remote}")

    if repo_dir.exists():
        log(name, "repository exists; fetching updates")
        run_git(repo_dir, "fetch", "--all", "--tags", "--prune")

        current_branch = run_git(repo_dir, "rev-parse", "--abbrev-ref", "HEAD").strip()
        if current_branch != "HEAD":
            log(name, f"pulling latest changes on {current_branch}")
            run_git(repo_dir, "pull", "--ff-only")
        else:
            log(name, "detached HEAD; skipping pull")
        action = "updated"
    else:
        log(name, f"cloning into {repo_dir}")
        run_git(
            None,
            "clone",
            "--progress",
            remote,
            str(repo_dir),
            stream_progress=True,
            progress_label=f"{name}:clone",
        )
        action = "cloned"

    log(name, "collecting rev-list data")
    revisions = [revision for revision in run_git(repo_dir, "rev-list", "--all").splitlines() if revision]
    log(name, f"captured {len(revisions)} revisions")


def prepare_local_repository(root_dir: Path, name: str, script_path: Path) -> None:
    repo_dir = root_dir / name
    log(name, f"building fixture with {script_path}")
    run_command("bash", str(script_path), str(repo_dir), cwd=SCRIPT_DIR.parent)
    revisions = [revision for revision in run_git(repo_dir, "rev-list", "--all").splitlines() if revision]
    log(name, f"captured {len(revisions)} revisions")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Clone or refresh test repositories for gitcore testing.",
    )
    parser.add_argument(
        "--root",
        default="testdata",
        help="Directory where repositories are cloned.",
    )
    parser.add_argument("--force", action="store_true", help="Force repository updates, even if done recently.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    root_dir = Path(args.root).resolve()
    try:
        with Path(root_dir / ".updated-at").open("r") as f:
            updated_at = datetime.strptime(f.read().strip(), timestamp_format)
    except FileNotFoundError:
        updated_at = datetime.fromtimestamp(0)

    if args.force or (datetime.now() - updated_at).total_seconds() >= 86400:
        log("repos", f"repository root: {root_dir}")
        root_dir.mkdir(parents=True, exist_ok=True)
        for name, remote in REPOSITORIES:
            update_repository(root_dir, name, remote)
        for name, script_path in LOCAL_REPOSITORIES:
            prepare_local_repository(root_dir, name, script_path)
        log("repos", f"prepared {len(REPOSITORIES) + len(LOCAL_REPOSITORIES)} repositories")

        with Path(root_dir / ".updated-at").open("w") as f:
            f.write(datetime.now().strftime(timestamp_format))
            log("prepare", f"update logged at {datetime.now()}")
    else:
        log("prepare", f"update already completed at {updated_at} (use --force)")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
