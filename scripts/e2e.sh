#!/usr/bin/env bash
# FastProxy 容器端到端联调（Linux / instabox 通用）
# 流程: 构建 -> 本地 fake 上游 + 目标站 -> scratch 容器起面板 -> API 配置 -> curl 连通性
set -u
PASS="${E2E_PASSWORD:-admin123}"
PROXY_PORT="${E2E_PROXY_PORT:-10201}"
ADMIN_PORT="${E2E_ADMIN_PORT:-17788}"
fail=0
step() { echo; echo "== $* =="; }
trap 'rm -f /tmp/fp-cookie 2>/dev/null; [ -n "${DEV_PID:-}" ] && kill "$DEV_PID" 2>/dev/null; docker rm -f fastproxy-e2e >/dev/null 2>&1; true' EXIT

step "1. 构建 fastproxy + devserver"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o fastproxy-e2e-linux . || exit 1
CGO_ENABLED=0 go build -o devserver-bin ./scripts/devserver || exit 1

step "2. 启动本地 devserver (fake 上游 :18180 + 目标站 :18181)"
./devserver-bin >devserver.log 2>&1 &
DEV_PID=$!
sleep 1
cat devserver.log

step "3. 构建并启动容器 (scratch, 无基础镜像拉取)"
docker build -f Dockerfile.e2e -t fastproxy:e2e . >/dev/null || exit 1
docker rm -f fastproxy-e2e >/dev/null 2>&1
docker run -d --name fastproxy-e2e \
  -p "${ADMIN_PORT}:7788" -p "${PROXY_PORT}:${PROXY_PORT}" \
  -e ADMIN_PASSWORD="$PASS" fastproxy:e2e >/dev/null || exit 1
ok=""
for i in $(seq 1 60); do
  curl -fsS "http://127.0.0.1:${ADMIN_PORT}/api/session" >/dev/null 2>&1 && { ok=1; break; }
  sleep 0.5
done
if [ -z "$ok" ]; then docker logs fastproxy-e2e; echo "[FAIL] 面板未就绪"; exit 1; fi
echo "[PASS] 面板就绪 (http://127.0.0.1:${ADMIN_PORT})"

