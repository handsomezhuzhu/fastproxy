# FastProxy

**轻量、便捷的多端口 HTTP/SOCKS5 转发中转服务。**

用户访问本服务器的某个端口（HTTP 或 SOCKS5 代理协议），流量按固定绑定关系转发到指定上游代理。带密码保护的 Web 管理面板（React + shadcn/ui 风格），支持 **多端口接入、多账户接入、上游代理绑定与批量导入**。

> 设计理念与 [Resin](https://github.com/Resinat/Resin)（代理池调度网关）不同：FastProxy 是**固定转发中转**——不订阅调度，但导入、账户、可观测性的设计语言与 Resin 对齐。

## 功能

- **双协议入站**：同一端口同时支持 HTTP 代理（明文请求 + CONNECT 隧道）和 SOCKS5（自动识别），强制认证
- **三层模型**：上游代理池 / 端口线路（默认上游）/ 账户（可绑定端口、可覆盖上游实现按用户分流）
- **上游批量导入（Resin 风格）**：订阅链接或文本粘贴，支持
  `http://user:pass@host:port#名称`、`socks5://`、`IP:PORT`、`IP:PORT:USER:PASS`、
  base64 订阅、sing-box JSON、Clash JSON/YAML 节点列表，自动去重并给出逐行错误
- **上游测速**：面板一键测连通性与延迟
- **复制向导**：选择「端口 / 账户」→ 选择对象 → 选择格式（HTTP URL、SOCKS5 URL、curl 命令、分字段、JSON），一键批量复制
- **完备日志**：每次连接一条结构化日志（客户端 IP / 账户 / 协议 / 目标 / 状态 / 流量 / 耗时），
  内存环形缓冲 + JSONL 文件（8MB 轮转），面板支持按级别/端口/账户筛选
- **总设置入口**：日志级别与条数、访问日志开关、直连策略、连接/握手超时、管理密码
- **热加载**：账户/上游/设置即时生效，不断已建立连接；仅端口变更重启对应监听
- **敏感配置**：密码支持 `${VAR}` / `${VAR:-默认值}` 环境变量引用，明文不落盘
- **部署简单**：单静态二进制 / Alpine 容器（前端内嵌），配置单 JSON 文件

## 快速开始（Docker）

```bash
mkdir fastproxy && cd fastproxy
docker run -d --name fastproxy \
  -p 7788:7788 -p 10001-10500:10001-10500 \
  -e ADMIN_PASSWORD=your-admin-password \
  -v $(pwd)/data:/data \
  ghcr.io/handsomezhuzhu/fastproxy:latest
```

打开 `http://服务器IP:7788` 登录面板，按「仪表盘 → 快速开始」三步配置：

1. **上游代理** → 添加或批量导入（如 `http://user:pass@1.2.3.4:8080`）
2. **端口线路** → 新建监听端口 `10201`，绑定默认上游（留空 = 直连）
3. **账户** → 新建用户名/密码，可绑定端口或全局可用

客户端使用（同一端口两种协议都可用）：

```bash
# HTTP 代理
curl -x http://alice:密码@服务器IP:10201 https://api.ipify.org

# SOCKS5 代理
curl --proxy socks5h://alice:密码@服务器IP:10201 https://api.ipify.org
```

或在面板中点击「复制」，用复制向导生成任意格式的接入信息。

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
      - ./data:/data
```

## 敏感配置与环境变量

配置文件中上游账号/密码、账户密码支持 `${VAR}` 或 `${VAR:-默认值}` 引用，运行时从环境展开；**明文不落盘**。启动与保存时严格校验，缺失变量会拒绝启动并列出名称。

在面板中把上游地址填成 `http://${UPSTREAM_USER_1}:${UPSTREAM_PASS_1}@1.2.3.4:8080` 即可。

| 变量 | 默认 | 说明 |
|---|---|---|
| `CONFIG` | `/data/config.json`（容器）/ `fastproxy.json` | 配置文件路径 |
| `ADMIN_ADDR` | `:7788` | 管理面板监听地址 |
| `ADMIN_PASSWORD` | 无 | 启动时重置管理面板密码 |
| `SESSION_SECRET` | 无 | 面板会话签名密钥（设置后不落盘） |

## 工程流程

```bash
make web      # 构建前端 web/ -> admin/dist（React + Tailwind + Radix，shadcn 风格）
make build    # 构建后端（前端内嵌进二进制）
make test     # Go 测试（config / importer / proxy 端到端）
make e2e      # 容器端到端（Docker：面板/配置/HTTP/SOCKS5/CONNECT/认证/统计）
make docker   # 多阶段镜像（node + go + alpine）
```

CI（`.github/workflows/ci.yml`）：`runs-on: instabox-medium`，前端构建 → go vet → go test；
main 分支推 GHCR 镜像（amd64/arm64）；打 tag 发布多平台二进制 Release。

> instabox CI 首次使用：在 [InstaBox 控制台](https://dashboard.instabox.sh) 的 **CI** 页完成一次 **Connect GitHub**（安装 App 并授权仓库），之后 push 自动运行。

## 目录结构

```
main.go              入口（监听管理面 + 启动代理线路）
config/              配置存储（上游/端口/账户/设置）+ 环境变量展开
proxy/               双协议入站、上游隧道、管理器、访问统计
admin/               管理 API + 内嵌前端（dist）
importer/            上游导入解析（URI/纯文本/base64/sing-box/Clash）
logx/                结构化日志（内存环形 + JSONL 轮转）
web/                 前端源码（Vite + React + shadcn 风格组件）
scripts/             devserver + 端到端测试脚本
docs/                设计文档 / API 文档
```

## 安全提示

- 面板建议置于内网/Nginx HTTPS 之后；所有代理端口强制认证
- 上游凭据保存在服务器本地配置文件（`${VAR}` 引用除外），注意文件权限
- 使用面板「系统设置 → 记录连接日志」审计转发行为

## License

MIT
