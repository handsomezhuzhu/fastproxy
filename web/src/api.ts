/** VLESS/VMess 安全层：none（无加密）/ tls / reality（VLESS 专用）。v2 起 `tls` 字段已删除。 */
export type Security = "" | "none" | "tls" | "reality";
/** 传输层：tcp / ws / grpc。 */
export type Transport = "" | "tcp" | "ws" | "grpc";

/** sing-box 出站字段（VLESS REALITY / XTLS Vision / WS / gRPC、VMess），后端 JSON 用小写。 */
export type SingBoxFields = {
  security?: Security;
  pbk?: string; // REALITY public key
  sid?: string; // REALITY short id
  flow?: string; // "xtls-rprx-vision" 或空
  fingerprint?: string; // 默认 "chrome"
  transport?: Transport;
  path?: string; // ws path
  host_header?: string; // ws Host
  service_name?: string; // grpc service name
  vmess_security?: string; // 默认 "auto"
};

export type Upstream = {
  id: string;
  name: string;
  type: string;
  host: string;
  username?: string;
  password?: string;
  remark?: string;
  method?: string;
  uuid?: string;
  sni?: string;
  health?: "ok" | "fail" | "unknown";
  latency_ms?: number;
  exit_ip?: string;
  last_check_at?: number;
  last_error?: string;
} & SingBoxFields;

// UpstreamInput: http/socks5 走 upstream 地址；shadowsocks/vless/vmess 走字段。
export type UpstreamInput = {
  name: string;
  type?: string;
  upstream?: string;
  host?: string;
  username?: string;
  password?: string;
  method?: string;
  uuid?: string;
  sni?: string;
  remark?: string;
} & SingBoxFields;

/** POST /api/upstreams/parse response: the parsed share link, in UpstreamInput shape. */
export type ParseUpstreamResult = {
  proxy: UpstreamInput;
};

export type ProbeResult = {
  ok: boolean;
  latency_ms: number;
  exit_ip?: string;
  error?: string;
};

/** POST /api/upstreams/check 的单条结果。 */
export type CheckResult = {
  id: string;
  ok: boolean;
  latency_ms?: number;
  exit_ip?: string;
  error?: string;
};

/** POST /api/upstreams/check response：`ids` 为空时后端检测全部。 */
export type CheckResponse = {
  results: CheckResult[];
  ok: number;
  failed: number;
};

export type Port = {
  id: string;
  port: number;
  enabled: boolean;
  remark?: string;
};

/** 新建/编辑端口允许的号段，由容器环境变量 PORT_RANGE 决定（默认 10001-10050）。 */
export type PortRange = { min: number; max: number };

export const DEFAULT_PORT_RANGE: PortRange = { min: 10001, max: 10050 };

export type Account = {
  id: string;
  username: string;
  password: string;
  port_id: string;
  upstream_id: string;
  enabled: boolean;
  remark?: string;
  is_default?: boolean;
};

/** POST /api/ports response: creating a port also generates its default account. */
export type CreatePortResult = {
  ok?: boolean;
  saved?: {
    /** Newer backends return the full Port object; older ones returned the bare port number. */
    port?: Port | number;
    default_account?: Account;
  };
  warning?: string;
};

export type Settings = {
  access_log: boolean;
  log_level: string;
  log_buffer: number;
  log_retention_days: number;
  dial_timeout_sec: number;
  handshake_timeout_sec: number;
  probe_enabled: boolean;
  probe_interval_sec: number;
  probe_timeout_sec: number;
  probe_concurrency: number;
  probe_target: string;
  public_host?: string;
};

export type PortStatus = {
  id: string;
  port: number;
  enabled: boolean;
  listening: boolean;
  error?: string;
  conns_total: number;
  conns_cur: number;
  bytes_in: number;
  bytes_out: number;
};

export type AccountStatus = {
  id: string;
  username: string;
  port_id: string;
  upstream_id: string;
  enabled: boolean;
  conns_total: number;
  conns_cur: number;
  bytes_in: number;
  bytes_out: number;
};

export type LogEntry = {
  time: string;
  level: string;
  port?: number;
  account?: string;
  client?: string;
  proto?: string;
  target?: string;
  status?: string;
  bytes_in: number;
  bytes_out: number;
  duration_ms: number;
  msg?: string;
};

export type Data = {
  version: string;
  upstreams: Upstream[];
  ports: Port[];
  accounts: Account[];
  status: { ports: PortStatus[]; accounts: AccountStatus[] };
  settings: Settings;
  /** v2：端口号允许范围；旧后端没有时回落到容器默认值。 */
  port_range?: PortRange;
  probe?: { server_ip: string; last_check_at: number; singbox_error?: string };
};

