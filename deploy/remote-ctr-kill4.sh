#!/usr/bin/env bash
# Runs ON THE VPS: disable the restart policy so kill+remove actually sticks.
set -euo pipefail

SHORT=$(docker ps -aq --filter 'name=fastproxy' | head -1 || true)
if [ -z "${SHORT:-}" ]; then
  echo "no stale container"; exit 0
fi
echo "disabling restart policy on $SHORT"
docker update --restart=no "$SHORT" || true

echo "killing task"
docker kill "$SHORT" 2>&1 || true
FULL=$(docker inspect --format '{{.Id}}' "$SHORT" 2>/dev/null || true)
PID=$(ctr -n moby tasks ls 2>/dev/null | awk -v id="${FULL:-}" '$1 == id {print $2; exit}')
if [ -n "${PID:-}" ]; then
  ctr -n moby tasks kill "$FULL" 2>&1 || true
  kill -9 "$PID" 2>/dev/null || true
fi
sleep 2
ctr -n moby tasks rm "$FULL" 2>&1 || true
docker rm -f "$SHORT" && echo "removed $SHORT" || echo "remove failed"
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Status}}'
