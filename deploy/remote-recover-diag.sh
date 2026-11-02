#!/usr/bin/env bash
# Runs ON THE VPS after reboot: assess disk/DB health and back up the DB
# before any recovery attempt. Does not modify the database.
set -uo pipefail

echo "== uptime / boot =="
uptime
who -b 2>/dev/null || true
echo
echo "== disk =="
df -h
df -i
findmnt -T /opt/fastproxy/data 2>&1 || true
echo
echo "== dmesg I/O =="
dmesg -T 2>/dev/null | grep -iE 'i/o error|blk_update_request|ext4-fs error|EXT4-fs \(.*\): error|nvme.*error|ata[0-9].*error|remount-ro|recovery complete' | tail -60 || echo "no matches"
echo
echo "== docker =="
systemctl is-active docker
docker ps -a --format '{{.Names}} | {{.Status}} | {{.Image}}' 2>&1 | head -10
echo
echo "== data files =="
ls -la /opt/fastproxy/data 2>&1
for f in /opt/fastproxy/data/fastproxy.db*; do [ -e "$f" ] && echo "$f $(stat -c '%s bytes, mtime %y' "$f" 2>&1)"; done
echo
echo "== container logs =="
docker logs --tail 80 fastproxy 2>&1 | tail -80 || true
echo
echo "== DB backup (copy) =="
STAMP=$(date +%Y%m%d-%H%M%S)
BK="/opt/fastproxy/data-backup-$STAMP"
mkdir -p "$BK"
if cp -a /opt/fastproxy/data/. "$BK"/ 2>&1; then
  echo "backup: $BK"
  ls -la "$BK"
else
  echo "backup FAILED"
fi
echo
echo "== integrity check on the copy =="
python3 - "$BK/fastproxy.db" <<'PY' 2>&1
import os, sqlite3, sys
path = sys.argv[1]
if not os.path.exists(path):
    print('db not found:', path); raise SystemExit
con = sqlite3.connect(path)
try:
    rows = con.execute('pragma integrity_check').fetchall()
    print('integrity_check (first 10):')
    for r in rows[:10]:
        print(' ', r[0])
    print('quick_check:', con.execute('pragma quick_check').fetchall()[:5])
    print('journal_mode:', con.execute('pragma journal_mode').fetchall())
    for t in ('upstreams','ports','accounts','logs','settings'):
        try:
            n = con.execute(f'select count(*) from {t}').fetchone()[0]
            print(f'{t}: {n}')
        except Exception as e:
            print(f'{t}: ERROR {e}')
except Exception as e:
    print('integrity check failed:', e)
finally:
    con.close()
PY
