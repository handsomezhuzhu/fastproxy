#!/usr/bin/env bash
# Runs ON THE VPS: kill the created-but-never-started task by full id, then rm.
set -euo pipefail

ID=$(docker ps -aq --filter 'name=fastproxy' | head -1 || true)
if [ -z "${ID:-}" ]; then
  echo "no stale fastproxy container"
else
  echo "stale: $ID"
  PID=$(ctr -n moby tasks ls 2>/dev/null | awk -v id="$ID" '$1 ~ id {print $2; exit}')
  echo "task pid: ${PID:-none}"
  if [ -n "${PID:-}" ]; then
    ctr -n moby tasks kill "$ID" 2>&1 || true
    kill -9 "$PID" 2>/dev/null || true
    sleep 1
  fi
  ctr -n moby tasks rm "$ID" 2>&1 || true
  sleep 1
  docker rm -f "$ID" && echo "removed" || echo "remove failed"
fi
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Status}}'