/** GET /api/singbox/status（需登录）。接口不存在时状态卡降级显示。 */
export type SingboxStatus = {
  running: boolean;
  started_at?: number;
  reload_count?: number;
  last_reload_at?: number;
  last_error?: string;
  upstreams?: { id: string; name?: string; port?: number }[];
  logs?: string[];
};

export type Subscription = {
  id: string;
  name: string;
  url: string;
  enabled: boolean;
  interval_sec: number;
  last_fetch_at: number;
  last_success_at: number;
  last_error?: string;
  node_count: number;
  created_at: number;
  updated_at: number;
};

export type SubscriptionInput = {
  name: string;
  url: string;
  enabled: boolean;
  interval_sec: number;
};

export type SubscriptionRefresh = {
  result: {
    added: number;
    updated: number;
    removed: number;
    problems?: string[];
    error?: string;
  };
  saved?: Subscription | null;
};

export type MetricPoint = {
  t: string;
  conns: number;
  bytes_in: number;
  bytes_out: number;
};

export type ImportResult = {
  added: number;
  total: number;
  duplicates: string[];
  problems: string[];
};

/** Shared batch-operation payload (partial fields per action). */
export type BulkResult = {
  ok?: boolean;
  updated?: number;
  deleted?: number;
  errors?: { id: string; error: string; reason?: string }[];
  skipped?: { id: string; reason: string; error?: string }[];
  checks?: { id: string; ok: boolean; latency_ms?: number; error?: string }[];
};

function normalizeBulk(raw: any): BulkResult {
  const src = raw?.saved && typeof raw.saved === "object" ? { ...raw, ...raw.saved } : raw ?? {};
  const errors = Array.isArray(src.errors)
    ? src.errors.map((x: any) => ({
        id: String(x?.id ?? ""),
        error: String(x?.error ?? x?.reason ?? ""),
        reason: String(x?.reason ?? x?.error ?? ""),
      }))
    : undefined;
  const skipped = Array.isArray(src.skipped)
    ? src.skipped.map((x: any) => ({
        id: String(x?.id ?? ""),
        reason: String(x?.reason ?? x?.error ?? ""),
        error: String(x?.error ?? x?.reason ?? ""),
      }))
    : undefined;
  return {
    ok: src.ok ?? true,
    updated: typeof src.updated === "number" ? src.updated : undefined,
    deleted: typeof src.deleted === "number" ? src.deleted : undefined,
    errors,
    skipped,
    checks: src.checks,
  };
}

export class ApiError extends Error {
  status: number;
  /** 429 登录限流时后端返回的等待秒数（v2：`{error, retry_after_sec}`）。 */
  retryAfterSec?: number;
  constructor(status: number, message: string, retryAfterSec?: number) {
    super(message);
    this.status = status;
    this.retryAfterSec = retryAfterSec;
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);
  try {
    const r = await fetch(path, {
      headers: { "Content-Type": "application/json" },
      signal: controller.signal,
      ...init,
    });
    let body: any = {};
    try {
      body = await r.json();
    } catch {
      /* empty body */
    }
    if (!r.ok || body?.error) {
      const sec =
        typeof body?.retry_after_sec === "number"
          ? body.retry_after_sec
          : Number(r.headers?.get?.("Retry-After") || 0) || undefined;
      throw new ApiError(r.status, body?.error || `HTTP ${r.status}`, sec);
    }
    return body as T;
  } catch (e: any) {
    if (e?.name === "AbortError") throw new ApiError(0, "timeout");
    throw e;
  } finally {
    clearTimeout(timer);
  }
}

export const defaultSettings: Settings = {
  access_log: true,
  log_level: "info",
  log_buffer: 1000,
  log_retention_days: 7,
  dial_timeout_sec: 15,
  handshake_timeout_sec: 60,
  probe_enabled: true,
  probe_interval_sec: 300,
  probe_timeout_sec: 8,
  probe_concurrency: 6,
  probe_target: "api.ipify.org",
  public_host: "",
};

export type AccessHostOption = {
  value: string;
  isIp: boolean;
};

/**
 * 解析客户端接入主机列表：
 * - 默认优先使用本机探测到的公网 IP（data.probe.server_ip），避免管理面板经 Cloudflare 等 CDN 反代后把不可直连的面板域名当成代理主机。
 * - 同时合并系统设置中配置的自定义直连域名（settings.public_host，支持逗号/空格分隔多个），供快速复制向导切换。
 */
