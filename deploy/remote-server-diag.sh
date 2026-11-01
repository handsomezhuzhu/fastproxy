#!/usr/bin/env bash
# Runs ON THE VPS: inspect what survived (data, unit, ports) and whether we can
# install docker.
set -uo pipefail
echo "== os =="
cat /etc/os-release | head -4
echo
echo "== /opt =="
ls -la /opt 2>&1
ls -la /opt/fastproxy 2>&1 || true
ls -la /opt/fastproxy/data 2>&1 || true
echo
echo "== fastproxy unit / binary =="
systemctl list-unit-files 2>/dev/null | grep -i fastproxy || echo "no fastproxy unit"
ls -la /usr/local/bin/fastproxy /root/fastproxy 2>&1 || true
echo
echo "== listening ports =="
ss -lntp 2>&1 | head -20
echo
echo "== egress =="
curl -s -m 10 https://api.ipify.org || echo "api.ipify.org failed"
echo
curl -s -m 10 -o /dev/null -w "get.docker.com: %{http_code}\n" https://get.docker.com || true
curl -s -m 10 -o /dev/null -w "download.docker.com: %{http_code}\n" https://download.docker.com/linux/ubuntu/ || true
echo
echo "== tools =="
which curl wget tar python3 apt-get 2>&1
echo
echo "== apt docker candidates =="
apt-cache policy docker.io 2>/dev/null | head -8
echo
echo "== home =="
ls -la /root 2>&1 | head -20
