#!/usr/bin/env bash
# Runs on the instabox jump box (China can't reach the VPS): deploy GHCR image
# to the VPS over SSH, then verify via the public panel API.
set -euo pipefail

: "${VPS_HOST:?}"
: "${VPS_PORT:=22}"
: "${VPS_PASS:?}"
: "${ADMIN_PASSWORD:?}"

ASK=/tmp/askpass-vps.sh
cat > "$ASK" <<EOF
#!/bin/sh
printf '%s\n' "\$VPS_PASS"
EOF
chmod +x "$ASK"
export SSH_ASKPASS="$ASK" SSH_ASKPASS_REQUIRE=force DISPLAY=:0

SSH="ssh -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 -p $VPS_PORT root@$VPS_HOST"

scp -P "$VPS_PORT" -o StrictHostKeyChecking=accept-new \
  deploy/remote-ghcr-deploy.sh "root@$VPS_HOST:/tmp/remote-ghcr-deploy.sh"

$SSH "ADMIN_PASSWORD='$ADMIN_PASSWORD' GHCR_TOKEN='${GHCR_TOKEN:-}' GHCR_USER='${GHCR_USER:-handsomezhuzhu}' bash /tmp/remote-ghcr-deploy.sh"
