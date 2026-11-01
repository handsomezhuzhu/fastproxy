#!/usr/bin/env bash
# Runs ON THE VPS: reset the (iteration-stage) data dir with a timestamped
# backup, then bring up the latest GHCR image via compose.
set -euo pipefail

cd /opt/fastproxy
docker rm -f fastproxy 2>/dev/null || true

mkdir -p /opt/fastproxy/backups
if [ -f data/fastproxy.db ]; then
  dest="backups/data-$(date +%Y%m%d-%H%M%S)"
  mv data "$dest"
  echo "legacy data moved to $dest"
fi

docker compose pull
docker compose up -d
sleep 3
docker ps --filter name=fastproxy --format '{{.Status}}'
curl -s -o /dev/null -w "loopback panel: %{http_code}\n" http://127.0.0.1:7788/api/session
curl -s http://127.0.0.1:7788/api/session
