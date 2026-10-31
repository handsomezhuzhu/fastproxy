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
probe_enabled, probe_interval_sec(30-3600), probe_timeout_sec(3-30), probe_target(域名)`。

## 上游代理

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/upstreams` | `{name, upstream:"http://user:pass@host:port"}` |
| PUT | `/api/upstreams/{id}` | 同上 |
| DELETE | `/api/upstreams/{id}` | 被引用时返回错误 |
| POST | `/api/upstreams/{id}/test` | 立即探测该上游（健康 + 延迟 + 出口 IP），返回 `{ok, latency_ms, exit_ip, error?}` |
| POST | `/api/upstreams/{id}/check` | 同上（`/test` 为兼容别名） |
| POST | `/api/upstreams/check-all` | 检测全部上游与本机出口，返回 `{ok, failed, checked, server_ip}` |
| POST | `/api/upstreams/import` | `{url}` 或 `{text}` → `{added,total,duplicates[],problems[]}` |

`/api/data` 的 `upstreams[]` 含探测字段：`health(ok|fail|unknown), latency_ms, exit_ip,
last_check_at(秒), last_error`；另有 `probe:{server_ip,last_check_at}`。

## 端口线路

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/ports` | `{port, default_upstream, remark}` |
| PUT | `/api/ports/{id}` | 同上 |
| PATCH | `/api/ports/{id}` | `{enabled}` |
| DELETE | `/api/ports/{id}` | 绑定账户自动解绑为全局 |

## 账户

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/accounts` | `{username, password, port_id, upstream_id, remark}` |
| PUT | `/api/accounts/{id}` | 同上 |
| PATCH | `/api/accounts/{id}` | `{enabled}` |
| DELETE | `/api/accounts/{id}` | |

`username` 全局唯一；`password` 支持 `${VAR}` 环境变量引用。
