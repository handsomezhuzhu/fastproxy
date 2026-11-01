#!/usr/bin/env bash
# Runs ON THE VPS: install docker engine + compose v2 from Ubuntu repos.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

echo "== apt update =="
apt-get update -qq

echo "== install docker.io + docker-compose-v2 =="
apt-get install -y -qq docker.io docker-compose-v2

echo "== enable docker =="
systemctl enable --now docker
systemctl is-active docker

echo "== versions =="
docker --version
docker compose version
docker info --format 'server={{.ServerVersion}} driver={{.Driver}}'

echo "== disk after install =="
df -h / | tail -1
