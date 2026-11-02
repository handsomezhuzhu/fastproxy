# FastProxy API

所有管理接口使用 Cookie 会话（`POST /api/login` 后自动携带），要求 `Content-Type: application/json`。

## 会话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/session` | `{auth, version}` |
| POST | `/api/login` | `{password}` → 设置会话 Cookie；连续失败达到 `LOGIN_MAX_ATTEMPTS` 后返回 429 + `Retry-After`/`{error, retry_after_sec}`，锁定仅在内存、重启清零 |
| POST | `/api/logout` | 退出 |
| POST | `/api/password` | `{old, new}` 修改管理密码 |

## 数据

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/data` | `{version, settings, upstreams, ports, accounts, port_range:{min,max}, status:{ports,accounts}, probe}` |
| GET | `/api/stats` | 仅统计 `{status}` |
| GET | `/api/logs?limit=&level=&port=&account=` | 连接日志（SQLite 查询），`level` 为最低级别 |
| GET | `/api/settings` | 全局设置 |
| PUT | `/api/settings` | 保存全局设置 |
| GET | `/api/singbox/status` | 内置 sing-box 状态：`{running, started_at, reload_count, last_reload_at, last_error, upstreams:[{id,name,port}], logs:[string]}` |

设置字段：`access_log, log_level(debug|info|warn|error), log_buffer(100-10000),
log_retention_days(1-90), dial_timeout_sec(3-120), handshake_timeout_sec(10-300),
probe_enabled, probe_interval_sec(30-3600), probe_timeout_sec(3-30), probe_concurrency(1-32),
probe_target(域名)`。直连不再有全局开关：账户不绑定上游即直连。

## 上游代理

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/upstreams` | 见下方两种请求形状 |
| PUT | `/api/upstreams/{id}` | 同上 |
| DELETE | `/api/upstreams/{id}` | 被引用时返回错误 |
| POST | `/api/upstreams/check` | 唯一探测入口：`{ids:["..."]}`（空/缺省=全部）→ `{results:[{id,ok,latency_ms,exit_ip,error}], ok, failed}`。前端的逐行实时刷新就是对单个 id 调用它 |
| POST | `/api/upstreams/import` | `{url}` 或 `{text}` → `{added,total,duplicates[],problems[],warning?}` |
| POST | `/api/upstreams/parse` | `{text:"vless://…"}` → `{proxy:{name,type,host,uuid,password,method,sni,security,pbk,sid,flow,fingerprint,transport,path,host_header,service_name,vmess_security}}`；解析失败 400 `{error:"中文原因"}`。只读，供前端“粘贴分享链接自动填表” |

`/api/data` 与 `GET /api/upstreams` 的 `upstreams[]` 字段：
`id, name, type, host, username?, password?, remark?, method?, uuid?, sni?`，
vless/vmess 高级字段 `security(""|none|tls|reality), pbk?, sid?, flow?, fingerprint?, transport?(""|tcp|ws|grpc), path?, host_header?, service_name?, vmess_security?`，
以及探测字段 `health(ok|fail|unknown), latency_ms, exit_ip, last_check_at(秒), last_error`；
另有 `probe:{server_ip,last_check_at}`。

> 上游增删改/导入后服务端会重建内嵌 sing-box：**所有 VLESS 与 VMess** 经
> 127.0.0.1:20000 起的本地 SOCKS 出站；http/socks5/ss 仍走原生实现。
> 正式二进制需以 `-tags with_utls` 构建（Dockerfile / CI / `make build-utls` 已带）。

### 上游创建/更新的两种请求形状

1. **地址形式**（http / socks5，兼容旧前端）
   ```json
   {"name":"US-01","type":"http","upstream":"http://user:pass@1.2.3.4:8080","remark":""}
   ```
   `upstream` 以 `ss://` / `vmess://` / `vless://` 开头时会自动用导入器解析为字段。

