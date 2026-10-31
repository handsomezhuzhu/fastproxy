export type Upstream = {
  id: string;
  name: string;
  type: string;
  host: string;
  username?: string;
  password?: string;
  remark?: string;
};

export type Port = {
  id: string;
  port: number;
  default_upstream: string;
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
};

export type Settings = {
  access_log: boolean;
  log_level: string;
  log_buffer: number;
  log_retention_days: number;
  allow_direct: boolean;
  dial_timeout_sec: number;
  handshake_timeout_sec: number;
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
};

export type ImportResult = {
  added: number;
  total: number;
  duplicates: string[];
  problems: string[];
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, {
    headers: { "Content-Type": "application/json" },
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
}

export const api = {
  session: () => req<{ auth: boolean; version: string }>("/api/session"),
  login: (password: string) =>
    req<{ ok: true }>("/api/login", { method: "POST", body: JSON.stringify({ password }) }),
  logout: () => req<{ ok: true }>("/api/logout", { method: "POST" }),
  data: () => req<Data>("/api/data"),

  saveSettings: (s: Settings) =>
    req<{ settings: Settings }>("/api/settings", { method: "PUT", body: JSON.stringify(s) }),

  createPort: (body: Partial<Port>) =>
    req<{ saved?: Port; warning?: string }>("/api/ports", { method: "POST", body: JSON.stringify(body) }),
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

  createUpstream: (body: { name: string; upstream: string; remark?: string }) =>
    req<{ saved?: Upstream; warning?: string }>("/api/upstreams", { method: "POST", body: JSON.stringify(body) }),
  updateUpstream: (id: string, body: { name: string; upstream: string; remark?: string }) =>
    req<{ saved?: Upstream; warning?: string }>(`/api/upstreams/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteUpstream: (id: string) => req<{ ok: true }>(`/api/upstreams/${id}`, { method: "DELETE" }),
  testUpstream: (id: string) =>
    req<{ ok: boolean; latency_ms: number; error?: string }>(`/api/upstreams/${id}/test`, {
      method: "POST",
    }),
  importUpstreams: (body: { text?: string; url?: string }) =>
    req<ImportResult>("/api/upstreams/import", { method: "POST", body: JSON.stringify(body) }),

  logs: (params: { limit?: number; port?: number; account?: string; level?: string }) => {
    const q = new URLSearchParams();
    if (params.limit) q.set("limit", String(params.limit));
    if (params.port) q.set("port", String(params.port));
    if (params.account) q.set("account", params.account);
    if (params.level) q.set("level", params.level);
    return req<{ entries: LogEntry[] }>(`/api/logs?${q.toString()}`);
  },

  stats: () =>
    req<{ status: { ports: PortStatus[]; accounts: AccountStatus[] } }>("/api/stats"),

  changePassword: (oldpwd: string, newpwd: string) =>
    req<{ ok: true }>("/api/password", {
      method: "POST",
      body: JSON.stringify({ old: oldpwd, new: newpwd }),
    }),
};