export function resolveAccessHosts(data: Data): {
  defaultHost: string;
  hosts: AccessHostOption[];
} {
  const serverIP = (data.probe?.server_ip ?? "").trim();
  const customRaw = (data.settings?.public_host ?? "").trim();
  const customList = customRaw
    ? customRaw
        .split(/[\s,;]+/)
        .map((s) => s.trim())
        .filter(Boolean)
    : [];

  const hosts: AccessHostOption[] = [];
  const seen = new Set<string>();
  if (serverIP) {
    hosts.push({ value: serverIP, isIp: true });
    seen.add(serverIP);
  }
  for (const h of customList) {
    if (!seen.has(h)) {
      hosts.push({ value: h, isIp: false });
      seen.add(h);
    }
  }
  if (hosts.length === 0) {
    const fallback = window.location.hostname || "127.0.0.1";
    hosts.push({ value: fallback, isIp: false });
  }
  return {
    defaultHost: hosts[0].value,
    hosts,
  };
}

/** The API returns null for empty slices; normalize every collection. */
function normalizeData(d: Data): Data {
  const pr = d.port_range;
  const port_range: PortRange =
    pr && Number(pr.min) > 0 && Number(pr.max) >= Number(pr.min)
      ? { min: Number(pr.min), max: Number(pr.max) }
      : { ...DEFAULT_PORT_RANGE };
  return {
    ...d,
    version: d.version || "dev",
    upstreams: d.upstreams ?? [],
    ports: d.ports ?? [],
    accounts: d.accounts ?? [],
    settings: { ...defaultSettings, ...(d.settings ?? {}) },
    port_range,
    status: {
      ports: d.status?.ports ?? [],
      accounts: d.status?.accounts ?? [],
    },
    probe: d.probe ?? { server_ip: "", last_check_at: 0 },
  };
}

