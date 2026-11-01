#!/usr/bin/env bash
# Verifies the v0.5 model on the server from a clean-network jump box:
# fresh DB -> probe -> create port -> bind default account -> chain test.
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
extract() { python3 -c '
import sys, json
d = json.load(sys.stdin)
if isinstance(d, dict) and "error" in d:
    sys.stderr.write("API error: " + str(d["error"]) + "\n"); sys.exit(9)
for k in sys.argv[1].split("."):
    d = d[k]
print(d)' "$1"; }

echo "== 2. import upstreams + probe =="
TEXT=$(cat deploy/sample-upstreams.txt)
api -d "{\"text\":$(python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))' <<<"$TEXT")}" "$BASE/api/upstreams/import" | python3 -m json.tool
probe=$(api -X POST "$BASE/api/upstreams/check-all")
echo "$probe"

echo "== 3. 新模型: 建端口 -> 默认账号 -> 绑上游 =="
UP_ID=$(api "$BASE/api/data" | python3 -c '
import json, sys
d = json.load(sys.stdin)
healthy = [u["id"] for u in d["upstreams"] if u.get("health") == "ok"]
print(healthy[0] if healthy else d["upstreams"][0]["id"])')
echo "选用上游: $UP_ID"

RESP=$(api -d '{"port":10206,"remark":"v5-verify"}' "$BASE/api/ports")
echo "$RESP" | python3 -m json.tool
PORT_ID=$(echo "$RESP" | extract 'saved.port.id')
DEF_ID=$(echo "$RESP" | extract 'saved.default_account.id')
DEF_USER=$(echo "$RESP" | extract 'saved.default_account.username')
DEF_PASS=$(echo "$RESP" | extract 'saved.default_account.password')
echo "端口 $PORT_ID, 默认账号 $DEF_USER / $DEF_PASS"

api -X PUT -d "{\"username\":\"$DEF_USER\",\"password\":\"$DEF_PASS\",\"port_id\":\"$PORT_ID\",\"upstream_id\":\"$UP_ID\",\"remark\":\"\"}" \
  "$BASE/api/accounts/$DEF_ID" | extract 'saved.id' >/dev/null
echo "默认账号已绑上游"

echo "== 4. 删除默认账号保护校验（应拒绝）=="
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -X DELETE "$BASE/api/accounts/$DEF_ID")
echo "DELETE 默认账号 -> $code"

echo "== 5. 跨端口拒认证校验（alice 只绑本端口应是唯一可用账号）=="
# 给另外一个端口再建一个账号，认证到其他端口必须 407
RESP2=$(api -d '{"port":10207,"remark":"v5-other"}' "$BASE/api/ports")
ALICE_PORT_ID=$(echo "$RESP2" | extract 'saved.port.id')
api -d "{\"username\":\"alice-test\",\"password\":\"wonder\",\"port_id\":\"$ALICE_PORT_ID\",\"upstream_id\":\"$UP_ID\"}" "$BASE/api/accounts" | extract 'saved.id' >/dev/null
sleep 2
wrong=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -x "http://alice-test:wonder@${VPS_HOST}:10206" "http://api.ipify.org")
echo "alice-test 在 10206 -> $wrong (期望 407)"

echo "== 6. 真实链路（默认账号走上游）=="
ADDR="${VPS_HOST}:10206"
echo "http proxy:  $(curl -s --max-time 25 -x "http://${DEF_USER}:${DEF_PASS}@$ADDR" "http://api.ipify.org" || echo FAIL)"
echo "https(conn): $(curl -s --max-time 25 -x "http://${DEF_USER}:${DEF_PASS}@$ADDR" "https://api.ipify.org" || echo FAIL)"
echo "socks5:      $(curl -s --max-time 25 --proxy "socks5h://${DEF_USER}:${DEF_PASS}@$ADDR" "https://api.ipify.org" || echo FAIL)"
echo "wrong pass:  $(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -x "http://${DEF_USER}:bad@$ADDR" "http://api.ipify.org")"

echo "== 7. metrics 接口 =="
api "$BASE/api/metrics/hours" | python3 -c '
import json, sys
d = json.load(sys.stdin)
pts = d["points"]
print("points:", len(pts), "conns-sum:", sum(p["conns"] for p in pts))'
