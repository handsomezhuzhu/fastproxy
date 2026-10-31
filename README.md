# FastProxy

**轻量、便捷的多端口 HTTP/SOCKS5 转发中转服务。**

用户访问本服务器的某个端口（HTTP 或 SOCKS5 代理协议），流量按固定绑定关系转发到指定上游代理。带密码保护的 Web 管理面板，支持 **多端口接入、多账户接入、上游代理绑定**。

> 设计理念与 [Resin](https://github.com/Resinat/Resin)（代理池调度网关）不同：FastProxy 是**固定转发中转**——不订阅节点、不做调度，适合"把某些入口端口稳定转发到固定 HTTP 代理"的场景。

## 功能

- **双协议入站**：同一端口同时支持 HTTP 代理（明文请求 + CONNECT 隧道）和 SOCKS5（自动识别）
- **强制认证**：SOCKS5 仅提供用户名/密码认证方式；HTTP 校验 `Proxy-Authorization`
- **三层模型**：
  - **上游代理（Upstream）**：`http://` 或 `socks5://` 上游池，含凭据，全局限复用
  - **端口线路（Port）**：每个监听端口绑定一个默认上游（留空则服务器直连）
  - **账户（Account）**：全局唯一用户名；可绑定端口（仅该端口可用）、可**覆盖端口默认上游**实现按用户分流
- **热加载**：账户/上游修改立即对新连接生效，不断正在转发的连接；仅端口本身增删才会重启对应监听
- **管理面板**：密码登录（PBKDF2 哈希存储）、登录限速、三页签管理（端口线路/账户/上游）、连接数与流量统计（端口+账户两级）
- **可观测**：每个端口/账户的当前连接、累计连接、双向流量；监听失败原因直接显示在面板
- **部署简单**：单个 Go 静态二进制 / 一个 Alpine 容器（~20MB），配置为单 JSON 文件，Docker volume 持久化

## 快速开始（Docker）

```bash
mkdir fastproxy && cd fastproxy
mkdir data

# 首次启动并设置管理密码
docker run -d --name fastproxy \
  -p 7788:7788 -p 10001-10500:10001-10500 \
  -e ADMIN_PASSWORD=your-admin-password \
  -v $(pwd)/data:/data \
  ghcr.io/handsomezhuzhu/fastproxy:latest
```

打开 `http://服务器IP:7788` 登录面板：

1. **上游代理** → 新增：如 `http://user:pass@1.2.3.4:8080`
2. **端口线路** → 新增：监听端口 `10001`，默认上游选刚建的上游
3. **账户** → 新增：用户名 `alice` / 密码，绑定端口 `10001`

客户端即可使用：

```bash
# HTTP 代理
curl -x http://alice:密码@服务器IP:10001 https://api.ipify.org

# SOCKS5 代理（同一端口）
curl --proxy socks5h://alice:密码@服务器IP:10001 https://api.ipify.org
```

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
      - "10001-10500:10001-10500"  # 代理端口范围（默认 10001-10500，按需调整/扩大）
    volumes:
      - ./data:/data
```

配套 `.env`（与 compose 同目录，勿提交仓库）：

```dotenv
ADMIN_PASSWORD=change-me
UPSTREAM_USER_1=myuser
UPSTREAM_PASS_1=secret
```

## 敏感配置与环境变量

**明文密码不必落盘。** 端口线路/账户/上游保存在 `CONFIG` 文件里，敏感字段支持 `${VAR}`（或带默认值 `${VAR:-xxxx}`）引用，运行时从环境变量展开：

| 位置 | 支持环境变量引用的字段 |
|---|---|
| 上游代理 | 账号、密码 |
| 账户 | 密码 |

在面板里把上游地址填成 `http://${UPSTREAM_USER_1}:${UPSTREAM_PASS_1}@1.2.3.4:8080` 即可——配置文件中只保存 `${VAR}` 引用，真实密码只存在于部署环境（compose `.env`、`docker run -e`、systemd `Environment=` 等）。

- 启动时校验：引用的环境变量缺失会**拒绝启动**并列出变量名；面板保存时同样校验
- `ADMIN_PASSWORD` 环境变量：每次启动重置管理面板密码
- `SESSION_SECRET` 环境变量：设置后作为面板会话签名密钥（多实例共用），不写入配置文件

> Docker 无法给已运行的容器追加映射端口，请预发布足够的端口范围，或使用 host 网络。
> 端口线路的"监听端口"必须落在发布的范围内。

## 转发规则（认证 → 上游解析）

```
客户端 → 端口（HTTP / SOCKS5 自动识别）
       → 账户认证（账户绑定端口 或 全局）
       → 上游 = 账户覆盖的上游 ?? 端口默认上游（均无 = 直连）
       → 上游为 HTTP 代理：CONNECT 隧道并携带上游凭据
       → 上游为 SOCKS5：完整 SOCKS5 握手（含上游认证）
```

## 本地运行（源码）

```bash
go build -o fastproxy .
CONFIG=./fastproxy.json ADMIN_ADDR=:7788 ADMIN_PASSWORD=admin123 ./fastproxy
# 不设 ADMIN_PASSWORD 时启动会生成随机密码并打印一次
```

环境变量：

| 变量 | 默认 | 说明 |
|---|---|---|
| `CONFIG` | `/data/config.json`（容器）/ `fastproxy.json` | 配置文件路径 |
| `ADMIN_ADDR` | `:7788` | 管理面板监听地址（代理端口范围由面板配置，见 compose 映射） |
| `ADMIN_PASSWORD` | 无 | 启动时重置管理面板密码 |
| `SESSION_SECRET` | 无 | 面板会话签名密钥（设置后不落盘） |

## 面板 API（供脚本化）

登录后（Cookie 会话）：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/data` | 全量配置 + 状态 |
| GET | `/api/stats` | 端口/账户统计 |
| POST/PUT/DELETE | `/api/upstreams[/{id}]` | 上游代理管理 |
| POST/PUT/PATCH/DELETE | `/api/ports[/{id}]` | 端口线路管理 |
| POST/PUT/PATCH/DELETE | `/api/accounts[/{id}]` | 账户管理 |
| POST | `/api/password` | 修改管理密码 |

## CI

使用 [InstaBox CI](https://instabox.sh/blog/github-actions)：`.github/workflows/ci.yml` 中 `runs-on: instabox-medium`。

> 首次使用需在 [InstaBox 控制台](https://dashboard.instabox.sh) 的 **CI** 页面完成一次 **Connect GitHub**（安装 instabox GitHub App 并关联仓库所在账号/组织），之后 push 即自动跑测试；打 tag（`v*`）会构建多平台二进制并发布 GitHub Release，main 分支构建镜像推送 GHCR。

## 安全提示

- 请把管理面板端口置于内网/防火墙之后，或用 Nginx 反代加 HTTPS
- 所有代理端口强制认证，防止被滥用为公共代理
- 上游凭据保存在服务器本地 `CONFIG` 文件中，注意文件权限（默认 0600）

## License

MIT
