#!/usr/bin/env bash
# Verbose SSH probe from the jump box (no grep, so resets are visible).
set -uo pipefail
ASK=/tmp/ap-v.sh
printf '#!/bin/sh\nprintf "%%s\\n" "%s"\n' "${VPS_PASS:?}" > "$ASK"
chmod +x "$ASK"
export SSH_ASKPASS="$ASK" SSH_ASKPASS_REQUIRE=force DISPLAY=:0
ssh -vv -o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 \
  -o NumberOfPasswordPrompts=1 -p "${VPS_PORT:-22}" "root@${VPS_HOST:?}" true 2>&1 | tail -40
