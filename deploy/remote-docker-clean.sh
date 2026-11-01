#!/usr/bin/env bash
# Runs ON THE VPS: show containers, force-remove the stale one, deploy.
set -euo pipefail

echo "=== containers before ==="
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Image}} | {{.Status}}' || docker info --format '{{json .ServerVersion}}'

stale=$(docker ps -aq --filter 'name=fastproxy' 2>/dev/null || true)
if [ -n "${stale:-}" ]; then
  echo "removing stale: $stale"
  docker kill $stale 2>&1 || true
  docker rm -f $stale 2>&1 || true
fi

echo "=== containers after ==="
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Image}} | {{.Status}}'
