#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TARGET_DIR="${1:-$REPO_ROOT/testdata/merge-base-traps}"

if [[ -e "$TARGET_DIR" ]]; then
    rm -rf "$TARGET_DIR"
fi

mkdir -p "$TARGET_DIR"
cd "$TARGET_DIR"

git init -q
git config user.name "GitVista Test"
git config user.email "test@gitvista.io"

export GIT_AUTHOR_NAME="GitVista Test"
export GIT_AUTHOR_EMAIL="test@gitvista.io"
export GIT_COMMITTER_NAME="GitVista Test"
export GIT_COMMITTER_EMAIL="test@gitvista.io"
export GIT_EDITOR=:
export VISUAL=:
export EDITOR=:

tick=0

set_time() {
    local ts
    ts="$(printf '2024-01-01T00:%02d:00Z' "$tick")"
    export GIT_AUTHOR_DATE="$ts"
    export GIT_COMMITTER_DATE="$ts"
    tick=$((tick + 1))
}

write_and_commit() {
    local message="$1"
    shift
    set_time
    "$@"
    git add -A
    git commit -q -m "$message"
}

write_commit_tree() {
    local message="$1"
    local content="$2"
    shift 2
    set_time
    printf '%s\n' "$content" > maze.txt
    git add maze.txt
    local tree
    tree="$(git write-tree)"
    printf '%s\n' "$message" | git commit-tree "$tree" "$@"
}

merge_with_resolution() {
    local merge_target="$1"
    local message="$2"
    local path="$3"
    local content="$4"
    git merge -q --no-ff "$merge_target" -m "$message" >/dev/null 2>&1 || true
    printf '%s\n' "$content" > "$path"
    git add "$path"
    set_time
    git commit -q --no-edit >/dev/null 2>&1
}

write_and_commit "root" bash -c '
    cat > README.md <<'"'"'EOF'"'"'
# Merge-base trap repository

Generated history for merge-base stress testing.

This repository intentionally contains:

- criss-cross merges with multiple best common ancestors
- timestamp-skewed criss-cross merges
- rebased branches
- an octopus merge
- an evil merge created with commit-tree

Useful refs are exposed as branches and tags:

- criss-left-tip / criss-right-tip
- skew-left-tip / skew-right-tip
- rebase-old-tip / rebase-new-base / rebase-new-tip
- octopus-tip
- evil-merge-tip
EOF
    printf "root\n" > maze.txt
'
git branch -M main

git checkout -q -b criss-left
write_and_commit "criss-left-1" bash -c 'printf "left-1\n" > maze.txt'
CRISS_LEFT_1="$(git rev-parse HEAD)"

git checkout -q main
git checkout -q -b criss-right
write_and_commit "criss-right-1" bash -c 'printf "right-1\n" > maze.txt'
CRISS_RIGHT_1="$(git rev-parse HEAD)"

git checkout -q criss-left
merge_with_resolution "criss-right" "criss-left-merge" "maze.txt" "left-merge-resolution"
CRISS_LEFT_2="$(git rev-parse HEAD)"

git checkout -q criss-right
merge_with_resolution "$CRISS_LEFT_1" "criss-right-merge" "maze.txt" "right-merge-resolution"
CRISS_RIGHT_2="$(git rev-parse HEAD)"

git tag criss-left-tip "$CRISS_LEFT_2"
git tag criss-right-tip "$CRISS_RIGHT_2"

git checkout -q main
git checkout -q -b skew-left
write_and_commit "skew-left-base" bash -c 'printf "skew-left-base\n" > skew.txt'
SKEW_LEFT_BASE="$(git rev-parse HEAD)"

git checkout -q main
git checkout -q -b skew-right
write_and_commit "skew-right-base" bash -c 'printf "skew-right-base\n" > skew.txt'
SKEW_RIGHT_BASE="$(git rev-parse HEAD)"

git checkout -q skew-left
git merge -q --no-ff skew-right -m "skew-left-merge" >/dev/null 2>&1 || true
printf "skew-left-merge\n" > skew.txt
git add skew.txt
export GIT_AUTHOR_DATE="2024-01-03T00:00:00Z"
export GIT_COMMITTER_DATE="2024-01-03T00:00:00Z"
git commit -q --no-edit >/dev/null 2>&1
SKEW_LEFT_TIP="$(git rev-parse HEAD)"

