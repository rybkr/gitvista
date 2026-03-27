#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TARGET_DIR="${1:-$REPO_ROOT/testdata/status-traps}"

if [[ -e "$TARGET_DIR" ]]; then
    rm -rf "$TARGET_DIR"
fi

mkdir -p "$TARGET_DIR"
cd "$TARGET_DIR"

git init -q
git config user.name "GitVista Test"
git config user.email "test@gitvista.io"
git config core.filemode true
git branch -M main

cat > README.md <<'EOF'
# Status trap repository

Generated working tree for `git status` / `gitvista-cli status` stress testing.

This repository intentionally leaves the working tree dirty with:

- unstaged mode-only changes
- staged mode-only changes
- unstaged content edits
- staged content edits
- files staged and then modified again
- files staged and then modified again with an added mode flip
- unstaged deletions
- staged deletions
- staged additions
- staged additions modified again
- file-to-symlink type changes
- staged file-to-symlink type changes
- untracked files and ignored files
EOF

printf "clean\n" > clean.txt
printf '#!/bin/sh\nprintf "mode worktree\\n"\n' > mode-worktree.sh
printf '#!/bin/sh\nprintf "mode staged\\n"\n' > mode-staged.sh
printf "base modified\n" > modified.txt
printf "base staged\n" > staged.txt
printf '#!/bin/sh\nprintf "baseline\\n"\n' > staged-then-modified.sh
printf "delete me later\n" > deleted.txt
printf "delete me in index\n" > deleted-staged.txt
printf "replace me with a symlink later\n" > typechange.txt
printf "replace me with a staged symlink later\n" > staged-typechange.txt
printf "space path baseline\n" > "space name.txt"
printf "*.log\n" > .gitignore

chmod 0644 \
    mode-worktree.sh \
    mode-staged.sh \
    staged-then-modified.sh

git add -A
git commit -q -m "baseline status trap"

chmod 0755 mode-worktree.sh

chmod 0755 mode-staged.sh
git add mode-staged.sh

printf "worktree edit\n" >> modified.txt

printf "staged edit\n" >> staged.txt
git add staged.txt

printf '#!/bin/sh\nprintf "staged copy\\n"\n' > staged-then-modified.sh
git add staged-then-modified.sh
printf 'printf "worktree tail\\n"\n' >> staged-then-modified.sh
chmod 0755 staged-then-modified.sh

rm deleted.txt

rm deleted-staged.txt
git add -u -- deleted-staged.txt

printf "new staged file\n" > added.txt
git add added.txt

printf "new staged file\n" > added-then-modified.txt
git add added-then-modified.txt
printf "worktree tail\n" >> added-then-modified.txt

printf "space path worktree edit\n" >> "space name.txt"

rm typechange.txt
ln -s README.md typechange.txt

rm staged-typechange.txt
ln -s README.md staged-typechange.txt
git add -A -- staged-typechange.txt

printf "ignored\n" > ignored.log
printf "visible untracked\n" > untracked.txt
mkdir -p nested
printf "nested untracked\n" > nested/untracked-nested.txt

echo "Created status trap repository at $TARGET_DIR"
