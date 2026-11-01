#!/usr/bin/env bash
# Runs ON THE VPS: deploy FastProxy from the published GHCR image
# (no compilation on the server). Keeps the SQLite data directory.
set -euo pipefail

IMAGE="${GHCR_IMAGE:-ghcr.io/handsomezhuzhu/fastproxy:latest}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:?ADMIN_PASSWORD required}"
GHCR_USER="${GHCR_USER:-handsomezhuzhu}"
GHCR_TOKEN="${GHCR_TOKEN:-}"

echo "== 1. 检查 Docker =="
systemctl restart docker 2>/dev/null || true
ok=""
for i in $(seq 1 40); do
  if docker info >/dev/null 2>&1; then ok=1; break; fi
  sleep 2
done
if [ -z "$ok" ]; then
  echo "docker daemon 不可用，无法拉取镜像。请先在服务器上修复 Docker（journalctl -u docker -n 50）"
  exit 1
fi
echo "docker ok"

echo "== 2. 停止旧的 systemd 服务 =="
systemctl stop fastproxy 2>/dev/null || true
systemctl disable fastproxy 2>/dev/null || true

echo "== 3. 配置 =="
mkdir -p /opt/fastproxy
if [ -n "$GHCR_TOKEN" ]; then
  echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
fi
cat > /opt/fastproxy/docker-compose.yml <<EOF
services:
  fastproxy:
    image: ${IMAGE}
    container_name: fastproxy
    restart: unless-stopped
    environment:
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}
    ports:
      - "7788:7788"
      - "10001-10500:10001-10500"
    volumes:
      - /opt/fastproxy/data:/data
EOF

echo "== 4. 清理旧容器并拉取镜像 =="
old=$(docker ps -aq --filter "name=^/?fastproxy$" 2>/dev/null); if [ -n "${old:-}" ]; then docker kill $old 2>/dev/null || true; docker rm -f $old 2>/dev/null || true; fi; cd /opt/fastproxy && docker compose rm -fs fastproxy 2>/dev/null || true
cd /opt/fastproxy
docker compose pull
docker compose up -d
sleep 3
docker ps --filter name=fastproxy --format '{{.Status}}'
curl -s -o /dev/null -w "loopback panel: %{http_code}\n" http://127.0.0.1:7788/api/session
