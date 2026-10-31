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
log_retention_days(1-90), allow_direct, dial_timeout_sec(3-120), handshake_timeout_sec(10-300)`。

## 上游代理

| 方法 | 路径 | Body |
|---|---|---|
| POST | `/api/upstreams` | `{name, upstream:"http://user:pass@host:port"}` |
| PUT | `/api/upstreams/{id}` | 同上 |
| DELETE | `/api/upstreams/{id}` | 被引用时返回错误 |
| POST | `/api/upstreams/{id}/test` | 通过该上游 CONNECT 到 api.ipify.org:443，返回 `{ok, latency_ms, error?}` |
| POST | `/api/upstreams/import` | `{url}` 或 `{text}` → `{added,total,duplicates[],problems[]}` |

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