export const api = {
  session: () => req<{ auth: boolean; version: string }>("/api/session"),
  login: (password: string) =>
    req<{ ok: true }>("/api/login", { method: "POST", body: JSON.stringify({ password }) }),
  logout: () => req<{ ok: true }>("/api/logout", { method: "POST" }),
  data: async () => normalizeData(await req<Data>("/api/data")),

  saveSettings: (s: Settings) =>
    req<{ settings: Settings }>("/api/settings", { method: "PUT", body: JSON.stringify(s) }),

  createPort: (body: Partial<Port>) =>
    req<CreatePortResult>("/api/ports", { method: "POST", body: JSON.stringify(body) }),
  updatePort: (id: string, body: Partial<Port>) =>
    req<{ saved?: Port; warning?: string }>(`/api/ports/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  togglePort: (id: string, enabled: boolean) =>
    req<{ saved?: Port; warning?: string }>(`/api/ports/${id}`, { method: "PATCH", body: JSON.stringify({ enabled }) }),
  deletePort: (id: string) => req<{ ok: true }>(`/api/ports/${id}`, { method: "DELETE" }),

  createAccount: (body: Partial<Account>) =>
    req<{ saved?: Account; warning?: string }>("/api/accounts", { method: "POST", body: JSON.stringify(body) }),
  updateAccount: (id: string, body: Partial<Account>) =>
    req<{ saved?: Account; warning?: string }>(`/api/accounts/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  toggleAccount: (id: string, enabled: boolean) =>
    req<{ saved?: Account; warning?: string }>(`/api/accounts/${id}`, { method: "PATCH", body: JSON.stringify({ enabled }) }),
  deleteAccount: (id: string) => req<{ ok: true }>(`/api/accounts/${id}`, { method: "DELETE" }),

  createUpstream: (body: UpstreamInput) =>
    req<{ saved?: Upstream; warning?: string }>("/api/upstreams", { method: "POST", body: JSON.stringify(body) }),
  updateUpstream: (id: string, body: UpstreamInput) =>
    req<{ saved?: Upstream; warning?: string }>(`/api/upstreams/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteUpstream: (id: string) => req<{ ok: true }>(`/api/upstreams/${id}`, { method: "DELETE" }),
  /**
   * v2 唯一检测入口：POST /api/upstreams/check `{ids}`（空数组 = 全部）。
   * 返回 `{results, ok, failed}`；旧版 `/{id}/check`、`/test`、`/check-all`、`/bulk` check 已删除。
   * 批量检测请按 id 分别调用（调用方控制并发），以便逐行实时刷新。
   */
  checkUpstreams: (ids: string[]) =>
    req<CheckResponse>("/api/upstreams/check", {
      method: "POST",
      body: JSON.stringify({ ids }),
    }).then((r) => ({
      results: r?.results ?? [],
      ok: r?.ok ?? (r?.results ?? []).filter((x) => x.ok).length,
      failed: r?.failed ?? (r?.results ?? []).filter((x) => !x.ok).length,
    })),
  importUpstreams: async (body: { text?: string; url?: string }) => {
    const r = await req<ImportResult>("/api/upstreams/import", {
      method: "POST",
      body: JSON.stringify(body),
    });
    return { ...r, duplicates: r.duplicates ?? [], problems: r.problems ?? [] };
  },

  /**
   * Parse one share link (vless://, vmess://, ss://, …) into form fields.
   *
   * Backend: POST /api/upstreams/parse `{text}` → `{proxy}`; failures answer
   * 400 `{error}` (surfaced as ApiError.message). Older backends without the
   * endpoint answer 404/405, which callers show as a plain form error.
   */
  parseUpstream: (text: string) =>
    req<ParseUpstreamResult>("/api/upstreams/parse", {
      method: "POST",
      body: JSON.stringify({ text }),
    }).then((r) => ({ proxy: r.proxy ?? ({} as UpstreamInput) })),

  /**
   * Server-paged upstream list; q fuzzy-matches name/host.
   *
   * The backend answers `{items, total, limit, offset}` (admin/server.go
   * handleListUpstreams). This used to read `r.entries`, so the list was always
   * empty and the page claimed "还没有上游代理". Keep the internal
   * `{entries, total}` shape for callers, and tolerate older/other payloads.
   */
  upstreamsList: async (params: { limit?: number; offset?: number; q?: string }) => {
    const q = new URLSearchParams();
    if (params.limit) q.set("limit", String(params.limit));
    if (params.offset) q.set("offset", String(params.offset));
    if (params.q) q.set("q", params.q);
    const r = await req<{ items?: Upstream[]; entries?: Upstream[]; total?: number }>(
      `/api/upstreams?${q.toString()}`
    );
    const entries = r.items ?? r.entries ?? [];
    return { entries, total: r.total ?? entries.length };
  },

  logs: (params: { limit?: number; offset?: number; port?: number; account?: string; level?: string }) => {
    const q = new URLSearchParams();
    if (params.limit) q.set("limit", String(params.limit));
    if (params.offset) q.set("offset", String(params.offset));
    if (params.port) q.set("port", String(params.port));
    if (params.account) q.set("account", params.account);
    if (params.level) q.set("level", params.level);
    return req<{ entries: LogEntry[]; total?: number }>(`/api/logs?${q.toString()}`).then((r) => ({
      entries: r.entries ?? [],
      total: r.total ?? (r.entries ?? []).length,
    }));
  },
  clearLogs: () => req<{ ok: true }>("/api/logs", { method: "DELETE" }),

  bulkAccounts: (ids: string[], action: "delete" | "enable" | "disable") =>
    req<any>("/api/accounts/bulk", { method: "POST", body: JSON.stringify({ ids, action }) }).then(normalizeBulk),
  bulkPorts: (ids: string[], action: "enable" | "disable" | "delete") =>
    req<any>("/api/ports/bulk", { method: "POST", body: JSON.stringify({ ids, action }) }).then(normalizeBulk),
  bulkUpstreams: (ids: string[], action: "delete") =>
    req<any>("/api/upstreams/bulk", { method: "POST", body: JSON.stringify({ ids, action }) }).then(normalizeBulk),

  /** 内置 sing-box 运行状态；旧后端没有该接口（404/405），调用方据此降级。 */
  singboxStatus: async () => {
    const r = await req<SingboxStatus>("/api/singbox/status");
    return {
      ...r,
      running: Boolean(r?.running),
      reload_count: Number(r?.reload_count ?? 0),
      last_reload_at: Number(r?.last_reload_at ?? 0),
      last_error: r?.last_error ?? "",
      upstreams: r?.upstreams ?? [],
      logs: r?.logs ?? [],
    };
  },

  subscriptions: () => req<{ items: Subscription[]; total: number }>("/api/subscriptions"),
  createSubscription: (body: SubscriptionInput) =>
    req<{ saved: Subscription }>("/api/subscriptions", { method: "POST", body: JSON.stringify(body) }),
  updateSubscription: (id: string, body: SubscriptionInput) =>
    req<{ saved: Subscription }>(`/api/subscriptions/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteSubscription: (id: string, deleteNodes = true) =>
    req<{ ok: boolean }>(
      `/api/subscriptions/${id}?delete_nodes=${deleteNodes ? "true" : "false"}`,
      { method: "DELETE" }
    ),
  refreshSubscription: (id: string) =>
    req<SubscriptionRefresh>(`/api/subscriptions/${id}/refresh`, { method: "POST" }),

  stats: async () => {
    const r = await req<{ status: { ports: PortStatus[]; accounts: AccountStatus[] } }>("/api/stats");
    return { status: { ports: r.status?.ports ?? [], accounts: r.status?.accounts ?? [] } };
  },

  /** Hourly metrics for the last 24 hours (may be empty before the first bucket). */
  metricsHours: () =>
    req<{ points: MetricPoint[] }>("/api/metrics/hours").then((r) => ({ points: r.points ?? [] })),

  changePassword: (oldpwd: string, newpwd: string) =>
    req<{ ok: true }>("/api/password", {
      method: "POST",
      body: JSON.stringify({ old: oldpwd, new: newpwd }),
    }),
};
