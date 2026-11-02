# FastProxy

**轻量、便捷的多端口 HTTP/SOCKS5 转发中转服务。**

用户访问本服务器的某个端口（HTTP 或 SOCKS5 代理协议），流量按固定绑定关系转发到指定上游代理。带密码保护的 Web 控制台（React + Tailwind v4 + shadcn 官方组件），支持 **多端口接入、多账户接入、上游代理绑定与批量导入**，连接日志结构化落库。

## 功能

- **双协议入站**：同一端口同时支持 HTTP 代理（明文请求 + CONNECT 隧道）和 SOCKS5（自动识别），强制认证
- **三层模型**：上游代理池 / 端口（纯入口）/ 账户（每端口自带默认账号，账户决定去向）· 无“全局账户”概念，端口与账户是硬边界
- **快速开始三步**：建上游 → 建端口（自动生成默认账号）→ 客户端立即可用；需要更多身份时就继续加普通账号并绑定上游
- **上游多协议**：上游出站支持 `http`、`socks5`、`Shadowsocks`（AEAD TCP：aes-128/256-gcm、chacha20-ietf-poly1305）、
  `VLESS`（TCP，可 TLS；REALITY / `flow=xtls-rprx-vision` / WebSocket / gRPC 由内嵌 sing-box 承载）、
  `VMess`（内嵌 sing-box：TCP/WS/gRPC，可 TLS）
- **上游批量导入**：订阅链接或文本粘贴，支持
  `http://user:pass@host:port#名称`、`socks5://`、`ss://`、`vless://`、`vmess://`、
  `IP:PORT`、`IP:PORT:USER:PASS`、base64 订阅、sing-box JSON、Clash JSON/YAML 节点列表，
  REALITY（`pbk`/`sid`）、Vision flow、ws/grpc 传输字段一并保存，自动去重并给出逐行错误
  （quic/kcp/h2 等无法承载的传输会明确跳过）
- **订阅定时刷新**：保存机场订阅 URL，按间隔拉取并按节点差异更新上游（新增/更新/移除），
  失败原因、上次刷新时间、节点数在面板可见；手动添加的上游不会被改动
- **上游测速**：控制台一键测试连通性与延迟
- **主动探测与 IP 检测**：定时（默认 5 分钟）通过每个上游请求出口 IP，记录健康状态、延迟、失败原因与**实际出口 IP**；本机直连出口 IP 同步展示；真实连接失败也会被动标记上游异常
- **复制向导**：选择「端口 / 账户」→ 选择对象 → 选择格式（HTTP URL、SOCKS5 URL、curl 命令、分字段、JSON），一键批量复制
- **完备日志**：每次连接一条结构化记录（客户端 IP / 账户 / 协议 / 目标 / 状态 / 流量 / 耗时），
  **SQLite 持久化**，控制台按级别/端口/账户筛选，按天自动清理
- **总设置入口**：日志级别/缓存/保留天数、访问日志开关、连接/握手超时、**探测开关/间隔/超时/并发/目标**、管理密码；直连由账户是否绑定上游决定
- **热加载**：账户/上游/设置即时生效，不断已建立连接；仅端口变更重启对应监听
- **敏感配置**：上游/账户密码在 SQLite 中用 AES-256-GCM 加密（密钥来自 `FASTPROXY_SECRET_KEY` 或 `/data/.secret_key`）；也支持 `${VAR}` 环境变量引用
- **中英双语与主题**：中文/English 切换，亮色/暗色/跟随系统（首次默认跟随系统）
- **部署简单**：单静态二进制 / Alpine 容器（前端内嵌），数据为单文件 SQLite

## 快速开始（Docker）

```bash
mkdir fastproxy && cd fastproxy
docker run -d --name fastproxy \
  -p 7788:7788 -p 10001-10050:10001-10050 \
  -e ADMIN_PASSWORD=your-admin-password \
  -e PORT_RANGE=10001-10050 \
  -e FASTPROXY_SECRET_KEY="$(python3 -c 'import os,base64;print(base64.urlsafe_b64encode(os.urandom(32)).decode())')" \
  -v $(pwd)/data:/data \
  ghcr.io/handsomezhuzhu/fastproxy:1.0.0-rc.3
```

打开 `http://服务器IP:7788` 登录控制台，按「仪表盘 → 快速开始」三步配置：

1. **上游代理** → 添加或批量导入（如 `http://user:pass@1.2.3.4:8080`）
2. **端口线路** → 新建监听端口 `10201`（系统自动生成默认账号 `default-10201`）
3. **账户** → 把默认账号绑到刚建的上游，或另加普通账号（每人可有自己的上游）

客户端使用（同一端口两种协议都可用，向谁认证就走谁的上游）：

```bash
# HTTP 代理
curl -x http://alice:密码@服务器IP:10201 https://api.ipify.org

# SOCKS5 代理
curl --proxy socks5h://alice:密码@服务器IP:10201 https://api.ipify.org
```

或在控制台点击「复制接入」，用复制向导生成任意格式的接入信息。

## docker-compose

