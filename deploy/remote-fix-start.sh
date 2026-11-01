#!/usr/bin/env bash
# Runs ON THE VPS: bring the fastproxy container back up and report why it was Created.
set -uo pipefail
cd /opt/fastproxy || { echo "no /opt/fastproxy"; exit 1; }
echo "== compose file =="
cat docker-compose.yml
echo
echo "== previous status =="
docker inspect -f '{{.State.Status}} exit={{.State.ExitCode}} err={{.State.Error}} started={{.State.StartedAt}}' fastproxy 2>&1 || true
echo
echo "== recreate =="
docker rm -f fastproxy 2>&1 || true
docker compose up -d 2>&1
sleep 5
echo
echo "== status =="
docker ps -a --filter name=fastproxy --format '{{.Status}} | {{.Ports}}' 2>&1
curl -s -m 8 -o /dev/null -w "loopback 7788 -> %{http_code}\n" http://127.0.0.1:7788/api/session
echo
echo "== logs =="
docker logs --tail 30 fastproxy 2>&1 | tail -30