2. **字段形式**（推荐，所有类型）
   ```json
   {"name":"HK-SS","type":"shadowsocks","host":"1.2.3.4:8388",
    "method":"aes-256-gcm","password":"secret","remark":""}
   ```
   ```json
   {"name":"JP-VLESS","type":"vless","host":"j.example.com:443",
    "uuid":"b831381d-6324-4d53-ad4f-8cda48b30811","security":"tls","sni":"cdn.example.com"}
   ```
   ```json
   {"name":"JP-REALITY","type":"vless","host":"j.example.com:443",
    "uuid":"b831381d-6324-4d53-ad4f-8cda48b30811","security":"reality",
    "pbk":"H5M42pBqRHTcRX3_LPHDXTeO0cEz_G_zYud220O96As","sid":"0123456789abcdef",
    "flow":"xtls-rprx-vision","fingerprint":"chrome","sni":"www.apple.com"}
   ```

`type` 取值：`http` | `socks5` | `shadowsocks` | `vless` | `vmess`（空值视为 `http`）。
校验规则：ss 必填 `method`（`aes-128-gcm`/`aes-256-gcm`/`chacha20-ietf-poly1305`）与 `password`；
vless/vmess 必填 `uuid`；TLS 由 `security` 表达（`none`/`tls`/`reality`，不再有 `tls` 布尔字段）；
REALITY 必填 32 字节 `pbk`、`sid` 为不超过 16 位十六进制（可空）；`transport` 为 `tcp/ws/grpc`。

## 订阅

| 方法 | 路径 | Body |
|---|---|---|
| GET | `/api/subscriptions` | `{items:[{id,name,url,enabled,interval_sec,last_fetch_at,last_success_at,last_error,node_count,...}],total}` |
| POST | `/api/subscriptions` | `{name,url,enabled,interval_sec}`；创建后若启用会异步拉取一次 |
| PUT | `/api/subscriptions/{id}` | 同上（名称/地址/间隔/启用） |
| DELETE | `/api/subscriptions/{id}?delete_nodes=true` | 删除订阅；默认删除其同步且未被账户引用的节点 |
| POST | `/api/subscriptions/{id}/refresh` | 立即拉取并按节点差异更新，返回 `{result:{added,updated,removed,problems,error},saved}` |

- 订阅 URL 加密存储；拉取要求 http(s)，默认拒绝内网/回环/链路本地地址，限制 4 MiB、25 秒、最多 3 次重定向，支持 ETag/Last-Modified 304
- 本地订阅转换器需要访问 127.0.0.1/内网时设置 `SUB_ALLOW_PRIVATE=1`
- 刷新只增删该订阅映射的节点，手动上游与账户绑定不受影响；被账户引用的旧节点不会删除
- 拉取失败会写入 `last_error`，并在订阅列表展示

## 端口线路

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/ports` | `{port, remark}` → `{ok, saved:{port, default_account}}`，端口创建时自动生成其默认账号（`default-<port>`） |
| PUT | `/api/ports/{id}` | 同上（仅端口与备注） |
| PATCH | `/api/ports/{id}` | `{enabled}` |
| DELETE | `/api/ports/{id}` | 连带删除默认账号；若还有其他绑定账户则返回 `{error:"请先删除绑定账户 N 个"}` |

## 账户

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/accounts` | `{username, password, port_id, upstream_id, remark}`，`port_id` 必填 |
| PUT | `/api/accounts/{id}` | 同上（默认账号禁止更换端口；其余字段可改） |
| PATCH | `/api/accounts/{id}` | `{enabled}` |
| DELETE | `/api/accounts/{id}` | 默认账号返回 `{error:"默认账户不可删除，删除端口连带删除"}` |

`username` 全局唯一；`port_id` 必须指向已存在的端口；`upstream_id` 选填（空=直连）。
密码支持 `${VAR}` 环境变量引用，明文不落盘。账户的 `is_default` 由系统自动维护，无法在 POST/PUT 中伪造。

## 指标

| GET | `/api/metrics/hours` | `{points:[{t:"HH:04", conns, bytes_in, bytes_out}, ... 24 个}` | 近 24 小时按小时聚合的连接与流量（仪表盘面积图数据源） |
