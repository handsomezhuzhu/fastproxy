#!/usr/bin/env bash
# Runs ON THE VPS: recover a stuck "Restarting" container by restarting
# containerd (kills every shim), then remove it.
set -euo pipefail

echo "restarting containerd+docker..."
systemctl restart containerd 2>/dev/null || systemctl restart docker-containerd 2>/dev/null || systemctl restart docker
sleep 4

for i in $(seq 1 30); do
  docker info >/dev/null 2>&1 && break
  sleep 2
done
docker info --format 'docker {{json .ServerVersion}} ok'

stale=$(docker ps -aq --filter 'name=fastproxy' 2>/dev/null || true)
if [ -n "${stale:-}" ]; then
  docker rm -f $stale && echo "removed $stale" || echo "remove failed"
else
  echo "no stale fastproxy container"
fi
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Status}}'