git checkout -q skew-right
git merge -q --no-ff "$SKEW_LEFT_BASE" -m "skew-right-merge" >/dev/null 2>&1 || true
printf "skew-right-merge\n" > skew.txt
git add skew.txt
export GIT_AUTHOR_DATE="2024-01-04T00:00:00Z"
export GIT_COMMITTER_DATE="2024-01-04T00:00:00Z"
git commit -q --no-edit >/dev/null 2>&1
SKEW_RIGHT_TIP="$(git rev-parse HEAD)"

git tag skew-left-tip "$SKEW_LEFT_TIP"
git tag skew-right-tip "$SKEW_RIGHT_TIP"

git checkout -q "$CRISS_LEFT_2"
git checkout -q -b criss-left-deeper
write_and_commit "criss-left-deeper-1" bash -c 'printf "left-deeper-1\n" >> maze.txt'
CRISS_LEFT_3="$(git rev-parse HEAD)"

git checkout -q "$CRISS_RIGHT_2"
git checkout -q -b criss-right-deeper
write_and_commit "criss-right-deeper-1" bash -c 'printf "right-deeper-1\n" >> maze.txt'
CRISS_RIGHT_3="$(git rev-parse HEAD)"

git checkout -q criss-left-deeper
merge_with_resolution "criss-right-deeper" "criss-left-deeper-merge" "maze.txt" "left-deeper-merge-resolution"
CRISS_LEFT_4="$(git rev-parse HEAD)"

git checkout -q criss-right-deeper
merge_with_resolution "$CRISS_LEFT_3" "criss-right-deeper-merge" "maze.txt" "right-deeper-merge-resolution"
CRISS_RIGHT_4="$(git rev-parse HEAD)"

git tag criss-left-deeper-tip "$CRISS_LEFT_4"
git tag criss-right-deeper-tip "$CRISS_RIGHT_4"

git checkout -q main
git checkout -q -b rebase-source
write_and_commit "rebase-source-1" bash -c 'printf "rebase source 1\n" > rebase.txt'
write_and_commit "rebase-source-2" bash -c 'printf "rebase source 2\n" > rebase.txt'
REBASE_OLD_TIP="$(git rev-parse HEAD)"

git checkout -q main
write_and_commit "main-advance-1" bash -c 'printf "main advance 1\n" > mainline.txt'
write_and_commit "main-advance-2" bash -c 'printf "main advance 2\n" > mainline.txt'
REBASE_NEW_BASE="$(git rev-parse HEAD)"

git checkout -q rebase-source
git rebase -q main
REBASE_NEW_TIP="$(git rev-parse HEAD)"
git tag rebase-old-tip "$REBASE_OLD_TIP"
git tag rebase-new-base "$REBASE_NEW_BASE"
git tag rebase-new-tip "$REBASE_NEW_TIP"

git checkout -q main
git checkout -q -b duplicate-patch-source
write_and_commit "duplicate-patch-source-1" bash -c 'printf "shared patch\n" > duplicate.txt'
DUPLICATE_SOURCE_1="$(git rev-parse HEAD)"
write_and_commit "duplicate-patch-source-2" bash -c 'printf "shared patch\nunique tail\n" > duplicate.txt'
DUPLICATE_SOURCE_2="$(git rev-parse HEAD)"

git checkout -q main
git checkout -q -b duplicate-patch-target
write_and_commit "duplicate-patch-target-1" bash -c 'printf "shared patch\n" > duplicate.txt'
DUPLICATE_TARGET_1="$(git rev-parse HEAD)"
git cherry-pick -q "$DUPLICATE_SOURCE_2" >/dev/null 2>&1 || true
printf "shared patch\nunique tail\ncherry-picked-via-resolution\n" > duplicate.txt
git add duplicate.txt
set_time
git cherry-pick --continue -q >/dev/null 2>&1 || git commit -q -m "duplicate-patch-target-2"
DUPLICATE_TARGET_2="$(git rev-parse HEAD)"

