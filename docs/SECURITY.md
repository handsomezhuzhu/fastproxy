# 安全与密钥管理

## 管理面

- 单管理员，管理密码以 **PBKDF2-SHA256** 哈希保存在 `settings` 表，不存明文
- 会话 Cookie：`HttpOnly` + `SameSite=Lax`；HTTPS（直连 TLS 或可信反代 `X-Forwarded-Proto: https`）时加 `Secure`
- 登录失败**全局**计数，默认连续 5 次锁定 5 分钟（`LOGIN_MAX_ATTEMPTS` / `LOGIN_LOCK_DURATION`）
  - 只存在内存，容器重启清零
  - 返回 `429` + `Retry-After`，不区分来源 IP
- 建议用 Nginx/OpenResty 终止 TLS，参考 [REVERSE_PROXY.md](REVERSE_PROXY.md)

## 数据密码加密

`upstreams.password`、`accounts.password`（以及后续订阅 URL 中的 token）在 SQLite 中以
AES-256-GCM 保存，密文前缀 `enc:v1:`。

密钥来源（按优先级）：

1. `FASTPROXY_SECRET_KEY`：32 字节密钥的 base64（std/url）或 64 位 hex
2. 数据目录 `.secret_key`：文件不存在时自动生成，权限 `0600`

```bash
# 生成一个密钥并写入 .env
python3 -c 'import os,base64; print(base64.urlsafe_b64encode(os.urandom(32)).decode())'
```

**重要：**

- 备份数据库时**必须同时备份 `.secret_key` 或 `FASTPROXY_SECRET_KEY`**
- 密钥丢失 = 上游/账户密码无法解密，服务无法使用这些凭据
- 更换密钥需要重新录入所有密码；本项目不会尝试自动迁移旧密钥
- `/data/.secret_key` 与 `/data/fastproxy.db` 同目录，整盘故障会一起丢失；请做异地备份或使用环境变量 + 密钥管理

## 备份

```bash
# 在线安全备份（SQLite backup API）
python3 - <<'PY'
import sqlite3
src = sqlite3.connect('/data/fastproxy.db')
dst = sqlite3.connect('/backup/fastproxy.db')
with dst:
    src.backup(dst)
dst.close(); src.close()
PY
```

同时保存 `.secret_key`（或环境变量）。定期做恢复演练，确认备份能打开且
`PRAGMA integrity_check` 为 `ok`。

## 反向代理信任

- 只有 `TRUSTED_PROXIES`（CIDR 列表）中的来源，`X-Forwarded-For` / `X-Forwarded-Proto` 才会被采信
- 不在列表中的来源伪造这些头会被忽略
- 同机 Nginx：`TRUSTED_PROXIES=127.0.0.1/32`
- Docker 网络中的反代容器：填其子网，例如 `172.18.0.0/16`
- 不要把面板 7788 直接暴露到公网；只暴露反代的 443

## 上游与出站

- VLESS/VMess 经内置 sing-box；REALITY 需要 `pbk`/`sid`，格式在入库前校验
- HTTP/SOCKS5/Shadowsocks 原生出站；TLS 证书严格校验（无 `InsecureSkipVerify`）
- 订阅 URL 拉取/节点导入的 SSRF 防护、大小/超时限制将在订阅功能中统一实现
  （当前 `POST /api/upstreams/import` 的 `url` 只应由管理员使用）

## 依赖与发布

- 生产镜像/二进制使用 `-tags with_utls` 构建
- Docker 镜像按版本号固定拉取（`1.x.y-rc.n` / `v1.x.y-rc.n`），不要长期依赖 `latest`
- 升级前先备份数据库与密钥，并保留上一版本镜像以便回滚
