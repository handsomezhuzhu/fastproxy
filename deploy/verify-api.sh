#!/usr/bin/env bash
# Verify v0.6 APIs on the VPS: paged list, bulk accounts, bulk ports, log paging.
set -euo pipefail
BASE="http://${VPS_HOST:?}:7788"
PASS="${ADMIN_PASSWORD:?}"
C=/tmp/fp-cookie
rm -f "$C"
curl -fsS -c "$C" -H 'Content-Type: application/json' -d "{\"password\":\"$PASS\"}" "$BASE/api/login" >/dev/null
api(){ curl -fsS -b "$C" -H 'Content-Type: application/json' "$@"; }

echo "== paged upstreams with keyword =="
api "$BASE/api/upstreams?limit=4&offset=0&q=209.50" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print("total:", d["total"], "items:", [(u["name"], u["host"]) for u in d["items"]])'

echo "== bulk accounts (default-protection) =="
DATA=$(api "$BASE/api/data")
DEF_ID=$(echo "$DATA" | python3 -c 'import json,sys
for a in json.load(sys.stdin)["accounts"]:
    if a["is_default"]: print(a["id"]); break')
RESP=$(api -X POST -d "{\"ids\":[\"$DEF_ID\"],\"action\":\"delete\"}" "$BASE/api/accounts/bulk")
echo "bulk delete default => $RESP"

echo "== bulk ports enable/disable =="
echo "$DATA" | python3 -c 'import json,sys
print("\n".join(p["id"] for p in json.load(sys.stdin)["ports"]))' > /tmp/pids
PORT_IDS=$(cat /tmp/pids | python3 -c 'import sys,json;print(json.dumps([l.strip() for l in sys.stdin if l.strip()]))')
api -X POST -d "{\"ids\":$PORT_IDS,\"action\":\"disable\"}" "$BASE/api/ports/bulk"
echo
api -X POST -d "{\"ids\":$PORT_IDS,\"action\":\"enable\"}" "$BASE/api/ports/bulk"
echo

echo "== port create response shape (crash path) =="
NEW=$(api -X POST -d '{"port":10320,"remark":"v6-verify"}' "$BASE/api/ports")
echo "$NEW" | python3 -m json.tool
PID=$(echo "$NEW" | python3 -c 'import sys,json;d=json.load(sys.stdin);s=d["saved"];print(s["port"]["id"])')
echo "delete created => $(api -X DELETE "$BASE/api/ports/$PID" && echo ok)"

echo "== logs paging =="
api "$BASE/api/logs?limit=5&offset=0" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print("total:", d["total"], "page1:", len(d["entries"]))'
api "$BASE/api/logs?limit=5&offset=5" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print("total:", d["total"], "page2:", len(d["entries"]))'

echo "== server bundle asset =="
curl -fsS "$BASE/" | grep -o "assets/index-[^\"]*\.js"
