#!/usr/bin/env bash
# Runs ON THE VPS: figure out what the imported VLESS nodes actually speak.
set -uo pipefail

test_node() {
  ip=$1
  port=$2
  sni=$3
  echo "=============================================================="
  echo "== $ip:$port  sni=$sni"
  if timeout 6 bash -c "cat < /dev/null > /dev/tcp/$ip/$port" 2>/dev/null; then
    echo "tcp: OPEN"
  else
    echo "tcp: BLOCKED"
    return
  fi
  echo "--- TLS certificate ---"
  echo | timeout 12 openssl s_client -connect "$ip:$port" -servername "$sni" 2>/dev/null |
    openssl x509 -noout -subject -issuer -dates 2>/dev/null || echo "no certificate / handshake failed"
  echo "--- TLS version/cipher ---"
  echo | timeout 12 openssl s_client -connect "$ip:$port" -servername "$sni" 2>/dev/null |
    grep -E "Protocol|Cipher" | head -3 || true
  echo "--- plain HTTP after TLS (a REALITY server would proxy it to the sni site) ---"
  printf 'GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: curl/8\r\nConnection: close\r\n\r\n' "$sni" |
    timeout 15 openssl s_client -quiet -connect "$ip:$port" -servername "$sni" 2>/dev/null |
    head -c 300 | tr -d '\r' | sed -n '1,6p'
  echo
}

test_node 2.27.109.138 18789 www.singaporeair.com
test_node 154.36.187.99 16767 www.apple.com
test_node 20.18.82.144 11443 www.apple.com
test_node 20.18.82.144 16666 www.htc.com
test_node 216.75.75.113 18964 www.apple.com
test_node 35.212.186.121 15446 www.apple.com
