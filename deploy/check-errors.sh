#!/usr/bin/env bash
# Runs on the jump box: fetch proxyscrape upstream failure reasons from panel.
set -euo pipefail
BASE="http://${VPS_HOST:?}:7788"
PASS="${ADMIN_PASSWORD:?}"
C=/tmp/fpc
rm -f "$C"
curl -fsS -c "$C" -H 'Content-Type: application/json' -d "{\"password\":\"$PASS\"}" "$BASE/api/login" >/dev/null
api(){ curl -fsS -b "$C" -H 'Content-Type: application/json' "$@"; }

echo "== upstream failures =="
api "$BASE/api/data" | python3 -c '
import json, sys
d = json.load(sys.stdin)
for u in d["upstreams"]:
    if u.get("health") == "fail":
        print(u["name"], "->", u.get("last_error", ""))'

echo "== direct tcp test (from VPS view via public ip targets) =="
python3 - <<'PY'
import socket
for host, port in [("45.3.38.106",3129),("104.207.37.113",3129),("45.3.48.250",3129)]:
    s = socket.socket(); s.settimeout(5)
    try:
        s.connect((host, port)); print(host, port, "OPEN")
    except Exception as e:
        print(host, port, "ERR:", e)
    finally:
        s.close()
PY
