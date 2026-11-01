#!/usr/bin/env bash
# Runs ON THE VPS: force-kill the wedged container via containerd task API.
set -euo pipefail

echo "=== ctr tasks ==="
for ns in moby docker; do
  if command -v ctr >/dev/null; then
    ctr -n "$ns" tasks ls 2>/dev/null && nsfound=$ns && break
  fi
done
nsfound="${nsfound:-moby}"
echo "namespace: $nsfound"

if command -v ctr >/dev/null; then
  ctr -n "$nsfound" tasks kill fastproxy 2>&1 || true
  ctr -n "$nsfound" tasks rm fastproxy 2>&1 || true
fi

# also purge shim if still present
shims=$(pgrep -f "containerd-shim.*fastproxy" || true)
if [ -n "${shims:-}" ]; then
  echo "killing shims: $shims"
  kill -9 $shims || true
fi

sleep 2
stale=$(docker ps -aq --filter 'name=fastproxy' 2>/dev/null || true)
if [ -n "${stale:-}" ]; then
  docker rm -f $stale && echo "removed $stale" || echo "remove failed"
else
  echo "stale gone"
fi
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Status}}'
