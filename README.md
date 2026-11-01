# FastProxy

**轻量、便捷的多端口 HTTP/SOCKS5 转发中转服务。**

用户访问本服务器的某个端口（HTTP 或 SOCKS5 代理协议），流量按固定绑定关系转发到指定上游代理。带密码保护的 Web 控制台（React + Tailwind v4 + shadcn 官方组件），支持 **多端口接入、多账户接入、上游代理绑定与批量导入**，连接日志结构化落库。

## 功能

- **双协议入站**：同一端口同时支持 HTTP 代理（明文请求 + CONNECT 隧道）和 SOCKS5（自动识别），强制认证
- **三层模型**：上游代理池 / 端口（纯入口）/ 账户（每端口自带默认账号，账户决定去向）· 无“全局账户”概念，端口与账户是硬边界
- **快速开始三步**：建上游 → 建端口（自动生成默认账号）→ 客户端立即可用；需要更多身份时就继续加普通账号并绑定上游
- **上游批量导入**：订阅链接或文本粘贴，支持
  `http://user:pass@host:port#名称`、`socks5://`、`IP:PORT`、`IP:PORT:USER:PASS`、
  base64 订阅、sing-box JSON、Clash JSON/YAML 节点列表，自动去重并给出逐行错误
- **上游测速**：控制台一键测试连通性与延迟
- **主动探测与 IP 检测**：定时（默认 5 分钟）通过每个上游请求出口 IP，记录健康状态、延迟、失败原因与**实际出口 IP**；本机直连出口 IP 同步展示；真实连接失败也会被动标记上游异常
- **复制向导**：选择「端口 / 账户」→ 选择对象 → 选择格式（HTTP URL、SOCKS5 URL、curl 命令、分字段、JSON），一键批量复制
- **完备日志**：每次连接一条结构化记录（客户端 IP / 账户 / 协议 / 目标 / 状态 / 流量 / 耗时），
  **SQLite 持久化**，控制台按级别/端口/账户筛选，按天自动清理
- **总设置入口**：日志级别/缓存/保留天数、访问日志开关、直连策略、连接/握手超时、**探测开关/间隔/超时/目标**、管理密码
- **热加载**：账户/上游/设置即时生效，不断已建立连接；仅端口变更重启对应监听
- **敏感配置**：密码支持 `${VAR}` / `${VAR:-默认值}` 环境变量引用，明文不落盘
- **部署简单**：单静态二进制 / Alpine 容器（前端内嵌），数据为单文件 SQLite

## 快速开始（Docker）

```bash
mkdir fastproxy && cd fastproxy
docker run -d --name fastproxy \
  -p 7788:7788 -p 10001-10500:10001-10500 \
  -e ADMIN_PASSWORD=your-admin-password \
  -v $(pwd)/data:/data \
  ghcr.io/handsomezhuzhu/fastproxy:latest
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
    image: ghcr.io/handsomezhuzhu/fastproxy:latest
    restart: unless-stopped
    environment:
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}        # 管理面板密码（必须）
      - SESSION_SECRET=${SESSION_SECRET:-}     # 可选：固定会话签名密钥
      - UPSTREAM_USER_1=${UPSTREAM_USER_1:-}   # 可选：注入上游凭据
      - UPSTREAM_PASS_1=${UPSTREAM_PASS_1:-}
    ports:
      - "7788:7788"                # 管理面板（默认 7788）
      - "10001-10500:10001-10500"  # 代理端口范围（按需扩大）
    volumes:
      - ./data:/data               # SQLite 数据库（fastproxy.db）
```

## 敏感配置与环境变量

数据库中的上游账号/密码、账户密码支持 `${VAR}` 或 `${VAR:-默认值}` 引用，运行时从环境展开；**明文不落盘**。启动与保存时严格校验，缺失变量会拒绝启动并列出名称。

在控制台把上游地址填成 `http://${UPSTREAM_USER_1}:${UPSTREAM_PASS_1}@1.2.3.4:8080` 即可。

| 变量 | 默认 | 说明 |
|---|---|---|
| `CONFIG` | `/data/fastproxy.db`（容器）/ `fastproxy.db` | SQLite 数据库路径 |
| `ADMIN_ADDR` | `:7788` | 管理面板监听地址 |
| `ADMIN_PASSWORD` | 无 | 启动时重置管理面板密码 |
| `SESSION_SECRET` | 无 | 面板会话签名密钥（设置后不落盘） |

> 从旧版本升级：旧的 `config.json` 放在数据库同目录，首次启动会自动迁移为 `config.json.migrated`。

## 数据与备份

- 全部状态在 SQLite 单文件（WAL 模式）：`/data/fastproxy.db`（+ `-wal`/`-shm`）
- 备份：`sqlite3 fastproxy.db ".backup backup.db"` 或停机复制文件
- 日志表按 `log_retention_days`（默认 7 天）自动清理，可在控制台调整

## 工程流程

```bash
make web      # 构建前端 web/ -> admin/dist（React + Tailwind v4 + shadcn 组件）
make build    # 构建后端（前端内嵌进二进制）
make test     # Go 测试（config/importer/proxy 端到端）
make e2e      # 容器端到端（Docker：面板/配置/HTTP/SOCKS5/CONNECT/认证/统计）
make docker   # 多阶段镜像（node + go + alpine）
```

CI（`.github/workflows/ci.yml`）：`runs-on: instabox-medium`，前端构建 → go vet → go test；
main 分支推 GHCR 镜像（amd64/arm64）；打 tag 发布多平台二进制 Release。

> instabox CI 首次使用：在 [InstaBox 控制台](https://dashboard.instabox.sh) 的 **CI** 页完成一次 **Connect GitHub**（安装 App 并授权仓库），之后 push 自动运行。

## 目录结构

```
main.go              入口（管理面 + 代理线路 + 日志清理）
config/              SQLite 存储（上游/端口/账户/设置/日志）+ 环境变量展开
proxy/               双协议入站、上游隧道、管理器、访问统计与 trace
admin/               管理 API + 内嵌前端（dist）
importer/            上游导入解析（URI/纯文本/base64/sing-box/Clash）
logx/                结构化日志（内存环形 + 异步批量落库）
web/                 前端源码（Vite + React + shadcn 官方组件）
scripts/             devserver + 端到端测试脚本
docs/                设计文档 / API 文档
```

## 安全提示

- 面板建议置于内网/Nginx HTTPS 之后；所有代理端口强制认证
- 上游凭据支持环境变量引用，避免明文进入数据库
- 使用控制台「连接日志」审计转发行为

## License

MIT
