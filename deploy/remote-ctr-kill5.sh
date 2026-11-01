#!/usr/bin/env bash
# Runs ON THE VPS: last-resort cleanup of the wedged "Restarting" container by
# patching its on-disk record (restart policy off, state dead), then removing.
set -euo pipefail

SHORT=$(docker ps -aq --filter 'name=fastproxy' | head -1 || true)
if [ -z "${SHORT:-}" ]; then
  echo "no stale container"; exit 0
fi
FULL=$(docker inspect --format '{{.Id}}' "$SHORT")
echo "stale: $SHORT ($FULL)"

echo "stop docker+containerd..."
systemctl stop docker.socket docker containerd 2>&1 || true
pkill -9 -f containerd-shim 2>/dev/null || true
sleep 3

CFG="/var/lib/docker/containers/$FULL/config.v2.json"
if [ ! -f "$CFG" ]; then
  echo "config missing: $CFG"
else
  python3 - "$FULL" <<'PY'
import json, sys, datetime
path = f"/var/lib/docker/containers/{sys.argv[1]}/config.v2.json"
with open(path) as f:
    cfg = json.load(f)
cfg["RestartPolicy"] = {"Name": "", "MaximumRetryCount": 0}
st = cfg.get("State", {})
st.update({
    "Running": False, "Paused": False, "Restarting": False,
    "OOMKilled": False, "Dead": True, "Pid": 0,
    "ExitCode": 137, "Error": "manually wedged container cleanup",
    "FinishedAt": datetime.datetime.utcnow().isoformat("T") + "Z",
})
cfg["State"] = st
with open(path, "w") as f:
    json.dump(cfg, f)
print("patched config.v2.json")
PY
fi

systemctl start containerd docker
for i in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 2; done
docker info --format 'docker {{json .ServerVersion}} ok'

stale=$(docker ps -aq --filter 'name=fastproxy' | head -1 || true)
if [ -n "${stale:-}" ]; then
  docker rm -f $stale && echo "removed $stale" || echo "remove failed"
else
  echo "stale gone"
fi
docker ps -a --format '{{.ID}} | {{.Names}} | {{.Status}}'
