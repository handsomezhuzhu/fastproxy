#!/usr/bin/env bash
# Runs ON THE VPS: check the current fastproxy container state.
set -uo pipefail
echo "== docker ps =="
docker ps -a --format '{{.Names}} | {{.Status}} | {{.Image}} | {{.Ports}}' 2>&1 | head -10
echo
echo "== images =="
docker images --format '{{.Repository}}:{{.Tag}} | {{.ID}} | {{.CreatedSince}}' 2>&1 | head -10
echo
echo "== compose ps =="
cd /opt/fastproxy 2>/dev/null && docker compose ps 2>&1 | head -20 || echo "no compose dir"
echo
echo "== panel loopback =="
curl -s -m 6 -o /dev/null -w "127.0.0.1:7788 -> %{http_code}\n" http://127.0.0.1:7788/api/session
echo
echo "== container logs (tail 40) =="
docker logs --tail 40 fastproxy 2>&1 | tail -40
echo
echo "== docker service =="
systemctl is-active docker
