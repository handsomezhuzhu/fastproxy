#!/usr/bin/env bash
# Executed on a throwaway cloud box (reachable from the VPS): copies the
# setup script to the VPS and runs it, printing the tunnel URL.
set -euo pipefail

: "${VPS_HOST:?VPS_HOST required}"
: "${VPS_PORT:=22}"
: "${VPS_PASS:?VPS_PASS required}"

ASK=/tmp/askpass-vps.sh
cat > "$ASK" <<'EOF'
#!/bin/sh
printf '%s\n' "${VPS_PASS}"
EOF
chmod +x "$ASK"
export SSH_ASKPASS="$ASK" SSH_ASKPASS_REQUIRE=force DISPLAY=:0
export VPS_PASS

SSH="ssh -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 -p $VPS_PORT root@$VPS_HOST"

scp -P "$VPS_PORT" -o StrictHostKeyChecking=accept-new deploy/remote-cloudflared.sh "root@$VPS_HOST:/tmp/remote-cloudflared.sh"
$SSH 'bash /tmp/remote-cloudflared.sh'
