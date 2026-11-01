#!/usr/bin/env bash
# Verifies the end-to-end chain on the server from a clean-network jump box:
# login -> import real upstreams -> probe -> bind port -> create account ->
# pass traffic through and compare exit IPs.
set -euo pipefail

BASE="http://${VPS_HOST:?}:7788"
PASS="${ADMIN_PASSWORD:?}"
COOKIE=/tmp/fp-cookie
rm -f "$COOKIE"

echo "== 1. login =="
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"password\":\"$PASS\"}" "$BASE/api/login" >/dev/null
echo "login ok"
api() { curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' "$@"; }

echo "== 2. import real upstreams =="
TEXT=$(cat deploy/sample-upstreams.txt)
api -d "{\"text\":$(python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))' <<<"$TEXT")}" "$BASE/api/upstreams/import" | python3 -m json.tool

echo "== 3. probe all =="
probe=$(api -X POST "$BASE/api/upstreams/check-all")
echo "$probe"

echo "== 4. pick a healthy upstream & bind port 10206 =="
UP_ID=$(api "$BASE/api/data" | python3 -c '
import json, sys
d = json.load(sys.stdin)
healthy = [u["id"] for u in d["upstreams"] if u.get("health") == "ok"]
print(healthy[0] if healthy else d["upstreams"][0]["id"])')
echo "upstream: $UP_ID"
api -d "{\"port\":10206,\"default_upstream\":\"$UP_ID\",\"remark\":\"proxyscrape-test\"}" "$BASE/api/ports" | python3 -m json.tool
if ! api "$BASE/api/data" | grep -q '"username":"demo"'; then
  api -d '{"username":"demo","password":"demo123","remark":"e2e"}' "$BASE/api/accounts" | python3 -m json.tool
fi

sleep 2
echo "== 5. chain test =="
ADDR="${VPS_HOST}:10206"
echo "http proxy:  $(curl -s --max-time 25 -x "http://demo:demo123@$ADDR" "http://api.ipify.org" || echo FAIL)"
echo "https(conn): $(curl -s --max-time 25 -x "http://demo:demo123@$ADDR" "https://api.ipify.org" || echo FAIL)"
echo "socks5:      $(curl -s --max-time 25 --proxy "socks5h://demo:demo123@$ADDR" "https://api.ipify.org" || echo FAIL)"
echo "wrong pass:  $(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -x "http://demo:bad@$ADDR" "http://api.ipify.org")"

echo "== 6. upstream health/exit ip =="
api "$BASE/api/data" | python3 -c '
import json, sys
d = json.load(sys.stdin)
for u in d["upstreams"]:
    print(u["name"], "| health:", u.get("health"), "| latency:", u.get("latency_ms"), "| exit-ip:", u.get("exit_ip"))
print("server-ip:", d["probe"]["server_ip"])'
