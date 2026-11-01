#!/usr/bin/env bash
# UI end-to-end run: build the SPA, build the Go binary with go:embed assets,
# boot it on a throwaway DB, then drive it with Playwright/Chromium.
#
# Used by .github/workflows/ci.yml (job ui-e2e) and for cloud checks via
#   npx -y instabox@latest run --push . --timeout 10m \
#     -e ADMIN_PASSWORD=e2e-admin-pass -- 'bash scripts/ui-e2e-ci.sh'
#
# Env: ADMIN_PASSWORD (required), ADMIN_ADDR (default 127.0.0.1:7788),
#      BASE_URL (default http://$ADMIN_ADDR), E2E_SKIP_INSTALL=1 to reuse Chromium.
set -euo pipefail

ADMIN_ADDR="${ADMIN_ADDR:-127.0.0.1:7788}"
BASE_URL="${BASE_URL:-http://${ADMIN_ADDR}}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:?ADMIN_PASSWORD is required}"
E2E_DIR="$(mktemp -d "${TMPDIR:-/tmp}/fp-e2e.XXXXXX")"
SERVER_PID=""

log() { echo "[ui-e2e] $*"; }

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

cd "$(dirname "$0")/.."

log "build web (writes admin/dist for go:embed)"
(cd web && npm ci --no-audit --no-fund && npm run build)

log "build fastproxy"
go build -tags with_utls -trimpath -o "$E2E_DIR/fastproxy" .

log "start fastproxy on $ADMIN_ADDR (db: $E2E_DIR/fastproxy.db)"
CONFIG="$E2E_DIR/fastproxy.db" ADMIN_ADDR="$ADMIN_ADDR" ADMIN_PASSWORD="$ADMIN_PASSWORD" \
  "$E2E_DIR/fastproxy" >"$E2E_DIR/server.log" 2>&1 &
SERVER_PID=$!

ready=""
for _ in $(seq 1 60); do
  if curl -fsS "http://${ADMIN_ADDR}/api/session" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then break; fi
  sleep 0.5
done
if [ -z "$ready" ]; then
  log "server did not become ready; log:"
  cat "$E2E_DIR/server.log" || true
  exit 1
fi
log "server ready"

if [ "${E2E_SKIP_INSTALL:-0}" != "1" ]; then
  log "install chromium"
  (cd web && npx playwright install --with-deps chromium)
fi

log "playwright test"
(
  cd web
  BASE_URL="$BASE_URL" ADMIN_PASSWORD="$ADMIN_PASSWORD" npx playwright test
)
log "done"