git checkout -q duplicate-patch-source
git rebase -q duplicate-patch-target >/dev/null 2>&1 || true
printf "shared patch\nunique tail\nrebased-after-duplicate-patch\n" > duplicate.txt
git add duplicate.txt
set_time
git rebase --continue >/dev/null 2>&1
DUPLICATE_REBASED_TIP="$(git rev-parse HEAD)"
git tag duplicate-source-tip "$DUPLICATE_SOURCE_2"
git tag duplicate-target-tip "$DUPLICATE_TARGET_2"
git tag duplicate-rebased-tip "$DUPLICATE_REBASED_TIP"

git checkout -q main
git checkout -q -b octo-a
write_and_commit "octo-a" bash -c 'printf "octo a\n" > octo-a.txt'
git checkout -q main
git checkout -q -b octo-b
write_and_commit "octo-b" bash -c 'printf "octo b\n" > octo-b.txt'
git checkout -q main
git checkout -q -b octo-c
write_and_commit "octo-c" bash -c 'printf "octo c\n" > octo-c.txt'
git checkout -q main
git merge -q --no-ff octo-a octo-b octo-c -m "octopus-merge" >/dev/null 2>&1
OCTOPUS_TIP="$(git rev-parse HEAD)"
git tag octopus-tip "$OCTOPUS_TIP"

git checkout -q main
git checkout -q -b evil-left
write_and_commit "evil-left" bash -c 'printf "evil left\n" > evil.txt'
EVIL_LEFT="$(git rev-parse HEAD)"

git checkout -q main
git checkout -q -b evil-right
write_and_commit "evil-right" bash -c 'printf "evil right\n" > evil.txt'
EVIL_RIGHT="$(git rev-parse HEAD)"

git checkout -q evil-left
EVIL_MERGE="$(write_commit_tree "evil-merge" "not a real merge result" -p "$EVIL_LEFT" -p "$EVIL_RIGHT")"
git reset -q --hard "$EVIL_MERGE"
git branch -f evil-merge "$EVIL_MERGE"
git tag evil-merge-tip "$EVIL_MERGE"

git checkout -q main
git checkout -q -b evil-left-2
write_and_commit "evil-left-2" bash -c 'printf "evil left 2\n" > evil2.txt'
EVIL_LEFT_2="$(git rev-parse HEAD)"

git checkout -q main
git checkout -q -b evil-right-2
write_and_commit "evil-right-2" bash -c 'printf "evil right 2\n" > evil2.txt'
EVIL_RIGHT_2="$(git rev-parse HEAD)"

git checkout -q evil-left-2
EVIL_MERGE_2="$(write_commit_tree "evil-merge-2" "synthetic merge result" -p "$EVIL_RIGHT_2" -p "$EVIL_LEFT_2")"
git reset -q --hard "$EVIL_MERGE_2"
git branch -f evil-merge-2 "$EVIL_MERGE_2"
git tag evil-merge-2-tip "$EVIL_MERGE_2"

git checkout -q "$EVIL_MERGE"
write_and_commit "evil-after-merge" bash -c 'printf "evil after merge\n" > evil.txt'
EVIL_AFTER_MERGE="$(git rev-parse HEAD)"
git tag evil-after-merge-tip "$EVIL_AFTER_MERGE"

git checkout -q main
git branch scenario/criss-left "$CRISS_LEFT_2"
git branch scenario/criss-right "$CRISS_RIGHT_2"
git branch scenario/criss-left-deeper "$CRISS_LEFT_4"
git branch scenario/criss-right-deeper "$CRISS_RIGHT_4"
git branch scenario/skew-left "$SKEW_LEFT_TIP"
git branch scenario/skew-right "$SKEW_RIGHT_TIP"
git branch scenario/rebase-old "$REBASE_OLD_TIP"
git branch scenario/rebase-new "$REBASE_NEW_TIP"
git branch scenario/duplicate-rebased "$DUPLICATE_REBASED_TIP"
git branch scenario/evil "$EVIL_MERGE"
git branch scenario/evil-2 "$EVIL_MERGE_2"
