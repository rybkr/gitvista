#!/usr/bin/env bash
set -euo pipefail

# Post-deploy smoke tests for GitVista.
# Usage: bash scripts/smoke-test.sh <base-url>
# Example: bash scripts/smoke-test.sh https://gitvista-staging.fly.dev

BASE_URL="${1:?Usage: smoke-test.sh <base-url>}"
TIMEOUT="${SMOKE_TIMEOUT:-30}"
INTERVAL="${SMOKE_INTERVAL:-5}"

# Strip trailing slash
BASE_URL="${BASE_URL%/}"

passed=0
failed=0

check() {
  local name="$1"
  shift
  if "$@"; then
    echo "  PASS: $name"
    ((passed++))
  else
    echo "  FAIL: $name"
    ((failed++))
  fi
}

wait_for_healthy() {
  local elapsed=0
  echo "Waiting for $BASE_URL to become healthy (timeout: ${TIMEOUT}s)..."
  while [ "$elapsed" -lt "$TIMEOUT" ]; do
    if curl -sf --max-time 5 "$BASE_URL/health" >/dev/null 2>&1; then
      echo "Service is healthy after ${elapsed}s"
      return 0
    fi
    sleep "$INTERVAL"
    elapsed=$((elapsed + INTERVAL))
  done
  echo "Service did not become healthy within ${TIMEOUT}s"
  return 1
}

check_health() {
  local status
  status=$(curl -sf --max-time 10 -o /dev/null -w '%{http_code}' "$BASE_URL/health")
  [ "$status" = "200" ]
}

check_api_repository() {
  local body
  body=$(curl -sf --max-time 10 "$BASE_URL/api/repository" 2>/dev/null) || return 1
  echo "$body" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null
}

check_websocket() {
  # Attempt a WebSocket upgrade — expect 101 or at least a non-error response
  local status
  status=$(curl -sf --max-time 10 -o /dev/null -w '%{http_code}' \
    -H "Upgrade: websocket" \
    -H "Connection: Upgrade" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    -H "Sec-WebSocket-Version: 13" \
    "$BASE_URL/api/ws" 2>/dev/null) || true
  # 101 = successful upgrade, 400 = server saw the request (acceptable for basic connectivity check)
  [ "$status" = "101" ] || [ "$status" = "400" ]
}

echo "Running smoke tests against $BASE_URL"
echo "---"

# Wait for service to come up before running checks
if ! wait_for_healthy; then
  echo ""
  echo "RESULT: Service unreachable — all tests skipped"
  exit 1
fi

echo ""
echo "Running checks..."
check "GET /health returns 200" check_health
check "GET /api/repository returns valid JSON" check_api_repository
check "WebSocket endpoint is reachable" check_websocket

echo ""
echo "---"
echo "RESULT: $passed passed, $failed failed"

if [ "$failed" -gt 0 ]; then
  exit 1
fi
