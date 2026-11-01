#!/usr/bin/env bash
# Runs ON THE VPS: kill stale container by its FULL containerd task id.
set -euo pipefail

SHORT=$(docker ps -aq --filter 'name=fastproxy' | head -1 || true)
if [ -z "${SHORT:-}" ]; then
  echo "no stale fastproxy container"; exit 0
fi
FULL=$(docker inspect --format '{{.Id}}' "$SHORT" 2>/dev/null || true)
echo "short=$SHORT full=$FULL"
if [ -z "${FULL:-}" ]; then echo "no full id"; exit 1; fi

PID=$(ctr -n moby tasks ls 2>/dev/null | awk -v id="$FULL" '$1 == id {print $2; exit}')
echo "task pid: ${PID:-none}"
if [ -n "${PID:-}" ]; then
  ctr -n moby tasks kill "$FULL" 2>&1 || true
  kill -9 "$PID" 2>/dev/null || true
  sleep 1
fi
ctr -n moby tasks rm "$FULL" 2>&1 || true
sleep 1
docker rm -f "$SHORT" && echo "removed" || echo "remove failed"
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Status}}'
