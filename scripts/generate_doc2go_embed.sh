#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/docs/site/api"
GOCACHE_DIR="${ROOT_DIR}/.cache/go-build"

if ! command -v doc2go >/dev/null 2>&1; then
    echo "doc2go not found. Install it with: go install go.abhg.dev/doc2go@latest" >&2
    exit 1
fi

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"
mkdir -p "${GOCACHE_DIR}"

cd "${ROOT_DIR}"
GOCACHE="${GOCACHE_DIR}" doc2go \
    -embed \
    -out "${OUT_DIR}" \
    ./gitcore
