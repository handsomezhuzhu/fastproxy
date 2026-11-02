#!/usr/bin/env bash
# Runs on the instabox jump box: copy and run the disk diagnostic on the VPS.
set -uo pipefail
ASK=/tmp/ap-vps.sh
printf '#!/bin/sh\nprintf "%%s\\n" "AnK33qPjGmfJ"\n' > "$ASK"
chmod +x "$ASK"
export SSH_ASKPASS="$ASK" SSH_ASKPASS_REQUIRE=force DISPLAY=:0
scp -P 52438 -o StrictHostKeyChecking=no -o ConnectTimeout=20 \
  /tmp/remote-disk-diag.sh root@198.200.49.224:/tmp/remote-disk-diag.sh || exit 1
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 -p 52438 \
  root@198.200.49.224 "bash /tmp/remote-disk-diag.sh"
