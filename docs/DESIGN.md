# FastProxy 设计文档

## 定位

把入口端口稳定转发到固定上游 HTTP/SOCKS5 代理的轻量中转：

- **快转发**：连接路径极短（单跳 relay），无订阅调度与探测开销
- **专业配置**：上游池 / 端口线路 / 账户 三层模型，字段显式、可绑定、可覆盖
- **完备记录**：每次连接一条结构化日志，落库可查询、可筛选、按天保留

## 数据模型（每端口自带默认账号，账户是唯一分流键）

```
Upstream（上游代理池）
  id, name, type(http|socks5|shadowsocks|vless|vmess), host,
  username, password,          # http/socks5 账号密码；ss 用 password
  method,                      # ss 加密方式
  uuid, sni,                   # vless/vmess 用户 ID 与 TLS SNI
  security, pbk, sid, flow,    # TLS 单一来源 none|tls|reality；REALITY 公钥/短 ID、XTLS Vision
  fingerprint, transport,      # uTLS 指纹、tcp/ws/grpc
  path, host_header, service_name,
  vmess_security,              # vmess auto/none/zero/aes-128-gcm/...
  remark

Port（纯入口端口，不再绑定上游）
  id, port, enabled, remark
        └── 自动生成 1 个默认账号（is_default=1）

Account（账户，必须绑定一个端口）
  id, username(全局唯一), password,
  port_id(必选), upstream_id(可空=直连),
  enabled, remark, is_default
```

认证与路由（一行读懂）：

```
客户端 → 端口（0x05 首字节=SOCKS5，否则 HTTP）
      → 账户（用户名+密码，且 port_id 必须等于本端口；跨端口一律 407）
      → 上游 = 账户绑定的上游（可空 = 直连）
```

约束（新模型，无全局账户概念）：

- 每个端口自动带一名默认账号（用户名 `default-<port>`，随机密码），初始直连
- 默认账号可改用户名/密码/上游/备注，**不可删除、不可更换绑定端口**
- 删除端口时连带删除其默认账号；**若还有普通账号绑定则拒绝删除**（"请先删除绑定账户 N 个"）
- `is_default` 标识用于面板展示与删除保护

## 存储（SQLite）

单文件数据库（纯 Go 驱动、WAL 日志模式、事务与索引），默认 `/data/fastproxy.db`：

| 表 | 说明 |
|---|---|
| `settings(key, value)` | 管理密码哈希、会话密钥、JSON 系统设置 |
| `upstreams` | 上游代理池（密码 AES-GCM 加密存储） |
| `ports` | 端口线路（port 唯一） |
| `accounts` | 账户（username 唯一；密码 AES-GCM 加密存储） |
| `logs` | 连接日志（ts/port/account/level 索引） |
| `subscriptions` | 订阅源（URL 加密、刷新间隔、ETag/Last-Modified、最近状态） |
| `subscription_nodes` | 订阅 → 上游映射与节点去重键（刷新 diff 用） |
| `schema_migrations` | 已应用的数据库迁移版本 |

- 启动时按 `schema_migrations` **版本化迁移**（事务执行、幂等；旧库自动补齐列并记录基线）
- 若同目录存在旧版 `config.json`，首次启动**自动迁移**并重命名为 `config.json.migrated`
- 备份 = 复制数据库文件（或 `sqlite3 fastproxy.db ".backup out.db"`）；**密码加密时还需备份密钥**
- 日志异步批量写入，不阻塞转发路径；按 `log_retention_days`（默认 7 天）定时清理

## 启动级配置

| 环境变量 | 说明 |
|---|---|
| `PORT_RANGE` | 允许创建的入站端口范围，默认 `10001-10050`；Docker 映射必须与之一致 |
| `FASTPROXY_SECRET_KEY` | 数据密码加密密钥（32 字节 base64/hex）；留空生成 `/data/.secret_key` |
| `LOGIN_MAX_ATTEMPTS` / `LOGIN_LOCK_DURATION` | 面板登录全局失败锁定（仅内存，重启清零） |
| `TRUSTED_PROXIES` | 可信反向代理 CIDR；只有这些来源的 `X-Forwarded-For/Proto` 被采信 |
| `SESSION_SECRET` | 可选固定会话签名密钥 |

## 热加载

- 账户 / 上游 / 系统设置：**不重启监听**，下一次连接即生效（认证、上游解析实时读库）
- 端口定义变更（端口号、默认上游）：仅重启该端口监听
- 日志级别 / 保留天数：保存后立即应用；直连 = 账户不绑定上游，没有全局开关

## 敏感信息

- 上游账号/密码、账户密码支持 `${VAR}` / `${VAR:-默认值}` 环境变量引用
- 所有密码字段在 SQLite 中以 **AES-256-GCM** 密文保存（`enc:v1:` 前缀），读取时解密；
  密钥来自 `FASTPROXY_SECRET_KEY`，未设置时生成数据目录下 `.secret_key`（0600）
- 备份数据库时必须同时备份密钥；密钥丢失则密码不可恢复
- 管理密码 PBKDF2-SHA256 哈希存储；登录失败为**全局**内存锁定（单管理员，重启清零）

## 日志

- 每次连接一条：`time/level/port/account/client/proto/target/status/bytes_in/bytes_out/duration_ms/msg`
- 状态：`ok`、`auth-failed`、`dial-error`、`upstream-error`、`bad-request`
- 访问日志可关闭（关闭后仅记录失败连接）；系统事件（启停/导入/登录失败）同样入库

## 上游导入

