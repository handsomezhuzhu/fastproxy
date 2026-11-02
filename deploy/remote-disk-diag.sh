#!/usr/bin/env bash
# Runs ON THE VPS: diagnose SQLite "disk I/O error (8714)".
set -uo pipefail

echo "== df =="
df -h
df -i
echo
echo "== mounts =="
findmnt -T /opt/fastproxy/data 2>&1 || true
echo
echo "== dmesg I/O errors =="
dmesg -T 2>/dev/null | grep -iE 'i/o error|blk_update_request|ext4-fs error|EXT4-fs \(.*\): error|nvme.*error|ata[0-9].*error|remount-ro' | tail -40 || echo "no dmesg errors"
echo
echo "== smart =="
if command -v smartctl >/dev/null; then
  smartctl -H /dev/vda 2>&1 | tail -5
  smartctl -A /dev/vda 2>&1 | grep -iE 'Reallocated|Pending|Uncorrect|CRC' | head -10
else
  echo "smartctl not installed"
fi
echo
echo "== data dir =="
ls -la /opt/fastproxy/data 2>&1
stat -f /opt/fastproxy/data 2>&1
echo
echo "== write test (64 MiB, fsync) =="
if dd if=/dev/zero of=/opt/fastproxy/data/.iotest bs=1M count=64 conv=fsync 2>&1 | tail -2; then
  sync
  rm -f /opt/fastproxy/data/.iotest
  echo "write test OK"
else
  echo "write test FAILED"
  rm -f /opt/fastproxy/data/.iotest 2>/dev/null || true
fi
echo
echo "== container =="
docker ps --filter name=fastproxy --format '{{.Status}}' 2>&1
docker logs --tail 200 fastproxy 2>&1 | grep -iE 'i/o|8714|sqlite|disk|error|错误' | tail -30 || true
echo
echo "== sqlite integrity (copy) =="
python3 - <<'PY' 2>&1
import glob, os, shutil, sqlite3, tempfile
src = '/opt/fastproxy/data/fastproxy.db'
if not os.path.exists(src):
    print('db not found:', src)
    raise SystemExit
tmp = tempfile.mkdtemp(prefix='fpcheck-')
dst = os.path.join(tmp, 'check.db')
for suffix in ('', '-wal', '-shm'):
    s = src + suffix
    if os.path.exists(s):
        shutil.copy2(s, dst + suffix)
        print('copied', s, os.path.getsize(s))
con = sqlite3.connect(dst)
try:
    print('quick_check:', con.execute('pragma quick_check').fetchall()[:5])
    print('journal_mode:', con.execute('pragma journal_mode').fetchall())
    print('page_count:', con.execute('pragma page_count').fetchall())
    print('freelist:', con.execute('pragma freelist_count').fetchall())
    print('tables:', con.execute("select name from sqlite_master where type='table'").fetchall())
finally:
    con.close()
    shutil.rmtree(tmp, ignore_errors=True)
PY
