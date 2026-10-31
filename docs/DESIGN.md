# FastProxy 设计文档

## 定位

把入口端口稳定转发到固定上游 HTTP/SOCKS5 代理的轻量中转：

- **快转发**：连接路径极短（单跳 relay），无订阅调度与探测开销
- **专业配置**：上游池 / 端口线路 / 账户 三层模型，字段显式、可绑定、可覆盖
- **完备记录**：每次连接一条结构化日志，落库可查询、可筛选、按天保留

## 数据模型

```
Upstream（上游代理池）
  id, name, type(http|socks5), host, username, password, remark
       ▲ default_upstream            ▲ upstream_id (override)
       │                             │
Port（接入端口）               Account（账户）
  id, port, default_upstream,   id, username(全局唯一), password,
  enabled, remark               port_id(空=全部端口), upstream_id(空=跟随端口), enabled, remark
```

认证与路由：

```
客户端 → 端口（0x05 首字节=SOCKS5，否则 HTTP）
      → 账户（用户名 + 密码，绑定端口或全局）
      → 上游 = 账户.upstream_id ?? 端口.default_upstream（均空 = 直连）
      → 上游类型：http → CONNECT + Proxy-Authorization；socks5 → RFC1929 认证
```

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
| 纯文本行 | `1.2.3.4:8080`、`1.2.3.4:8080:user:pass`（默认 http） |
| base64 订阅 | 整段 base64 编码的文本订阅 |
| sing-box JSON | `{"outbounds":[{type:"http",server,server_port,username,password,tag}]}` |
| Clash JSON/YAML | `proxies:` 列表，流式与块式均支持 |

- 仅导入 http/socks5 节点，其余类型计入"跳过"
- 去重键：`type|host|username|password`，与现有上游比较后追加
- 返回 `{added, total, duplicates[], problems[]}`，面板展示明细

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
