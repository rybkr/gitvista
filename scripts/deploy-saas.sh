#!/usr/bin/env bash
set -euo pipefail

# Deploy GitVista to Fly.io.
# Creates the persistent volume if it doesn't already exist.

REGION="${FLY_REGION:-iad}"
VOLUME_SIZE="${FLY_VOLUME_SIZE:-10}"

echo "Ensuring Fly volume exists (region=$REGION, size=${VOLUME_SIZE}GB)..."
fly volumes create gitvista_data --region "$REGION" --size "$VOLUME_SIZE" --yes 2>/dev/null || true

echo "Deploying..."
fly deploy
