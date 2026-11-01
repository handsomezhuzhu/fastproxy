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
  tls?: boolean;
  sni?: string;
  health?: "ok" | "fail" | "unknown";
  latency_ms?: number;
  exit_ip?: string;
  last_check_at?: number;
  last_error?: string;
};

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
  tls?: boolean;
  sni?: string;
  remark?: string;
};

export type ProbeResult = {
  ok: boolean;
  latency_ms: number;
  exit_ip?: string;
  error?: string;
};

export type Port = {
  id: string;
  port: number;
  enabled: boolean;
  remark?: string;
};

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
  allow_direct: boolean;
  dial_timeout_sec: number;
  handshake_timeout_sec: number;
  probe_enabled: boolean;
  probe_interval_sec: number;
  probe_timeout_sec: number;
  probe_target: string;
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
  probe?: { server_ip: string; last_check_at: number };
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
  errors?: { id: string; error: string }[];
  skipped?: { id: string; reason: string }[];
  checks?: { id: string; ok: boolean; latency_ms?: number; error?: string }[];
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
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
      throw new ApiError(r.status, body?.error || `请求失败 (HTTP ${r.status})`);
    }
    return body as T;
  } catch (e: any) {
    if (e?.name === "AbortError") throw new ApiError(0, "请求超时（15 秒）");
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
  allow_direct: true,
  dial_timeout_sec: 15,
  handshake_timeout_sec: 60,
  probe_enabled: true,
  probe_interval_sec: 300,
  probe_timeout_sec: 8,
  probe_target: "api.ipify.org",
};

/** The API returns null for empty slices; normalize every collection. */
function normalizeData(d: Data): Data {
  return {
    ...d,
    version: d.version || "dev",
    upstreams: d.upstreams ?? [],
    ports: d.ports ?? [],
    accounts: d.accounts ?? [],
    settings: { ...defaultSettings, ...(d.settings ?? {}) },
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
  checkUpstream: (id: string) =>
    req<ProbeResult>(`/api/upstreams/${id}/check`, { method: "POST" }),
  checkAllUpstreams: () =>
    req<{ ok: number; failed: number; checked: number; server_ip: string }>(
      "/api/upstreams/check-all",
      { method: "POST" }
    ),
  importUpstreams: async (body: { text?: string; url?: string }) => {
    const r = await req<ImportResult>("/api/upstreams/import", {
      method: "POST",
      body: JSON.stringify(body),
    });
    return { ...r, duplicates: r.duplicates ?? [], problems: r.problems ?? [] };
  },

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
    req<BulkResult>("/api/accounts/bulk", { method: "POST", body: JSON.stringify({ ids, action }) }),
  bulkPorts: (ids: string[], action: "enable" | "disable" | "delete") =>
    req<BulkResult>("/api/ports/bulk", { method: "POST", body: JSON.stringify({ ids, action }) }),
  bulkUpstreams: (ids: string[], action: "delete" | "check") =>
    req<BulkResult>("/api/upstreams/bulk", { method: "POST", body: JSON.stringify({ ids, action }) }),

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