`POST /api/upstreams/import`，输入 `{url}` 或 `{text}`，解析：
| 格式 | 示例 |
|---|---|
| URI 行 | `http://user:pass@1.2.3.4:8080#名称`、`socks5://...` |
| Shadowsocks URI | `ss://BASE64(method:pass)@host:port#名称`（SIP002）、`ss://method:pass@host:port`（百分号编码）、`ss://BASE64(method:pass@host:port)`（旧版整体 base64） |
| VLESS URI | `vless://uuid@host:port?encryption=none&security=tls&type=tcp&sni=xx#名称` |
| VMess URI | `vmess://BASE64(JSON)`，JSON 字段 `v/ps/add/port/id/scy/net/tls/sni/host/path` |
| 纯文本行 | `1.2.3.4:8080`、`1.2.3.4:8080:user:pass`（默认 http） |
| base64 订阅 | 整段 base64 编码的文本订阅 |
| sing-box JSON | `{"outbounds":[{type:"http",server,server_port,username,password,tag}]}` |
| Clash JSON/YAML | `proxies:` 列表，流式与块式均支持（`type: ss/vmess/vless` 字段 `cipher/password/uuid/servername/sni/tls/network`） |

- 支持 http/socks5/shadowsocks/vless（含 REALITY / Vision / WS / gRPC）/vmess 节点；以下情况**跳过并给出 problem**：
  非 tcp/ws/grpc 传输（`h2`/`quic`）、VLESS `encryption` 非 `none`、
  非 AEAD 的 Shadowsocks 加密方式（`rc4-md5`、`*-cfb`、`2022-*`）。
  REALITY 必须带 32 字节的 `pbk`（`sid` 可空），入库前会校验格式
- 去重键：`type|host|method|uuid|username|password|pbk|sid|flow|transport|path|host_header|service_name`，
  与现有上游及同批次节点比较
- 返回 `{added, total, duplicates[], problems[], warning?}`，面板展示明细；`ImportUpstreams` 入库前再校验一次，
  不合格节点以 problem 返回而不入库

## 订阅刷新（Resin 风格）

- `subscriptions` 保存 URL（AES-GCM 加密）、刷新间隔（5-1440 分钟）、启用状态、ETag/Last-Modified 与最近状态
- 后台每 30 秒扫描到期订阅（最多 2 个并发），手动刷新走 `POST /api/subscriptions/{id}/refresh`
- 拉取：仅 http(s)，默认拒绝内网/回环/链路本地地址（`SUB_ALLOW_PRIVATE=1` 可放开给本地转换器），
  4 MiB 上限、25 秒超时、最多 3 次重定向；支持 304
- 解析复用导入器；节点以去重键映射到上游：存在则原地更新，新增则插入，消失则删除映射并删除未被账户引用的上游
- 手动上游不受影响；被账户引用的旧节点会保留（只删映射），避免断线
- 刷新后若节点有变化，重建内置 sing-box；失败写入 `last_error`，不会保留半套配置

## 上游出站协议

所有出站统一走 `proxy.DialTunnel(up, addr, timeout)`，入站 CONNECT / SOCKS5 / 明文 HTTP / 健康探测共用，
因此新增协议自动对全链路（含探测）生效。

| 协议 | 传输 | 说明 | 状态 |
|---|---|---|---|
| http | CONNECT | 明文 HTTP 请求走标准 Transport | 支持 |
| socks5 | TCP | 用户名/密码认证可选 | 支持 |
| shadowsocks | AEAD TCP | `aes-128-gcm` / `aes-256-gcm` / `chacha20-ietf-poly1305`，`go-shadowsocks2/core` 实现；不支持 UDP 与 `2022-*` | 支持 |
| vless | TCP / WS / gRPC（可选 TLS/REALITY） | **全部经内置 sing-box**：每个上游在 `127.0.0.1:20000+` 起一个 SOCKS 入口，`DialTunnel` 经本地 SOCKS 转发；支持 REALITY、`flow=xtls-rprx-vision`、WS、gRPC | 支持 |
| vmess | TCP / WS / gRPC（可选 TLS） | 内置 sing-box 出站（VMess AEAD），支持 `security=auto/none/zero/aes-128-gcm/chacha20-poly1305` | 支持 |

内置 sing-box 只在存在 VLESS / VMess 上游时启动；http/socks5/ss 不依赖它，sing-box 启动失败也不影响这些协议。
REALITY / uTLS 需要 `-tags with_utls` 构建（Dockerfile、CI、`make build-utls` 已带）。上游增删改后服务端会重建内置实例，
面板的 `GET /api/singbox/status` 展示运行状态、已注册上游与最近事件。

## 探测与出口 IP

- **主动探测**：默认每 300 秒（可在系统设置调整 30-3600 秒）并发（`probe_concurrency`，1-32，默认 6）检测全部上游：
  通过上游隧道请求探测目标（默认 `api.ipify.org`，先 :80 再 :443/TLS），
  记录健康状态（ok/fail）、握手延迟、响应中的**出口 IP**、失败原因与检测时间。
- **本机出口 IP**：同一轮探测中直连探测一次，控制台仪表盘展示。
- **被动标记**：真实转发发生 `dial-error`/`upstream-error` 时，按上游 30 秒节流写入失败状态，
  与主动探测共同维护健康视图。
- 凭证/地址变更会自动重置该上游的探测状态；探测结果存于 `upstreams` 表相关列，
  接口见 `docs/API.md`。

## 管理面

- SPA（React + Tailwind + shadcn 官方组件），Go 二进制内嵌
- 会话：HMAC 签名 Cookie，12 小时有效；修改密码即轮换密钥
- API 见 `docs/API.md`

## 构建流程

```
web (React + Tailwind v4 + shadcn/Base UI) --vite build--> admin/dist
                                                          │ go:embed
go build  ───────────────────────────────────────────────┘
   └── 单二进制（前端内嵌）+ Docker（node → go → alpine）
```

`make web && make build`；`make test`；`make e2e`（容器端到端）。
