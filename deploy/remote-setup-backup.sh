#!/usr/bin/env bash
# Runs ON THE VPS: install a daily SQLite backup (VACUUM-safe .backup) and run it once.
set -euo pipefail

cat > /opt/fastproxy/backup-db.sh <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
BK=/opt/fastproxy/backups
mkdir -p "$BK"
python3 - <<'PY'
import datetime, glob, os, sqlite3, sys
src = '/opt/fastproxy/data/fastproxy.db'
out = '/opt/fastproxy/backups/fastproxy-' + datetime.datetime.now().strftime('%Y%m%d-%H%M%S') + '.db'
src_con = sqlite3.connect(src, timeout=15)
dst_con = sqlite3.connect(out)
with dst_con:
    src_con.backup(dst_con)
dst_con.close()
src_con.close()
# verify the backup opens and has the expected tables
check = sqlite3.connect(out)
try:
    ok = check.execute('pragma quick_check').fetchone()[0]
    n = check.execute('select count(*) from upstreams').fetchone()[0]
finally:
    check.close()
if ok != 'ok':
    print('backup integrity failed:', ok, file=sys.stderr)
    sys.exit(1)
print('backup:', out, 'upstreams=', n)
files = sorted(glob.glob('/opt/fastproxy/backups/fastproxy-*.db'))
for old in files[:-14]:
    os.remove(old)
PY
EOS
chmod +x /opt/fastproxy/backup-db.sh

cat > /etc/cron.d/fastproxy-backup <<'EOS'
# Daily FastProxy SQLite backup (keeps 14 days on the same disk).
17 4 * * * root /opt/fastproxy/backup-db.sh >> /var/log/fastproxy-backup.log 2>&1
EOS
chmod 644 /etc/cron.d/fastproxy-backup

echo "== run once =="
/opt/fastproxy/backup-db.sh
echo
echo "== backups =="
ls -la /opt/fastproxy/backups

echo
echo "== current status =="
docker ps --filter name=fastproxy --format '{{.Status}}'
curl -s -o /dev/null -w 'panel: %{http_code}\n' http://127.0.0.1:7788/api/session
echo "dmesg I/O errors:"
dmesg -T 2>/dev/null | grep -iE 'i/o error|blk_update_request|ext4-fs error|xfs.*error|remount-ro' | tail -10 || echo "none"
