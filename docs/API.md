# FastProxy API

所有管理接口使用 Cookie 会话（`POST /api/login` 后自动携带），要求 `Content-Type: application/json`。

## 会话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/session` | `{auth, version}` |
| POST | `/api/login` | `{password}` → 设置会话 Cookie |
| POST | `/api/logout` | 退出 |
| POST | `/api/password` | `{old, new}` 修改管理密码 |

## 数据

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/data` | `{version, settings, upstreams, ports, accounts, status:{ports,accounts}}` |
| GET | `/api/stats` | 仅统计 `{status}` |
| GET | `/api/logs?limit=&level=&port=&account=` | 连接日志（SQLite 查询），`level` 为最低级别 |
| GET | `/api/settings` | 全局设置 |
| PUT | `/api/settings` | 保存全局设置 |

设置字段：`access_log, log_level(debug|info|warn|error), log_buffer(100-10000),
log_retention_days(1-90), allow_direct, dial_timeout_sec(3-120), handshake_timeout_sec(10-300),
probe_enabled, probe_interval_sec(30-3600), probe_timeout_sec(3-30), probe_concurrency(1-32),
probe_target(域名)`。

## 上游代理

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/upstreams` | 见下方两种请求形状 |
| PUT | `/api/upstreams/{id}` | 同上 |
| DELETE | `/api/upstreams/{id}` | 被引用时返回错误 |
| POST | `/api/upstreams/{id}/test` | 立即探测该上游（健康 + 延迟 + 出口 IP），返回 `{ok, latency_ms, exit_ip, error?}` |
| POST | `/api/upstreams/{id}/check` | 同上（`/test` 为兼容别名） |
| POST | `/api/upstreams/check-all` | 检测全部上游与本机出口，返回 `{ok, failed, checked, server_ip}` |
| POST | `/api/upstreams/import` | `{url}` 或 `{text}` → `{added,total,duplicates[],problems[]}` |

`/api/data` 与 `GET /api/upstreams` 的 `upstreams[]` 字段：
`id, name, type, host, username?, password?, remark?, method?, uuid?, tls?, sni?`，
以及探测字段 `health(ok|fail|unknown), latency_ms, exit_ip, last_check_at(秒), last_error`；
另有 `probe:{server_ip,last_check_at}`。

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
    "uuid":"b831381d-6324-4d53-ad4f-8cda48b30811","tls":true,"sni":"cdn.example.com"}
   ```

`type` 取值：`http` | `socks5` | `shadowsocks` | `vless` | `vmess`（空值视为 `http`）。
校验规则：ss 必填 `method`（`aes-128-gcm`/`aes-256-gcm`/`chacha20-ietf-poly1305`）与 `password`；
vless/vmess 必填 `uuid`；`tls` 仅对 vless/vmess 生效；`host` 必须是 `host:port`。
`vmess` 可保存但出站隧道返回 `vmess 暂未支持`（探测显示为失败）。

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
