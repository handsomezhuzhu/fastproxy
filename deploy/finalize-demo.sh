#!/usr/bin/env bash
# Finalize the demo state on the VPS: delete test leftovers, put the 10206
# default account back to direct egress (works out of the box once user binds
# their own upstream).
set -euo pipefail
BASE="http://${VPS_HOST:?}:7788"
PASS="${ADMIN_PASSWORD:?}"
C=/tmp/fp-cookie
rm -f "$C"
curl -fsS -c "$C" -H 'Content-Type: application/json' -d "{\"password\":\"$PASS\"}" "$BASE/api/login" >/dev/null
api(){ curl -fsS -b "$C" -H 'Content-Type: application/json' "$@"; }

rows=$(api "$BASE/api/data" | python3 -c '
import json, sys
d = json.load(sys.stdin)
for a in d["accounts"]:
    print("\t".join([a["id"], a["username"], a["password"], a["port_id"], a["upstream_id"], str(a["is_default"]), "acc"]))
for p in d["ports"]:
    print("\t".join([p["id"], str(p["port"]), "", "", "", "", "port"]))
')

while IFS=$'\t' read -r id name pwd pid uid isdef kind; do
  if [ "$kind" = "acc" ] && [ "$name" = "alice-test" ]; then
    api -X DELETE "$BASE/api/accounts/$id" >/dev/null && echo "deleted alice-test"
  fi
  if [ "$kind" = "port" ] && [ "$name" = "10207" ]; then
    api -X DELETE "$BASE/api/ports/$id" >/dev/null && echo "deleted port 10207"
  fi
  if [ "$kind" = "acc" ] && [ "$isdef" = "True" ] && [ "$name" = "default-10206" ]; then
    api -X PUT -d "{\"username\":\"$name\",\"password\":\"$pwd\",\"port_id\":\"$pid\",\"upstream_id\":\"\",\"remark\":\"\"}" \
      "$BASE/api/accounts/$id" >/dev/null
    echo "default-10206 upstream -> direct"
  fi
done <<< "$rows"

echo "=== final state ==="
api "$BASE/api/data" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print("ports:", [(p["port"], p["id"]) for p in d["ports"]])
for a in d["accounts"]:
    print("acc:", a["username"], "| port:", a["port_id"], "| upstream_id:", a["upstream_id"] or "direct", "| default:", a["is_default"])
print("upstreams:", [u["name"] for u in d["upstreams"]])'
