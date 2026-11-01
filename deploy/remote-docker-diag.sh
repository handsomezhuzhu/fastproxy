#!/usr/bin/env bash
# Runs ON THE VPS: diagnose why docker is not usable.
set -uo pipefail
echo "== system =="
uname -a
uptime
echo
echo "== disk =="
df -h /
df -i /
echo
echo "== memory =="
free -m
echo
echo "== docker service =="
systemctl is-enabled docker 2>&1 || true
systemctl is-active docker 2>&1 || true
systemctl status docker --no-pager -l 2>&1 | head -30
echo
echo "== journalctl docker (last 80) =="
journalctl -u docker -n 80 --no-pager 2>&1 | tail -80
echo
echo "== socket / processes =="
ls -la /var/run/docker.sock 2>&1 || true
ps -eo pid,etime,comm,args | grep -E 'docker|containerd' | grep -v grep || true
echo
echo "== docker info after start attempt =="
systemctl start docker 2>&1 || true
sleep 5
docker info 2>&1 | head -25 || true
echo
echo "== docker images =="
docker images 2>&1 | head -15 || true
echo
echo "== containers =="
docker ps -a 2>&1 | head -15 || true
