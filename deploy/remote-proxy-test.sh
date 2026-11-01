#!/usr/bin/env bash
# Runs ON THE VPS: check whether the user's :3129 proxy endpoints are usable
# from THIS server (the only network that matters for FastProxy).
set -uo pipefail

U='mkxjkaewtj82'
P='91ahto8mhjq6eez'
IPS='209.50.178.45 209.50.181.15 104.207.46.59 45.3.52.11'

echo "== server egress IP =="
curl -s -m 10 https://api.ipify.org || echo "egress check failed"
echo

echo "== outbound port sanity (portquiz.net) =="
for p in 80 443 3129 8080; do
  code=$(curl -s -m 10 -o /dev/null -w '%{http_code}' "http://portquiz.net:$p")
  echo "portquiz.net:$p -> $code"
done
echo

echo "== tcp connect to proxy :3129 (python) =="
python3 - "$IPS" <<'PY'
import socket, sys
for ip in sys.argv[1].split():
    s = socket.socket(); s.settimeout(6)
    try:
        s.connect((ip, 3129)); print(f"{ip}:3129 OPEN")
    except Exception as e:
        print(f"{ip}:3129 FAIL: {e}")
    finally:
        s.close()
PY
echo

echo "== real HTTP proxy request (with credentials) =="
for ip in $IPS; do
  echo "-- $ip --"
  curl -sS -m 15 -x "http://$U:$P@$ip:3129" -o /tmp/proxy-out.txt \
    -w 'http status=%{http_code}\n' http://api.ipify.org 2>&1 | tail -2
  echo "body: $(head -c 120 /tmp/proxy-out.txt 2>/dev/null)"
done
echo

echo "== real HTTPS CONNECT request (with credentials, first ip) =="
ip=$(echo $IPS | awk '{print $1}')
curl -sS -m 15 -x "http://$U:$P@$ip:3129" -o /tmp/proxy-out2.txt \
  -w 'https status=%{http_code}\n' https://api.ipify.org 2>&1 | tail -2
echo "body: $(head -c 120 /tmp/proxy-out2.txt 2>/dev/null)"