step "4. 面板 API 配置（登录/上游/端口/账户）"
COOKIE=/tmp/fp-cookie
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"password\":\"$PASS\"}" "http://127.0.0.1:${ADMIN_PORT}/api/login" >/dev/null || exit 1
echo "[PASS] 登录成功"
api() { curl -sS -b "$COOKIE" -H 'Content-Type: application/json' "$@"; }
# extract 是 JSON 路径取值（如 saved.id）；接口返回 {"error":...} 时打到 stderr 并退出 9
extract() { python3 -c '
import sys, json
d = json.load(sys.stdin)
if isinstance(d, dict) and "error" in d:
    sys.stderr.write("API error: " + str(d["error"]) + "\n")
    sys.exit(9)
for k in sys.argv[1].split("."):
    d = d[k]
print(d)' "$1"; }

UP_ID=$(api -d '{"name":"devserver-upstream","upstream":"http://host.docker.internal:18180"}' \
  "http://127.0.0.1:${ADMIN_PORT}/api/upstreams" | extract 'saved.id') || exit 1
PORT_RESP=$(api -d "{\"port\":${PROXY_PORT},\"remark\":\"e2e\"}" \
  "http://127.0.0.1:${ADMIN_PORT}/api/ports")
PORT_ID=$(echo "$PORT_RESP" | extract 'saved.port.id') || exit 1
DEF_ID=$(echo "$PORT_RESP" | extract 'saved.default_account.id') || exit 1
DEF_USER=$(echo "$PORT_RESP" | extract 'saved.default_account.username') || exit 1
DEF_PASS=$(echo "$PORT_RESP" | extract 'saved.default_account.password') || exit 1
echo "[PASS] 端口创建，默认账号自动生成: ${DEF_USER} / ${DEF_PASS}"

# 默认账号绑定上游（新模型：端口纯入口，账户决定分流）
api -X PUT -d "{\"username\":\"${DEF_USER}\",\"password\":\"${DEF_PASS}\",\"port_id\":\"${PORT_ID}\",\"upstream_id\":\"${UP_ID}\",\"remark\":\"\"}" \
  "http://127.0.0.1:${ADMIN_PORT}/api/accounts/${DEF_ID}" | extract 'saved.id' >/dev/null || exit 1
echo "[PASS] 默认账号已绑定上游 $UP_ID"
# 另建一个普通账号验证普通路径
api -d "{\"username\":\"alice\",\"password\":\"wonder\",\"port_id\":\"${PORT_ID}\",\"upstream_id\":\"${UP_ID}\"}" \
  "http://127.0.0.1:${ADMIN_PORT}/api/accounts" | extract 'saved.id' >/dev/null || exit 1
echo "[PASS] account=alice"

# 容器 -> 宿主机: 先试 host.docker.internal, 不通则热切换 bridge 网关 (Linux Docker)
probe() { curl -s --max-time 8 -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/" 2>/dev/null; }
R=$(probe)
if ! echo "$R" | grep -q hello; then
  GW=$(docker inspect fastproxy-e2e --format '{{(index .NetworkSettings.Networks "bridge").Gateway}}')
  echo "host.docker.internal 不通, 热切换上游到网关 $GW (同时验证 PUT 热加载)"
  api -X PUT -d "{\"name\":\"devserver-upstream\",\"upstream\":\"http://${GW}:18180\"}" \
    "http://127.0.0.1:${ADMIN_PORT}/api/upstreams/${UP_ID}" >/dev/null || exit 1
  R=$(probe)
fi
if echo "$R" | grep -q hello; then echo "[PASS] 容器经上游连通宿主机 devserver"; else echo "[FAIL] 容器无法连通宿主机: $R"; exit 1; fi

step "5. 代理协议连通性"
check() { local name="$1" got="$2" want="$3"; if echo "$got" | grep -q "$want"; then echo "[PASS] $name"; else echo "[FAIL] $name: ${got:0:200}"; fail=$((fail+1)); fi }

check "HTTP 明文代理 (经上游)" "$(curl -s --max-time 15 -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")" hello
check "SOCKS5 代理 (经上游)" "$(curl -s --max-time 15 --proxy "socks5h://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")" hello
check "CONNECT 隧道 (curl --proxytunnel)" "$(curl -s --max-time 15 -p -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")" hello

c407=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -x "http://alice:badpass@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if [ "$c407" = "407" ]; then echo "[PASS] 错误代理密码返回 407"; else echo "[FAIL] 期望 407 实际 $c407"; fail=$((fail+1)); fi

c401=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 "http://127.0.0.1:${ADMIN_PORT}/api/data")
if [ "$c401" = "401" ]; then echo "[PASS] 未登录访问面板 API 返回 401"; else echo "[FAIL] 期望 401 实际 $c401"; fail=$((fail+1)); fi

cbad=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST -H 'Content-Type: application/json' -d '{"password":"wrong"}' "http://127.0.0.1:${ADMIN_PORT}/api/login")
if [ "$cbad" = "401" ]; then echo "[PASS] 面板错误密码登录返回 401"; else echo "[FAIL] 期望 401 实际 $cbad"; fail=$((fail+1)); fi

ST=$(api "http://127.0.0.1:${ADMIN_PORT}/api/data" | python3 -c '
import sys, json
d = json.load(sys.stdin)
for p in d["status"]["ports"]:
    if p["id"] == "'"${PORT_ID}"'" and p["conns_total"] >= 3 and p["bytes_in"] > 0:
        print("ok", p["conns_total"], p["bytes_in"], p["bytes_out"]); break
else:
    print("bad", json.dumps(d["status"]["ports"]))')
if echo "$ST" | grep -q '^ok'; then echo "[PASS] 端口统计累计: $ST"; else echo "[FAIL] 统计异常: $ST"; fail=$((fail+1)); fi

echo
if [ "$fail" -eq 0 ]; then
  echo "E2E 全部通过 ✔"
else
  echo "E2E 失败 $fail 项 ✘"
  docker logs fastproxy-e2e
  exit 1
fi
