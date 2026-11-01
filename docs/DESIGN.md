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
  uuid, tls, sni,              # vless/vmess 用户 ID 与 TLS 参数
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
| `upstreams` | 上游代理池 |
| `ports` | 端口线路（port 唯一） |
| `accounts` | 账户（username 唯一） |
| `logs` | 连接日志（ts/port/account/level 索引） |

- 启动时自动建表；若同目录存在旧版 `config.json`，首次启动**自动迁移**并重命名为 `config.json.migrated`
- 备份 = 复制数据库文件（或 `sqlite3 fastproxy.db ".backup out.db"`）
- 日志异步批量写入，不阻塞转发路径；按 `log_retention_days`（默认 7 天）定时清理

## 热加载

- 账户 / 上游 / 系统设置：**不重启监听**，下一次连接即生效（认证、上游解析实时读库）
- 端口定义变更（端口号、默认上游）：仅重启该端口监听
- 日志级别 / 保留天数 / 直连策略：保存后立即应用

## 敏感信息

- 上游账号/密码、账户密码支持 `${VAR}` / `${VAR:-默认值}` 环境变量引用
- 数据库中只保存引用，**明文密码不落盘**；启动与保存时严格校验（缺失变量 → 拒绝）
- 管理密码 PBKDF2-SHA256 哈希存储，登录失败按 IP 限速

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

- 支持 http/socks5/shadowsocks/vless/vmess 节点；以下情况**跳过并给出 problem**：
  非 tcp 传输（`ws`/`grpc`/`h2`/`quic`）、VLESS `encryption` 非 `none`、
  非 AEAD 的 Shadowsocks 加密方式（`rc4-md5`、`*-cfb`、`2022-*`）、
  VLESS 的 REALITY（需要 pbk/sid 与 uTLS 握手）与 `flow=xtls-rprx-vision`（前者会伪装成真实
  SNI 网站、握手后无响应，因此直接拒绝而不是存成永远失败的上游）
- 去重键：`type|host|method|uuid|username|password`，与现有上游及同批次节点比较
- 返回 `{added, total, duplicates[], problems[]}`，面板展示明细；`ImportUpstreams` 入库前再校验一次，
  不合格节点以 problem 返回而不入库

## 上游出站协议

所有出站统一走 `proxy.DialTunnel(up, addr, timeout)`，入站 CONNECT / SOCKS5 / 明文 HTTP / 健康探测共用，
因此新增协议自动对全链路（含探测）生效。

| 协议 | 传输 | 说明 | 状态 |
|---|---|---|---|
| http | CONNECT | 明文 HTTP 请求走标准 Transport | 支持 |
| socks5 | TCP | 用户名/密码认证可选 | 支持 |
| shadowsocks | AEAD TCP | `aes-128-gcm` / `aes-256-gcm` / `chacha20-ietf-poly1305`，`go-shadowsocks2/core` 实现；不支持 UDP 与 `2022-*` | 支持 |
| vless | TCP（可选 TLS） | 标准请求头，uuid + addonsLen + cmd=TCP + port + ATYP 地址；TLS 严格校验系统根证书，SNI 取 `sni`，为空则取地址 hostname（IP 不发送 SNI） | 支持（不支持 ws/grpc、Reality、flow） |
| vmess | — | 无小巧可靠的纯 Go 客户端可引（现代服务端仅接受 VMess AEAD，引入 xray-core/sing-box 等于嵌入整套代理引擎），因此只保存配置，隧道返回明确错误 | **未支持** |

VMess 的配置可以正常保存与导入，健康探测会把 `vmess 暂未支持` 记为失败原因，不会伪装成可用。

## 探测与出口 IP

- **主动探测**：默认每 300 秒（可在系统设置调整 30-3600 秒）并发（≤4）检测全部上游：
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