```yaml
services:
  fastproxy:
    image: ghcr.io/handsomezhuzhu/fastproxy:${IMAGE_TAG:-1.0.0-rc.3}
    restart: unless-stopped
    environment:
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}        # 管理面板密码（必须）
      - PORT_RANGE=${PORT_RANGE:-10001-10050}   # 允许创建并映射的端口范围
      - FASTPROXY_SECRET_KEY=${FASTPROXY_SECRET_KEY:-}  # 数据密码加密密钥（建议设置）
      - LOGIN_MAX_ATTEMPTS=${LOGIN_MAX_ATTEMPTS:-5}
      - LOGIN_LOCK_DURATION=${LOGIN_LOCK_DURATION:-5m}
      - TRUSTED_PROXIES=${TRUSTED_PROXIES:-}    # Nginx/OpenResty 反代 CIDR
    ports:
      - "7788:7788"                # 管理面板（默认 7788）
      - "${PORT_RANGE:-10001-10050}:${PORT_RANGE:-10001-10050}"
    volumes:
      - ./data:/data               # SQLite 数据库 + .secret_key
```

## 敏感配置与环境变量

上游/账户密码在 SQLite 中以 AES-256-GCM 密文保存（`enc:v1:`），读取时解密；密钥来自 `FASTPROXY_SECRET_KEY`，未设置时自动生成 `/data/.secret_key`（0600）。备份数据库时必须一并备份密钥。

密码也支持 `${VAR}` 或 `${VAR:-默认值}` 引用，运行时从环境展开。启动与保存时会严格校验，缺失变量会拒绝启动并列出名称。

在控制台把上游地址填成 `http://${UPSTREAM_USER_1}:${UPSTREAM_PASS_1}@1.2.3.4:8080` 即可。

| 变量 | 默认 | 说明 |
|---|---|---|
| `CONFIG` | `/data/fastproxy.db`（容器）/ `fastproxy.db` | SQLite 数据库路径 |
| `ADMIN_ADDR` | `:7788` | 管理面板监听地址 |
| `ADMIN_PASSWORD` | 无 | 启动时重置管理面板密码 |
| `PORT_RANGE` | `10001-10050` | 允许创建/映射的入站端口范围；Docker 映射必须一致 |
| `FASTPROXY_SECRET_KEY` | 自动生成 `/data/.secret_key` | 数据密码加密密钥（32 字节 base64/hex） |
| `LOGIN_MAX_ATTEMPTS` | `5` | 面板登录全局失败次数上限 |
| `LOGIN_LOCK_DURATION` | `5m` | 锁定持续时间（Go duration） |
| `TRUSTED_PROXIES` | 无 | 可信反向代理 CIDR，逗号分隔 |
| `SUB_ALLOW_PRIVATE` | 无 | 设为 `1` 才允许订阅拉取内网/回环地址（本地转换器） |
| `SESSION_SECRET` | 自动生成 | 可选：固定会话签名密钥 |

> 从旧版本升级：旧的 `config.json` 放在数据库同目录，首次启动会自动迁移为 `config.json.migrated`。

## 数据与备份

- 全部状态在 SQLite 单文件（WAL 模式）：`/data/fastproxy.db`（+ `-wal`/`-shm`）
- 备份：`sqlite3 fastproxy.db ".backup backup.db"` 或停机复制文件；**同时备份 `/data/.secret_key`**（若未用环境变量）
- 日志表按 `log_retention_days`（默认 7 天）自动清理，可在控制台调整

## 工程流程

```bash
make web      # 构建前端 web/ -> admin/dist（React + Tailwind v4 + shadcn 组件）
make build    # 构建后端（前端内嵌进二进制；缺少 dist 时会先自动 make web）
make test     # Go 测试（含 sing-box/VLESS/VMess，使用 with_utls）
make e2e      # 容器端到端（Docker：面板/配置/HTTP/SOCKS5/CONNECT/认证/统计）
make docker   # 多阶段镜像（node + go + alpine）
```

> `admin/dist` 与 `deploy/` 不再提交到仓库：CI/Docker/Release 会现场构建前端；部署脚本仅本地使用。

CI（`.github/workflows/ci.yml`）：`runs-on: instabox-medium`，前端构建 → go vet → go test；
main 分支推 GHCR 镜像（amd64/arm64）；打 tag 发布多平台二进制 Release。

> instabox CI 首次使用：在 [InstaBox 控制台](https://dashboard.instabox.sh) 的 **CI** 页完成一次 **Connect GitHub**（安装 App 并授权仓库），之后 push 自动运行。

## 目录结构

```
main.go              入口（管理面 + 代理线路 + 日志清理 + 订阅调度）
config/              SQLite 存储（迁移/加密/上游/端口/账户/设置/日志/订阅）
proxy/               双协议入站、上游隧道（http/socks5/ss）、管理器、访问统计与 trace
singbox/             内嵌 sing-box（VLESS REALITY/Vision/WS/gRPC + VMess）
subscription/        订阅拉取（SSRF 防护/ETag）与节点 diff 刷新
admin/               管理 API + 内嵌前端（dist）
importer/            上游导入解析（URI/纯文本/base64/sing-box/Clash）
logx/                结构化日志（内存环形 + 异步批量落库）
web/                 前端源码（Vite + React + shadcn 官方组件）
scripts/             devserver + 端到端测试脚本
docs/                设计/API/安全/反代文档
```

## 安全提示

- 面板建议置于内网/Nginx HTTPS 之后；所有代理端口强制认证
- 上游/账户密码加密存储，备份数据库时务必同时备份密钥；订阅拉取默认拒绝内网地址
- 使用控制台「连接日志」审计转发行为

## License

MIT
