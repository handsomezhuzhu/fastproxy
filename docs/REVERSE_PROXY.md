# Nginx / OpenResty 反向代理

FastProxy 面板本身只监听 HTTP。推荐由 Nginx/OpenResty 终止 TLS，FastProxy 只绑定本机：

```bash
# docker-compose.yml 或 docker run
ADMIN_ADDR=127.0.0.1:7788
TRUSTED_PROXIES=127.0.0.1/32
```

> 如果反代在另一个容器里，`TRUSTED_PROXIES` 填该容器的 Docker 子网（如 `172.18.0.0/16`），
> 并把 FastProxy 端口只发布到该网络，不要发布到公网。

## Nginx

```nginx
upstream fastproxy_panel {
    server 127.0.0.1:7788;
    keepalive 16;
}

server {
    listen 443 ssl http2;
    server_name fp.example.com;

    ssl_certificate     /etc/letsencrypt/live/fp.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/fp.example.com/privkey.pem;

    # 面板是单页应用，静态资源很小；请求体只有 JSON
    client_max_body_size 2m;

    location / {
        proxy_pass http://fastproxy_panel;
        proxy_http_version 1.1;

        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;

        # WebSocket / 未来 SSE 实时日志
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_buffering off;
        proxy_cache off;

        proxy_connect_timeout 10s;
        proxy_send_timeout    60s;
        proxy_read_timeout    60s;
    }

    # 可选：只允许自己的 IP 访问面板
    # allow 203.0.113.0/24;
    # deny all;
}

map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    server_name fp.example.com;
    return 301 https://$host$request_uri;
}
```

## OpenResty

与 Nginx 相同；若使用 `lua-resty` 做鉴权/WAF，把 `X-Forwarded-*` 头补齐即可。
OpenResty 默认的 `proxy_buffering on` 会让未来的 SSE 实时日志不实时，务必在面板 location 关闭。

## 安全要点

- `TRUSTED_PROXIES` 只填真实反代地址；否则客户端可以伪造 `X-Forwarded-For`
- 面板 7788 不应对公网开放（`docker run -p 127.0.0.1:7788:7788` 或仅 Docker 内网）
- HTTPS 下 FastProxy 会自动给会话 Cookie 加 `Secure`（依赖可信代理的 `X-Forwarded-Proto`）
- 若使用 Cloudflare 等 CDN，再在其后加一层反代时，把 CDN 回源地址加入 `TRUSTED_PROXIES`，
  并确保最右侧的 `X-Forwarded-For` 是真实客户端
- 代理业务端口（`PORT_RANGE`）不要经过面板反代；它们由 Docker 直接映射

## 验证

```bash
curl -sI https://fp.example.com/api/session
# 登录后检查 Set-Cookie 是否带 Secure / HttpOnly / SameSite=Lax
```

登录连续失败达到上限会返回 `429` + `Retry-After`；这是应用级全局限流，重启容器清零。
