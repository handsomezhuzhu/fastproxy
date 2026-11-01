#!/usr/bin/env bash
# Generic jump helper: copy a local script to the VPS and run it over SSH.
# Usage (env): VPS_HOST, VPS_PORT (default 22), VPS_PASS. First arg = local script.
set -euo pipefail

SCRIPT="${1:?usage: jump-run.sh <local-script>}"
: "${VPS_HOST:?}"
: "${VPS_PORT:=22}"
: "${VPS_PASS:?}"

ASK=/tmp/askpass-vps.sh
printf '#!/bin/sh\nprintf "%%s\\n" "%s"\n' "$VPS_PASS" > "$ASK"
chmod +x "$ASK"
export SSH_ASKPASS="$ASK" SSH_ASKPASS_REQUIRE=force DISPLAY=:0

BASE=$(basename "$SCRIPT")
scp -P "$VPS_PORT" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 \
  "$SCRIPT" "root@$VPS_HOST:/tmp/$BASE"
ssh -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 -p "$VPS_PORT" \
  "root@$VPS_HOST" "bash /tmp/$BASE"
