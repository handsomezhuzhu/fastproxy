#!/usr/bin/env bash
# Debug SSH auth from the jump box to the VPS (verbose, single attempt).
set -euo pipefail
: "${VPS_HOST:?}"
: "${VPS_PORT:=22}"
: "${VPS_PASS:?}"

ASK=/tmp/ap-debug.sh
printf '#!/bin/sh\nprintf "%%s\\n" "%s"\n' "$VPS_PASS" > "$ASK"
chmod +x "$ASK"
export SSH_ASKPASS="$ASK" SSH_ASKPASS_REQUIRE=force DISPLAY=:0

ssh -vv -o StrictHostKeyChecking=accept-new -o ConnectTimeout=12 \
  -o NumberOfPasswordPrompts=1 -p "$VPS_PORT" "root@$VPS_HOST" true 2>&1 |
  grep -E "Remote protocol|Authentications that can continue|Permission denied|Authenticated|Server host key|kex: algorithm|password" | head -15
