<div align="center">

# FastProxy

**高性能多端口 HTTP / SOCKS5 认证网关与多协议代理分流中枢**

基于 Go 与内嵌 `sing-box` 构建的轻量级代理路由网关。支持**单端口双协议自动识别**、**端口 × 账户 × 上游三层硬边界路由**、**VLESS REALITY / VMess / Shadowsocks / SOCKS5 / HTTP 全协议出站**、**机场订阅定时增量同步**与**主动健康探测**，内置基于 React 19 + Tailwind CSS v4 + shadcn/ui 构建的现代化控制台。

[![CI](https://github.com/handsomezhuzhu/fastproxy/actions/workflows/ci.yml/badge.svg)](https://github.com/handsomezhuzhu/fastproxy/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fhandsomezhuzhu%2Ffastproxy-blue?logo=docker)](https://github.com/handsomezhuzhu/fastproxy/pkgs/container/fastproxy)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![UI](https://img.shields.io/badge/UI-shadcn%2Fui%20%2B%20Tailwind%20v4-09090b?logo=react)](https://ui.shadcn.com/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)

</div>

---

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./docs/assets/dashboard-dark.png" />
    <source media="(prefers-color-scheme: light)" srcset="./docs/assets/dashboard.png" />
    <img alt="FastProxy Console Dashboard" src="./docs/assets/dashboard.png" width="100%" />
  </picture>
</p>

---

## 核心特性

### 1. 端口 × 账户 × 上游三层路由架构
- **双协议自适应入站**：同一监听端口同时承载 `HTTP`（明文代理 + `CONNECT` 隧道）与 `SOCKS5` 协议，首字节自动嗅探识别，全程强制账密鉴权。
- **按账户精确分流**：每个端口僅作为网络入口并自动生成专属默认账号；同一端口下可创建任意数量的业务账户，**不同账户可独立绑定不同上游节点或直连出站**——客户端向哪个账号认证，流量即按该账号绑定的上游转发。
- **零中断热加载**：新增或切换账户绑定、修改上游节点、更新超时与日志策略均**毫秒级热生效**，不中断任何存量长连接；仅在增删端口或启停端口时按需操作对应监听器。

```
                     ┌──────────────────────┐
  HTTP / CONNECT ───►│                      ├─── auth: default-10001 ───► HK · BGP HTTP
                     │   Inbound :10001     │
  SOCKS5         ───►│  (Auto Protocol Mux) ├─── auth: crawler-us    ───► US · VLESS REALITY
                     │                      │
                     └──────────────────────┘─── auth: ops-direct    ───► Direct (直连出站)
```

### 2. 原生出站 + 内嵌 `sing-box` 多协议引擎
| 出站协议 | 传输层 / 特性支持 | 承载引擎 |
| :--- | :--- | :--- |
| **HTTP / HTTPS** | 标准 HTTP CONNECT 隧道、Basic 认证 | Go 原生高性能拨号器 |
| **SOCKS5** | RFC 1928 用户名/密码认证、远端域名解析 | Go 原生高性能拨号器 |
| **Shadowsocks** | AEAD TCP（`aes-128-gcm`、`aes-256-gcm`、`chacha20-ietf-poly1305`） | Go 原生 AEAD 引擎 |
| **VLESS** | `REALITY`（uTLS Chrome 指纹）、`xtls-rprx-vision`、`TLS`、`TCP` / `WS` / `gRPC` | 内嵌 `sing-box`（`with_utls`） |
| **VMess** | `auto` / `aes-128-gcm` / `chacha20-poly1305`、`TLS`、`TCP` / `WS` / `gRPC` | 内嵌 `sing-box`（`with_utls`） |

### 3. 批量导入、订阅增量同步与主动探测
- **多格式智能导入**：支持一键粘贴或拉取 `vless://`、`vmess://`、`ss://`、`socks5://`、`http://`、`IP:PORT:USER:PASS`、Base64 订阅、Clash YAML 与 sing-box JSON，自动去重并反馈逐行解析报告。
- **机场订阅定时刷新**：内置 SSRF 防护与 ETag/Last-Modified 缓存协商，定时拉取远端订阅并按节点指纹增量 Diff（新增 / 更新 / 移除），不干扰手动维护的节点。
- **主动健康探测与出口 IP 识别**：周期性通过各上游并发探测连通性、RTT 延迟与**真实落地出口 IP**，同时支持真实业务转发失败时被动标记异常。

### 4. 安全存储与可观测性
- **凭据落盘加密**：SQLite（WAL 模式）中所有上游密码、账户密码与订阅链接均使用 `AES-256-GCM`（`enc:v1:`）加密存储；同时支持 `${ENV_VAR}` 与 `${ENV_VAR:-default}` 环境变量动态注入，实现密文/明文零落盘。
- **全链路结构化审计**：每次代理请求记录客户端 IP、入站端口、认证账户、协议、目标地址、上下行字节数、耗时与状态码，支持按端口/账户/级别实时检索与按天自动归档清理。

---

## 界面预览

<table>
  <tr>
    <td width="50%">
      <img src="./docs/assets/login.png" alt="登录页与三层分流架构图" />
      <p align="center"><sub><b>登录控制台</b> · 分栏式认证入口与端口/账户分流示意</sub></p>
    </td>
    <td width="50%">
      <img src="./docs/assets/upstreams.png" alt="多协议上游代理管理" />
      <p align="center"><sub><b>上游代理池</b> · VLESS REALITY / SS / SOCKS5 / HTTP 延迟与出口 IP 监测</sub></p>
    </td>
  </tr>
  <tr>
    <td width="50%">
      <img src="./docs/assets/accounts-copy.png" alt="账户管理与一键复制接入向导" />
      <p align="center"><sub><b>复制接入向导</b> · 按端口/账户一键生成 URL、curl、JSON 与分字段配置</sub></p>
    </td>
    <td width="50%">
      <img src="./docs/assets/dashboard-dark.png" alt="深色模式仪表盘" />
      <p align="center"><sub><b>深色主题（Dark Mode）</b> · 中英双语切换与系统主题自适应</sub></p>
    </td>
  </tr>
</table>

---

## 快速开始

### 方式一：Docker 一键部署

```bash
mkdir -p fastproxy && cd fastproxy

docker run -d --name fastproxy \
  --restart unless-stopped \
  -p 7788:7788 \
  -p 10001-10050:10001-10050 \
  -e ADMIN_PASSWORD=your-strong-password \
  -e PORT_RANGE=10001-10050 \
  -e FASTPROXY_SECRET_KEY="$(python3 -c 'import os,base64;print(base64.urlsafe_b64encode(os.urandom(32)).decode())')" \
  -v "$(pwd)/data:/data" \
  ghcr.io/handsomezhuzhu/fastproxy:1.0.0
```

### 方式二：Docker Compose 部署

```yaml
version: "3.3"
services:
  fastproxy:
    image: ghcr.io/handsomezhuzhu/fastproxy:${IMAGE_TAG:-1.0.0}
    container_name: fastproxy
    restart: unless-stopped
    environment:
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}               # 管理面板密码（必填）
      - PORT_RANGE=${PORT_RANGE:-10001-10050}          # 允许创建并映射的入站端口段
      - FASTPROXY_SECRET_KEY=${FASTPROXY_SECRET_KEY:-} # AES-256-GCM 密钥（留空则自动生成 /data/.secret_key）
      - LOGIN_MAX_ATTEMPTS=${LOGIN_MAX_ATTEMPTS:-5}    # 登录防暴力破解阈值
      - LOGIN_LOCK_DURATION=${LOGIN_LOCK_DURATION:-5m} # 触发阈值后的锁定时长
      - TRUSTED_PROXIES=${TRUSTED_PROXIES:-}           # 反向代理可信 CIDR（逗号分隔）
    ports:
      - "7788:7788"
      - "${PORT_RANGE:-10001-10050}:${PORT_RANGE:-10001-10050}"
    volumes:
      - ./data:/data
```

### 三步完成接入与验证

1. **导入上游节点**：访问 `http://<服务器IP>:7788` 登录控制台，在 **「上游代理」** 中添加或批量导入节点，或前往 **「订阅」** 添加机场订阅链接。
2. **创建入站端口**：在 **「端口线路」** 中新建端口（如 `10001`），系统将自动生成同名默认账号 `default-10001`。
3. **按账号分流调用**：在 **「账户」** 页面将默认账号或新建的自定义账号绑定至指定上游，点击右上角 **「复制接入」** 生成客户端命令：

```bash
# 同一端口自动识别 HTTP 代理与 SOCKS5 协议，按账号路由至对应上游出口
curl -x http://crawler-us-west:fp_us_99281a@proxy.example.com:10001 https://api.ipify.org
curl --proxy socks5h://crawler-us-west:fp_us_99281a@proxy.example.com:10001 https://api.ipify.org
```

---

## 环境变量与加密配置

上游密码、账户密码与订阅 URL 在 SQLite 中以 `AES-256-GCM` 密文存储（`enc:v1:` 前缀）。同时支持在表单中填写 `${ENV_VAR}` 或 `${ENV_VAR:-默认值}` 引用环境变量（如 `http://${UP_USER}:${UP_PASS}@1.2.3.4:8080`），运行时动态展开。

| 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `CONFIG` | `/data/fastproxy.db`（容器）/ `fastproxy.db` | SQLite 数据库文件路径（自动开启 WAL 模式） |
| `ADMIN_ADDR` | `:7788` | Web 控制台与管理 API 监听地址 |
| `ADMIN_PASSWORD` | 首次启动随机生成并打印一次 | 启动时设置或重置控制台管理员密码 |
| `PORT_RANGE` | `10001-10050` | 允许创建的入站端口范围，需与容器 `-p` 映射范围保持一致 |
| `FASTPROXY_SECRET_KEY` | 自动生成 `/data/.secret_key` | 数据库敏感字段加密密钥（32 字节 Base64 或 64 位 Hex） |
| `SESSION_SECRET` | 自动生成 | 控制台 Cookie 会话 HMAC 签名密钥 |
| `LOGIN_MAX_ATTEMPTS` | `5` | 登录连续失败次数上限 |
| `LOGIN_LOCK_DURATION` | `5m` | 登录失败达上限后的全局锁定时长（Go `time.Duration` 格式） |
| `TRUSTED_PROXIES` | 空 | 可信反向代理 CIDR 列表（用于解析 `X-Forwarded-For`） |
| `SUB_ALLOW_PRIVATE` | 空 | 设为 `1` 时允许订阅拉取访问内网/回环地址（如本地 Subconverter） |

> [!IMPORTANT]
> **数据备份说明**：所有配置与日志均存放在 `/data/fastproxy.db`。可通过 `sqlite3 /data/fastproxy.db ".backup backup.db"` 进行在线热备；若未通过环境变量固定 `FASTPROXY_SECRET_KEY`，**备份数据库时务必同时备份 `/data/.secret_key`**。

---

## 本地开发与测试

项目采用前后端一体化打包设计：前端构建产物（`admin/dist`）通过 Go `embed` 直接内嵌进单一可执行二进制文件。

```bash
# 1. 构建前端控制台（web/ -> admin/dist）
make web

# 2. 编译单文件服务端二进制（自动携带 -tags with_utls 启用 REALITY / uTLS）
make build

# 3. 运行 Go 全量单元与并发测试（含 config / proxy / singbox / subscription / admin）
go test -tags with_utls ./... -count=1

# 4. 运行 Playwright 浏览器端到端回归测试（20 项 UI + 后端全链路用例）
cd web && npx playwright test
```

### 仓库结构

```text
├── main.go            # 服务入口：协调 SQLite 存储、代理管理器、主动探测、订阅调度与管理面
├── admin/             # 管理端 HTTP API、会话鉴权、限流与内嵌前端静态资源（admin/dist）
├── config/            # SQLite 持久化层：Schema 迁移、AES-256-GCM 凭据加解密、CRUD 与指标聚合
├── proxy/             # 核心转发层：HTTP/SOCKS5 双协议多路复用入站、原生出站拨号与流量统计
├── singbox/           # 内嵌 sing-box 引擎生命周期管理（VLESS REALITY / Vision / WS / gRPC / VMess）
├── importer/          # 节点解析器：支持分享链接、Base64、Clash YAML、sing-box JSON 批量导入
├── subscription/      # 订阅拉取器：SSRF 防护、ETag 缓存与节点差异对比同步
├── probe/             # 主动健康探测器：并发测量上游延迟与落地出口 IP
├── logx/              # 结构化连接日志：内存环形缓冲 + 异步批量 SQLite 落库
├── web/               # 前端工程（Vite + React 19 + TypeScript + Tailwind v4 + shadcn/ui + Playwright）
└── docs/              # 架构设计（DESIGN.md）、API 契约（API.md）、反代配置与安全规范
```

---

## 文档导航

- [架构与数据模型设计 (`docs/DESIGN.md`)](./docs/DESIGN.md)
- [管理端 HTTP API 规范 (`docs/API.md`)](./docs/API.md)
- [安全机制与加密说明 (`docs/SECURITY.md`)](./docs/SECURITY.md)
- [Nginx / Caddy 反向代理配置 (`docs/REVERSE_PROXY.md`)](./docs/REVERSE_PROXY.md)

## License

Released under the [MIT License](./LICENSE).
