# FastProxy 设计文档

## 定位

为"把入口端口稳定转发到固定上游 HTTP 代理"而生的轻量中转：

- **快转发**：连接路径极短（单跳 relay），无订阅调度、无健康探测开销
- **专业配置**：上游池 / 端口线路 / 账户 三层模型，字段显式、可绑定、可覆盖
- **完备记录**：每次连接一条结构化日志（客户端/账户/目标/状态/流量/耗时）

与 [Resin](https://github.com/Resinat/Resin) 的差异：Resin 是代理池调度网关（节点来自订阅、自动调度、粘连 IP）；FastProxy 是**静态绑定中转**，转发目标是人工指定的上游代理，但**导入、账户、可观测性的设计语言对齐 Resin**。

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

## 热加载

- 账户 / 上游的增删改：**不重启监听**，下一次连接即生效（认证与上游解析每次都读取实时配置）
- 端口定义变更（端口号、默认上游）：仅重启该端口的监听
- 修改系统设置：日志器即时应用；超时在下一连接读取

## 敏感信息

- 上游账号/密码、账户密码支持 `${VAR}` / `${VAR:-默认值}` 环境变量引用
- 配置文件只保存引用，**明文密码不落盘**；启动与保存时严格校验（缺失变量 → 拒绝）
- 管理密码 PBKDF2-SHA256(120k 迭代) 哈希存储，登录失败按 IP 限速

## 日志

- 每次连接一条 JSONL：`time/level/port/account/client/proto/target/status/bytes_in/bytes_out/duration_ms/msg`
- 状态：`ok`、`auth-failed`、`dial-error`、`upstream-error`、`bad-request`
- 双写：内存环形缓冲（面板实时查询）+ `CONFIG 目录/access.log`（8MB 轮转一次）
- 访问日志可关闭（关闭后仅记录失败连接）；系统消息（启动/停止/导入/登录失败）同样入库

## 上游导入（Resin 风格）

`POST /api/upstreams/import`，输入 `{url}` 或 `{text}`，解析：

| 格式 | 示例 |
|---|---|
| URI 行 | `http://user:pass@1.2.3.4:8080#名称`、`socks5://...` |
| 纯文本行 | `1.2.3.4:8080`、`1.2.3.4:8080:user:pass`（默认 http） |
| base64 订阅 | 整段 base64 编码的文本订阅 |
| sing-box JSON | `{"outbounds":[{type:"http",server,server_port,username,password,tag}]}` |
| Clash JSON/YAML | `proxies:` 列表，流式 `- {name,type,server,port,...}` 与块式均支持 |

- 仅导入 http/socks5 节点，其余类型计入"跳过"
- 去重键：`type|host|username|password`，与现有上游比较后追加
- 返回 `{added, total, duplicates[], problems[]}`，面板展示明细

## 管理面 API

见 `docs/API.md`。

## 构建流程

```
web (React+Tailwind+Radix, shadcn 风格) --vite build--> admin/dist
                                                         │ go:embed
go build  ───────────────────────────────────────────────┘
   └── 单二进制（前端内嵌）+ Docker（node → go → alpine）
```

`make web && make build`；`make test`；`make e2e`（容器端到端）。
