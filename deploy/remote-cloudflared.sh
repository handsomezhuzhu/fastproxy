#!/usr/bin/env bash
# Executed ON THE VPS: install cloudflared and expose the panel over a quick
# HTTPS tunnel (no domain needed).
set -euo pipefail

if ! command -v cloudflared >/dev/null 2>&1; then
  curl -fsSL -o /usr/local/bin/cloudflared \
    https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64
  chmod +x /usr/local/bin/cloudflared
fi

cat > /etc/systemd/system/cloudflared-fastproxy.service <<'UNIT'
[Unit]
Description=Cloudflare quick tunnel for FastProxy panel
After=network-online.target fastproxy.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/cloudflared tunnel --no-autoupdate --url http://127.0.0.1:7788
Restart=always
RestartSec=5
StandardOutput=append:/var/log/cloudflared-fastproxy.log
StandardError=append:/var/log/cloudflared-fastproxy.log

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now cloudflared-fastproxy
echo "waiting for tunnel URL..."
for i in $(seq 1 20); do
  url=$(grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /var/log/cloudflared-fastproxy.log 2>/dev/null | tail -1 || true)
  if [ -n "${url:-}" ]; then
    echo "TUNNEL_URL=$url"
    curl -s -o /dev/null -w "tunnel check: %{http_code}\n" --max-time 20 "$url/api/session" || true
    exit 0
  fi
  sleep 2
done
echo "no tunnel URL yet; log tail:"
tail -20 /var/log/cloudflared-fastproxy.log || true
exit 1
