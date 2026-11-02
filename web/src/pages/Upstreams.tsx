import * as React from "react";
import {
  AlertTriangle,
  Ellipsis,
  Import,
  Layers,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  Zap,
} from "lucide-react";
import { toast } from "sonner";
import { api, type CheckResult, type Data, type ImportResult, type Upstream, type UpstreamInput } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  DotBadge,
  EmptyState,
  Field,
  FormSelect,
  PageHeader,
  PaginationControls,
  SelectAllCheckbox,
  Toolbar,
  useSelection,
} from "@/components/page";
import { useI18n } from "@/i18n";
import type { T } from "@/i18n";
import { explainError, explainErrorLine, explainProblem } from "@/lib/errors";
import { timeAgo } from "@/lib/utils";

function HealthCell({ u, checking }: { u: Upstream; checking?: boolean }) {
  const { t, lang } = useI18n();
  if (checking) {
    return (
      <DotBadge tone="muted">
        <Loader2 className="size-3 animate-spin" aria-hidden="true" />
        {t("upstreams.health.checking")}
      </DotBadge>
    );
  }
  if (!u.last_check_at) {
    return <DotBadge tone="muted">{t("upstreams.health.unknown")}</DotBadge>;
  }
  const checkedAt = new Date(u.last_check_at * 1000).toISOString();
  if (u.health === "ok") {
    return (
      <div className="space-y-0.5">
        <div className="flex items-center gap-1.5">
          <DotBadge tone="ok">{t("upstreams.health.ok")}</DotBadge>
          <span className="text-xs tabular-nums text-muted-foreground">{u.latency_ms ?? 0} ms</span>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("upstreams.health.checkedAt", { when: timeAgo(checkedAt, lang) })}
        </p>
      </div>
    );
  }
  const reason = u.last_error ? explainErrorLine(u.last_error, lang) : t("upstreams.health.failDefault");
  return (
    <div className="space-y-0.5">
      <DotBadge tone="fail" title={u.last_error || t("upstreams.health.failDefault")}>
        <AlertTriangle className="size-3" aria-hidden="true" />
        {t("upstreams.health.fail")}
      </DotBadge>
      <p className="max-w-[220px] truncate text-xs text-muted-foreground" title={u.last_error}>
        {reason} · {timeAgo(checkedAt, lang)}
      </p>
    </div>
  );
}

type BulkAction = "delete" | "check";

type UpstreamFormType = "http" | "socks5" | "shadowsocks" | "vless" | "vmess";

const TYPE_OPTIONS = [
  { value: "http", label: "HTTP" },
  { value: "socks5", label: "SOCKS5" },
  { value: "shadowsocks", label: "Shadowsocks" },
  { value: "vless", label: "VLESS" },
  { value: "vmess", label: "VMess" },
];

const TYPE_LABEL: Record<string, string> = {
  http: "HTTP",
  socks5: "SOCKS5",
  shadowsocks: "SS",
  vless: "VLESS",
  vmess: "VMess",
};

const SS_METHODS = [
  { value: "aes-256-gcm", label: "aes-256-gcm" },
  { value: "aes-128-gcm", label: "aes-128-gcm" },
  { value: "chacha20-ietf-poly1305", label: "chacha20-ietf-poly1305" },
];

type UpstreamForm = {
  name: string;
  type: UpstreamFormType;
  upstream: string;
  host: string;
  username: string;
  password: string;
  method: string;
  uuid: string;
  sni: string;
  remark: string;
  // sing-box 出站字段（VLESS / VMess）。v2 起 TLS 只由 security 表达，没有独立的 tls 字段。
  security: "none" | "tls" | "reality";
  pbk: string;
  sid: string;
  flow: string;
  fingerprint: string;
  transport: "tcp" | "ws" | "grpc";
  path: string;
  hostHeader: string;
  serviceName: string;
  vmessSecurity: string;
};

const EMPTY_FORM: UpstreamForm = {
  name: "",
  type: "http",
  upstream: "",
  host: "",
  username: "",
  password: "",
  method: "aes-256-gcm",
  uuid: "",
  sni: "",
  remark: "",
  security: "tls",
  pbk: "",
  sid: "",
  flow: "",
  fingerprint: "chrome",
  transport: "tcp",
  path: "",
  hostHeader: "",
  serviceName: "",
  vmessSecurity: "auto",
};

const isTunnelForm = (t: UpstreamFormType) => t === "shadowsocks" || t === "vless" || t === "vmess";

const SECURITY_OPTIONS = (t: T) => [
  { value: "none", label: t("upstreams.security.none") },
  { value: "tls", label: "TLS" },
  { value: "reality", label: "REALITY" },
];

const FINGERPRINT_OPTIONS = [
  { value: "chrome", label: "chrome" },
  { value: "firefox", label: "firefox" },
  { value: "safari", label: "safari" },
  { value: "ios", label: "ios" },
  { value: "android", label: "android" },
];

const FLOW_OPTIONS = (t: T) => [
  { value: "", label: t("upstreams.field.flowEmpty") },
  { value: "xtls-rprx-vision", label: "xtls-rprx-vision" },
];

const TRANSPORT_OPTIONS = [
  { value: "tcp", label: "TCP" },
  { value: "ws", label: "WebSocket" },
  { value: "grpc", label: "gRPC" },
];

const VMESS_SECURITY_OPTIONS = [
  { value: "auto", label: "auto" },
  { value: "none", label: "none" },
  { value: "zero", label: "zero" },
  { value: "aes-128-gcm", label: "aes-128-gcm" },
  { value: "chacha20-poly1305", label: "chacha20-poly1305" },
];

const asSecurity = (v: unknown): UpstreamForm["security"] =>
  v === "none" || v === "tls" || v === "reality" ? v : "tls";

const asTransport = (v: unknown): UpstreamForm["transport"] =>
  v === "tcp" || v === "ws" || v === "grpc" ? v : "tcp";

const str = (v: unknown) => (typeof v === "string" ? v : "");

/** List badges for a VLESS/VMess row: REALITY / Vision / WS / gRPC (monochrome outline). */
function RouteBadges({ u }: { u: Upstream }) {
  const t = useI18n().t;
  if (u.type !== "vless" && u.type !== "vmess") return null;
  const marks: string[] = [];
  if (u.type === "vless") {
    if (u.security === "reality" || u.pbk) marks.push(t("upstreams.route.reality"));
    if (u.flow) marks.push(t("upstreams.route.vision"));
  }
  if (u.transport === "ws") marks.push(t("upstreams.route.ws"));
  if (u.transport === "grpc") marks.push(t("upstreams.route.grpc"));
  if (marks.length === 0) return null;
  return (
    <>
      {marks.map((m) => (
        <Badge key={m} variant="outline" className="text-muted-foreground">
          {m}
        </Badge>
      ))}
    </>
  );
}

/** Address sub-line for VLESS/VMess: UUID + security layer + transport (uses `security`, never `tls`). */
function securityLabel(u: Upstream, t: T): string {
  if (u.type === "vless" && u.security === "reality") return t("upstreams.route.reality");
  if (u.security === "tls") return "TLS";
  return "";
}

export function UpstreamsPage({ data, reload }: { data: Data; reload: () => Promise<void> }) {
  const { t, lang } = useI18n();
  // Server-side pagination state
  const [list, setList] = React.useState<Upstream[] | null>(null);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(20);
  const [keyword, setKeyword] = React.useState("");
  const [q, setQ] = React.useState("");
  const [refreshing, setRefreshing] = React.useState(false);

  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Upstream | null>(null);
  const [form, setForm] = React.useState<UpstreamForm>(EMPTY_FORM);
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [shareLink, setShareLink] = React.useState("");
  const [parsing, setParsing] = React.useState(false);
  const [parseError, setParseError] = React.useState("");
  const [deleting, setDeleting] = React.useState<Upstream | null>(null);
  const [checking, setChecking] = React.useState<Record<string, boolean>>({});
  const [checkingAll, setCheckingAll] = React.useState(false);
  const [checkProgress, setCheckProgress] = React.useState({ done: 0, total: 0 });

  const [importOpen, setImportOpen] = React.useState(false);
  const [importText, setImportText] = React.useState("");
  const [importURL, setImportURL] = React.useState("");
  const [importing, setImporting] = React.useState(false);
  const [result, setResult] = React.useState<ImportResult | null>(null);

  // Bulk state
  const sel = useSelection();
  const [bulkBusy, setBulkBusy] = React.useState(false);
  const [bulkConfirmDelete, setBulkConfirmDelete] = React.useState(false);

  // 300ms 防抖搜索
  React.useEffect(() => {
    const timer = window.setTimeout(() => {
      setQ(keyword.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [keyword]);

  const load = React.useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!opts?.silent) setRefreshing(true);
      try {
        const r = await api.upstreamsList({
          limit: pageSize,
          offset: (page - 1) * pageSize,
          q: q || undefined,
        });
        setList(r.entries ?? []);
        setTotal(r.total ?? r.entries?.length ?? 0);
      } catch (err: any) {
        // 后端分页接口未上线/网络错误时不要整页崩溃，也保留已有列表（不清空）
        setList((prev) => prev ?? []);
        toast.error(err?.message || t("common.loadListFailed"));
      } finally {
        setRefreshing(false);
      }
    },
    [page, pageSize, q, t]
  );

  React.useEffect(() => {
    load();
  }, [load]);

  const refreshAll = React.useCallback(async () => {
    await load({ silent: true });
    await reload(); // /api/data 全量同步（统计卡、账户下拉等）
  }, [load, reload]);

  function openCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setError("");
    setParseError("");
    setShareLink("");
    setOpen(true);
  }
  function openEdit(u: Upstream) {
    setEditing(u);
    const type = (["http", "socks5", "shadowsocks", "vless", "vmess"] as const).includes(
      u.type as UpstreamFormType
    )
      ? (u.type as UpstreamFormType)
      : "http";
    const creds = u.username || u.password ? `${u.username}:${u.password}@` : "";
    setForm({
      ...EMPTY_FORM,
      name: u.name,
      type,
      upstream:
        type === "http" || type === "socks5" ? `${type}://${creds}${u.host}` : "",
      host: u.host || "",
      username: u.username || "",
      password: u.password || "",
      method: u.method || "aes-256-gcm",
      uuid: u.uuid || "",
      sni: u.sni || "",
      remark: u.remark || "",
      // 按类型回填 sing-box 字段（旧记录没有这些键时回落到默认值）
      security: asSecurity(u.security),
      pbk: u.pbk || "",
      sid: u.sid || "",
      flow: u.flow || "",
      fingerprint: u.fingerprint || "chrome",
      transport: asTransport(u.transport),
      path: u.path || "",
      hostHeader: u.host_header || "",
      serviceName: u.service_name || "",
      vmessSecurity: u.vmess_security || "auto",
    });
    setParseError("");
    setShareLink("");
    setError("");
    setOpen(true);
  }

  /**
   * 把 /api/upstreams/parse 返回的 proxy（UpstreamInput 形状）填进表单。
   * 分享链接里没有的字段保留当前值；type 按协议自动切换。
   */
  function applyParsed(p: UpstreamInput) {
    const rawType = str(p.type).toLowerCase();
    const type: UpstreamFormType | null = (
      ["http", "socks5", "shadowsocks", "vless", "vmess"] as const
    ).includes(rawType as UpstreamFormType)
      ? (rawType as UpstreamFormType)
      : null;
    const addrs = type === "http" || type === "socks5";
    const transport = p.transport ? asTransport(p.transport) : null;
    const security = p.security ? asSecurity(p.security) : null;
    setForm((f) => ({
      ...f,
      ...(type ? { type } : {}),
      name: str(p.name) || f.name,
      upstream: addrs
        ? str(p.upstream) ||
          (str(p.host)
            ? `${str(p.type).toLowerCase()}://${
                p.username || p.password ? `${str(p.username)}:${str(p.password)}@` : ""
              }${str(p.host)}`
            : f.upstream)
        : f.upstream,
      host: str(p.host) || f.host,
      username: str(p.username) || f.username,
      password: str(p.password) || f.password,
      method: str(p.method) || f.method,
      uuid: str(p.uuid) || f.uuid,
      sni: p.sni !== undefined ? str(p.sni) : f.sni,
      remark: p.remark !== undefined ? str(p.remark) : f.remark,
      ...(security ? { security } : {}),
      pbk: p.pbk !== undefined ? str(p.pbk) : f.pbk,
      sid: p.sid !== undefined ? str(p.sid) : f.sid,
      flow: p.flow !== undefined ? str(p.flow) : f.flow,
      fingerprint: str(p.fingerprint) || f.fingerprint,
      ...(transport ? { transport } : {}),
      path: p.path !== undefined ? str(p.path) : f.path,
      hostHeader: p.host_header !== undefined ? str(p.host_header) : f.hostHeader,
      serviceName: p.service_name !== undefined ? str(p.service_name) : f.serviceName,
      vmessSecurity: str(p.vmess_security) || f.vmessSecurity,
    }));
  }

  async function parseShareLink() {
    const text = shareLink.trim();
    if (!text) return;
    setParsing(true);
    setParseError("");
    setError("");
    try {
      const { proxy } = await api.parseUpstream(text);
      applyParsed(proxy);
      toast.success(t("upstreams.share.parsed"));
    } catch (err: any) {
      // 后端未合并该接口时会是 404/405；统一在表单里给出可读提示，不崩溃。
      const status = typeof err?.status === "number" ? err.status : 0;
      const msg =
        status === 404 || status === 405
          ? t("upstreams.share.unsupported")
          : err?.message || t("upstreams.share.parseFailed");
      setParseError(msg);
    } finally {
      setParsing(false);
    }
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const body: UpstreamInput = {
      name: form.name.trim(),
      type: form.type,
      remark: form.remark.trim(),
    };
    if (form.type === "http" || form.type === "socks5") {
      body.upstream = form.upstream.trim();
    } else if (form.type === "shadowsocks") {
      body.host = form.host.trim();
      body.method = form.method;
      body.password = form.password;
    } else if (form.type === "vless") {
      // REALITY 隐含 TLS，统一用 security 表达（v2 起不再发送 tls 字段）。
      const security = asSecurity(form.security);
      const tlsLike = security !== "none";
      body.host = form.host.trim();
      body.uuid = form.uuid.trim();
      body.security = security;
      body.sni = tlsLike ? form.sni.trim() : "";
      body.pbk = security === "reality" ? form.pbk.trim() : "";
      body.sid = security === "reality" ? form.sid.trim() : "";
      body.fingerprint = security === "reality" ? form.fingerprint || "chrome" : "";
      body.flow = form.flow;
      body.transport = form.transport;
      body.path = form.transport === "ws" ? form.path.trim() : "";
      body.host_header = form.transport === "ws" ? form.hostHeader.trim() : "";
      body.service_name = form.transport === "grpc" ? form.serviceName.trim() : "";
    } else {
      // VMess：TLS 开关写 security = "tls" / "none"。
      const security = form.security === "tls" ? "tls" : "none";
      body.host = form.host.trim();
      body.uuid = form.uuid.trim();
      body.security = security;
      body.sni = security === "tls" ? form.sni.trim() : "";
      body.transport = form.transport;
      body.path = form.transport === "ws" ? form.path.trim() : "";
      body.host_header = form.transport === "ws" ? form.hostHeader.trim() : "";
      body.service_name = form.transport === "grpc" ? form.serviceName.trim() : "";
      body.vmess_security = form.vmessSecurity || "auto";
    }
    try {
      if (editing) await api.updateUpstream(editing.id, body);
      else await api.createUpstream(body);
      toast.success(t("upstreams.saved"));
      setOpen(false);
      await refreshAll();
    } catch (err: any) {
      setError(err?.message || t("common.saveFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteUpstream(deleting.id);
      toast.success(t("upstreams.deleted", { name: deleting.name }));
      setDeleting(null);
      await refreshAll();
    } catch (err: any) {
      toast.error(err?.message || t("common.deleteFailed"));
    }
  }

  // Merge one check result into the visible list immediately (no full reload),
  // so a concurrent bulk check updates rows as each result arrives.
  function applyCheckResult(r: CheckResult) {
    setList((prev) =>
      prev
        ? prev.map((u) =>
            u.id === r.id
              ? {
                  ...u,
                  health: r.ok ? ("ok" as const) : ("fail" as const),
                  latency_ms: r.ok ? r.latency_ms ?? 0 : 0,
                  exit_ip: r.ok ? r.exit_ip || "" : "",
                  last_error: r.ok ? "" : r.error || t("upstreams.health.failDefault"),
                  last_check_at: Math.floor(Date.now() / 1000),
                }
              : u
          )
        : prev
    );
  }

  /** v2 统一检测：每个 id 单独 POST /api/upstreams/check {ids:[id]}，完成即写回行。返回单条结果。 */
  async function runCheck(id: string): Promise<CheckResult> {
    setChecking((c) => ({ ...c, [id]: true }));
    try {
      const { results } = await api.checkUpstreams([id]);
      const r = results.find((x) => x.id === id) ?? results[0];
      const out: CheckResult = r
        ? { ...r, id }
        : { id, ok: false, error: t("upstreams.health.failDefault") };
      applyCheckResult(out);
      return out;
    } catch (err: any) {
      // 404/405：后端尚未合并统一检测接口，向上抛出，由调用方统一提示一次。
      if (err?.status === 404 || err?.status === 405) throw err;
      const out: CheckResult = { id, ok: false, error: err?.message || t("upstreams.health.failDefault") };
      applyCheckResult(out);
      return out;
    } finally {
      setChecking((c) => ({ ...c, [id]: false }));
    }
  }

  async function checkSingleRow(u: Upstream) {
    try {
      const r = await runCheck(u.id);
      if (r.ok) {
        toast.success(
          t("upstreams.checkOk", {
            name: u.name,
            ms: r.latency_ms ?? 0,
            ip: r.exit_ip || t("upstreams.unknownProvince"),
          })
        );
      } else {
        toast.error(t("upstreams.checkFail", { name: u.name, err: explainErrorLine(r.error, lang) }));
      }
      await reload();
    } catch (err: any) {
      if (err?.status === 404 || err?.status === 405) {
        toast.error(t("upstreams.checkUnsupported"));
        return;
      }
      toast.error(err?.message || t("upstreams.checkFailed"));
      await reload();
    }
  }

  // 批量并发检测：并发数取 settings.probe_concurrency（1-32），每个 id 一个请求；
  // 每完成一个立刻写回对应行并推进进度，不等全部完成。
  async function checkInBatch(ids: string[]) {
    if (ids.length === 0) {
      toast.info(t("upstreams.checkNoTarget"));
      return;
    }
    const concurrency = Math.min(
      Math.max(1, Math.floor(Number(data.settings.probe_concurrency) || 6)),
      32,
      ids.length
    );
    setBulkBusy(true);
    setCheckingAll(true);
    setCheckProgress({ done: 0, total: ids.length });
    const queue = [...ids];
    let ok = 0;
    let fail = 0;
    let unsupported = false;
    const worker = async () => {
      for (;;) {
        if (unsupported) return;
        const id = queue.shift();
        if (!id) return;
        try {
          const good = await runCheck(id);
          if (good.ok) ok++;
          else fail++;
        } catch (err: any) {
          // 统一检测接口不存在：停止后续请求，保留已完成的结果。
          if (err?.status === 404 || err?.status === 405) {
            unsupported = true;
            return;
          }
          fail++;
        } finally {
          setCheckProgress((p) => ({ done: Math.min(p.done + 1, p.total), total: p.total }));
        }
      }
    };
    await Promise.all(Array.from({ length: Math.min(concurrency, ids.length) }, () => worker()));
    setBulkBusy(false);
    setCheckingAll(false);
    if (unsupported) {
      toast.error(t("upstreams.checkUnsupported"));
    } else if (fail > 0) {
      toast.warning(t("upstreams.checkDoneMixed", { ok, fail }));
    } else {
      toast.success(t("upstreams.checkDoneOk", { ok }));
    }
    // 全量同步一次（统计卡、健康数等），行内结果已经在过程中写回。
    await reload();
  }

  async function checkAll() {
    try {
      const all = await api.upstreamsList({ limit: 1000, offset: 0 });
      await checkInBatch(all.entries.map((u) => u.id));
    } catch (err: any) {
      toast.error(err?.message || t("upstreams.checkFailed"));
    }
  }

  async function runImport(useURL: boolean) {
    setImporting(true);
    setResult(null);
    try {
      const r = await api.importUpstreams(useURL ? { url: importURL } : { text: importText });
      setResult(r);
      if (r.added > 0) {
        toast.success(t("upstreams.import.ok", { n: r.added }));
        await refreshAll();
      } else {
        toast.warning(t("upstreams.import.none"));
      }
    } catch (err: any) {
      toast.error(err?.message || t("upstreams.import.failed"));
    } finally {
      setImporting(false);
    }
  }

  async function runBulk(action: BulkAction) {
    const ids = [...sel.selected];
    if (ids.length === 0) return;
    if (action === "check") {
      await checkInBatch(ids);
      sel.clear();
      setBulkConfirmDelete(false);
      return;
    }
    setBulkBusy(true);
    try {
      const r = await api.bulkUpstreams(ids, action);
      const skipped = r.skipped ?? [];
      const deleted = r.deleted ?? Math.max(0, ids.length - skipped.length);
      if (skipped.length > 0) {
        toast.error(t("upstreams.bulkSkipped", { n: skipped.length, reason: skipped[0].reason }));
        if (deleted > 0) toast.success(t("upstreams.bulkDeleted", { n: deleted }));
      } else {
        toast.success(t("upstreams.bulkDeleted", { n: deleted }));
      }
      sel.clear();
      await refreshAll();
    } catch (err: any) {
      toast.error(err?.message || t("common.bulkFailed"));
    } finally {
      setBulkBusy(false);
      setBulkConfirmDelete(false);
    }
  }

  function askBulk(action: BulkAction) {
    if (action === "delete") setBulkConfirmDelete(true);
    else runBulk("check");
  }

  const refs = (id: string) => data.accounts.filter((a) => a.upstream_id === id).length;

  const pageIDs = (list ?? []).map((u) => u.id);
  const pageAllSelected = pageIDs.length > 0 && pageIDs.every((id) => sel.selected.has(id));
  const pageSomeSelected = pageIDs.some((id) => sel.selected.has(id));

  // 首次加载：Skeleton 覆盖工具栏与表格
  if (list === null) {
    return (
      <div className="space-y-5">
        <PageHeader title={t("upstreams.title")} description={t("upstreams.description")} />
        <div className="flex items-center justify-between gap-3">
          <Skeleton className="h-5 w-32" />
          <div className="flex gap-2">
            <Skeleton className="h-7 w-40" />
            <Skeleton className="h-7 w-20" />
            <Skeleton className="h-7 w-20" />
          </div>
        </div>
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </div>
    );
  }

  const showEmpty = total === 0 && !q;

  return (
    <div className="space-y-5">
      <PageHeader title={t("upstreams.title")} description={t("upstreams.description")} />

      {showEmpty ? (
        <EmptyState
          icon={<Layers className="size-5" aria-hidden="true" />}
          title={t("upstreams.empty.title")}
          description={t("upstreams.empty.desc")}
          action={
            <div className="flex flex-wrap items-center justify-center gap-2">
              <Button variant="outline" onClick={() => setImportOpen(true)}>
                <Import className="size-4" aria-hidden="true" /> {t("upstreams.import")}
              </Button>
              <Button onClick={openCreate}>
                <Plus className="size-4" aria-hidden="true" /> {t("upstreams.add")}
              </Button>
            </div>
          }
        />
      ) : (
        <>
          <Toolbar
            count={t("upstreams.count", { n: total })}
            detail={
              q
                ? t("upstreams.filteredBy", { q })
                : refreshing
                  ? t("upstreams.refreshing")
                  : undefined
            }
          >
            <div className="relative">
              <Search
                className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                className="h-7 w-44 pl-8 text-xs"
                placeholder={t("upstreams.searchPlaceholder")}
                aria-label={t("upstreams.searchLabel")}
              />
            </div>
            <Button variant="outline" size="sm" onClick={checkAll} disabled={checkingAll || bulkBusy}>
              {checkingAll ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <RefreshCw className="size-4" aria-hidden="true" />
              )}
              <span className="inline-block min-w-20 text-center tabular-nums">
                {checkingAll
                  ? t("upstreams.checking", { done: checkProgress.done, total: checkProgress.total })
                  : t("upstreams.checkAll")}
              </span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setImportOpen(true);
                setResult(null);
              }}
            >
              <Import className="size-4" aria-hidden="true" /> {t("upstreams.import")}
            </Button>
            <Button size="sm" onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> {t("upstreams.add")}
            </Button>
          </Toolbar>

          {sel.selected.size > 0 ? (
            <div
              className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2"
              role="status"
            >
              <span className="text-sm tabular-nums">
                {checkingAll
                  ? `${t("upstreams.checking", { done: checkProgress.done, total: checkProgress.total })} ·`
                  : `${t("common.selected", { n: sel.selected.size })} ·`}
              </span>
              <Button size="sm" variant="outline" onClick={() => askBulk("check")} disabled={bulkBusy}>
                {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-8 text-center">{t("upstreams.checkSelected")}</span>
              </Button>
              <Button size="sm" variant="destructive" onClick={() => askBulk("delete")} disabled={bulkBusy}>
                {t("common.delete")}
              </Button>
              <Button size="sm" variant="ghost" onClick={sel.clear} aria-label={t("common.clearSelection")}>
                {t("common.cancel")}
              </Button>
            </div>
          ) : null}

          {(list ?? []).length === 0 ? (
            <EmptyState
              icon={<Search className="size-5" aria-hidden="true" />}
              title={t("upstreams.noMatch.title")}
              description={t("upstreams.noMatch.desc")}
              action={
                <Button size="sm" variant="outline" onClick={() => setKeyword("")}>
                  {t("upstreams.clearSearch")}
                </Button>
              }
            />
          ) : (
            <>
              <div className="overflow-hidden rounded-xl border">
                <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <SelectAllCheckbox
                        checked={pageAllSelected}
                        indeterminate={!pageAllSelected && pageSomeSelected}
                        onCheckedChange={(v) => sel.toggleMany(pageIDs, v)}
                        label={t("common.selectAllPage")}
                      />
                    </TableHead>
                    <TableHead>{t("upstreams.col.name")}</TableHead>
                    <TableHead>{t("upstreams.col.type")}</TableHead>
                    <TableHead>{t("upstreams.col.address")}</TableHead>
                    <TableHead>{t("upstreams.col.health")}</TableHead>
                    <TableHead>{t("upstreams.col.exitIp")}</TableHead>
                    <TableHead className="text-right">{t("upstreams.col.refs")}</TableHead>
                    <TableHead className="w-12">
                      <span className="sr-only">{t("common.actions")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(list ?? []).map((u) => {
                    const checked = sel.selected.has(u.id);
                    return (
                      <TableRow key={u.id} data-state={checked ? "selected" : undefined}>
                        <TableCell>
                          <input
                            type="checkbox"
                            className="size-3.5 accent-primary focus-visible:outline-2 focus-visible:outline-ring"
                            checked={checked}
                            onChange={(e) => sel.toggle(u.id, e.target.checked)}
                            aria-label={t("upstreams.selectRow", { name: u.name })}
                          />
                        </TableCell>
                        <TableCell>
                          <div className="font-medium">{u.name}</div>
                          {u.remark ? (
                            <div className="max-w-[160px] truncate text-xs text-muted-foreground">{u.remark}</div>
                          ) : null}
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap items-center gap-1.5">
                            <Badge variant="outline">{TYPE_LABEL[u.type] ?? u.type}</Badge>
                            <RouteBadges u={u} />
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="font-mono text-xs">{u.host}</div>
                          {u.type === "shadowsocks" ? (
                            <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
                              {u.method || t("upstreams.methodUnset")}
                            </div>
                          ) : u.type === "vless" || u.type === "vmess" ? (
                            <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
                              {u.uuid
                                ? `UUID ${u.uuid.slice(0, 8)}…`
                                : t("upstreams.uuidUnset")}
                              {securityLabel(u, t) ? ` · ${securityLabel(u, t)}` : ""}
                              {u.transport && u.transport !== "tcp"
                                ? ` · ${u.transport === "ws" ? "WS" : "gRPC"} ${u.transport === "ws" ? u.path || "/" : u.service_name || ""}`.trimEnd()
                                : ""}
                            </div>
                          ) : u.username ? (
                            <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
                              {t("upstreams.userPrefix", { name: u.username })}
                            </div>
                          ) : null}
                        </TableCell>
                        <TableCell>
                          <HealthCell u={u} checking={!!checking[u.id]} />
                        </TableCell>
                        <TableCell>
                          {u.exit_ip ? (
                            <span className="font-mono text-xs tabular-nums">{u.exit_ip}</span>
                          ) : (
                            <span className="text-xs text-muted-foreground">–</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right text-xs tabular-nums">{refs(u.id)}</TableCell>
                        <TableCell>
                          <DropdownMenu>
                            <DropdownMenuTrigger
                              className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                              aria-label={t("upstreams.rowMenu", { name: u.name })}
                            >
                              {checking[u.id] ? (
                                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                              ) : (
                                <Ellipsis className="size-4" aria-hidden="true" />
                              )}
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-40">
                              <DropdownMenuItem onClick={() => checkSingleRow(u)} disabled={checking[u.id]}>
                                <Zap className="size-4" aria-hidden="true" /> {t("upstreams.menu.check")}
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => openEdit(u)}>
                                <Pencil className="size-4" aria-hidden="true" /> {t("common.edit")}
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem variant="destructive" onClick={() => setDeleting(u)}>
                                <Trash2 className="size-4" aria-hidden="true" /> {t("common.delete")}
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
                </Table>
              </div>
              <PaginationControls
                page={page}
                pageSize={pageSize}
                total={total}
                onChange={(p, s) => {
                  setPage(p);
                  setPageSize(s);
                }}
              />
            </>
          )}
        </>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editing ? t("upstreams.dialog.edit", { name: editing.name }) : t("upstreams.dialog.create")}
            </DialogTitle>
            <DialogDescription>{t("upstreams.dialog.desc")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <Field label={t("upstreams.field.name")} htmlFor="up-name">
                <Input
                  id="up-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder={t("upstreams.field.namePlaceholder")}
                />
              </Field>
              <Field label={t("upstreams.field.type")} htmlFor="up-type">
                <FormSelect
                  id="up-type"
                  value={form.type}
                  onValueChange={(v) => setForm({ ...form, type: v as UpstreamFormType })}
                  items={TYPE_OPTIONS}
                />
              </Field>
            </div>

            {!editing ? (
              <div className="space-y-1.5 rounded-lg border bg-muted/30 px-3 py-2">
                <Label htmlFor="up-share">{t("upstreams.share.label")}</Label>
                <div className="flex gap-2">
                  <Input
                    id="up-share"
                    className="font-mono text-xs"
                    value={shareLink}
                    onChange={(e) => setShareLink(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        if (!parsing) parseShareLink();
                      }
                    }}
                    placeholder="vless://uuid@1.2.3.4:443?security=reality&pbk=…&sid=…&sni=…#name"
                    spellCheck={false}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={parseShareLink}
                    disabled={parsing || !shareLink.trim()}
                    className="shrink-0"
                  >
                    {parsing ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                    {t("upstreams.share.parse")}
                  </Button>
                </div>
                {parseError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {parseError}
                  </p>
                ) : (
                  <p className="text-xs text-muted-foreground">{t("upstreams.share.hint")}</p>
                )}
              </div>
            ) : null}

            {form.type === "http" || form.type === "socks5" ? (
              <>
                <Field
                  label={t("upstreams.field.address")}
                  htmlFor="up-address"
                  hint={t("upstreams.field.addressHint")}
                >
                  <Input
                    id="up-address"
                    className="font-mono"
                    value={form.upstream}
                    onChange={(e) => setForm({ ...form, upstream: e.target.value })}
                    placeholder={`${form.type}://user:pass@1.2.3.4:${form.type === "http" ? "8080" : "1080"}`}
                    spellCheck={false}
                  />
                </Field>
                <p className="text-xs text-muted-foreground">{t("upstreams.field.addressPaste")}</p>
              </>
            ) : null}

            {isTunnelForm(form.type) ? (
              <Field label={t("upstreams.field.host")} htmlFor="up-host" hint={t("upstreams.field.hostHint")}>
                <Input
                  id="up-host"
                  className="font-mono"
                  value={form.host}
                  onChange={(e) => setForm({ ...form, host: e.target.value })}
                  placeholder="1.2.3.4:8388"
                  spellCheck={false}
                />
              </Field>
            ) : null}

            {form.type === "shadowsocks" ? (
              <div className="grid grid-cols-2 gap-3">
                <Field label={t("upstreams.field.method")} htmlFor="up-method">
                  <FormSelect
                    id="up-method"
                    value={form.method}
                    onValueChange={(v) => setForm({ ...form, method: v })}
                    items={SS_METHODS}
                  />
                </Field>
                <Field label={t("common.password")} htmlFor="up-ss-password">
                  <Input
                    id="up-ss-password"
                    type="password"
                    value={form.password}
                    onChange={(e) => setForm({ ...form, password: e.target.value })}
                    autoComplete="new-password"
                  />
                </Field>
              </div>
            ) : null}

            {form.type === "vless" || form.type === "vmess" ? (
              <div className="grid grid-cols-2 gap-3">
                <Field label={t("upstreams.field.uuid")} htmlFor="up-uuid">
                  <Input
                    id="up-uuid"
                    className="font-mono"
                    value={form.uuid}
                    onChange={(e) => setForm({ ...form, uuid: e.target.value })}
                    placeholder="b831381d-6324-4d53-ad4f-8cda48b30811"
                    spellCheck={false}
                  />
                </Field>
                <Field label={t("upstreams.field.transport")} htmlFor="up-transport">
                  <FormSelect
                    id="up-transport"
                    value={form.transport}
                    onValueChange={(v) => setForm({ ...form, transport: v as UpstreamForm["transport"] })}
                    items={TRANSPORT_OPTIONS}
                  />
                </Field>
              </div>
            ) : null}

            {form.type === "vless" ? (
              <>
                <div className="grid grid-cols-2 gap-3">
                  <Field label={t("upstreams.field.security")} htmlFor="up-security">
                    <FormSelect
                      id="up-security"
                      value={form.security}
                      onValueChange={(v) => setForm({ ...form, security: asSecurity(v) })}
                      items={SECURITY_OPTIONS(t)}
                    />
                  </Field>
                  <Field label={t("upstreams.field.flow")} htmlFor="up-flow" hint={t("upstreams.field.flowHint")}>
                    <FormSelect
                      id="up-flow"
                      value={form.flow}
                      onValueChange={(v) => setForm({ ...form, flow: v })}
                      items={
                        FLOW_OPTIONS(t).some((o) => o.value === form.flow)
                          ? FLOW_OPTIONS(t)
                          : [...FLOW_OPTIONS(t), { value: form.flow, label: form.flow }]
                      }
                    />
                  </Field>
                </div>

                {form.security === "reality" ? (
                  <div className="grid grid-cols-3 gap-3">
                    <Field label={t("upstreams.field.pbk")} htmlFor="up-pbk">
                      <Input
                        id="up-pbk"
                        className="font-mono"
                        value={form.pbk}
                        onChange={(e) => setForm({ ...form, pbk: e.target.value })}
                        placeholder={t("upstreams.field.pbkPlaceholder")}
                        spellCheck={false}
                      />
                    </Field>
                    <Field label={t("upstreams.field.sid")} htmlFor="up-sid">
                      <Input
                        id="up-sid"
                        className="font-mono"
                        value={form.sid}
                        onChange={(e) => setForm({ ...form, sid: e.target.value })}
                        placeholder="sid"
                        spellCheck={false}
                      />
                    </Field>
                    <Field label={t("upstreams.field.fingerprint")} htmlFor="up-fingerprint">
                      <FormSelect
                        id="up-fingerprint"
                        value={form.fingerprint}
                        onValueChange={(v) => setForm({ ...form, fingerprint: v })}
                        items={FINGERPRINT_OPTIONS}
                      />
                    </Field>
                  </div>
                ) : (
                  <div className="flex items-center justify-between rounded-lg border px-3 py-2">
                    <div>
                      <div className="text-sm font-medium">{t("upstreams.field.tls")}</div>
                      <p className="text-xs text-muted-foreground">{t("upstreams.field.tlsHint")}</p>
                    </div>
                    <Switch
                      checked={form.security === "tls"}
                      onCheckedChange={(v) =>
                        setForm({ ...form, security: v ? "tls" : "none" })
                      }
                      aria-label={t("upstreams.field.tls")}
                    />
                  </div>
                )}
              </>
            ) : null}

            {form.type === "vmess" ? (
              <div className="grid grid-cols-2 gap-3">
                <Field label={t("upstreams.field.vmessSecurity")} htmlFor="up-vmess-security">
                  <FormSelect
                    id="up-vmess-security"
                    value={form.vmessSecurity}
                    onValueChange={(v) => setForm({ ...form, vmessSecurity: v })}
                    items={VMESS_SECURITY_OPTIONS}
                  />
                </Field>
                <div className="flex items-center justify-between rounded-lg border px-3 py-2">
                  <div>
                    <div className="text-sm font-medium">{t("upstreams.field.tls")}</div>
                    <p className="text-xs text-muted-foreground">{t("upstreams.field.vmessTlsHint")}</p>
                  </div>
                  <Switch
                    checked={form.security === "tls"}
                    onCheckedChange={(v) => setForm({ ...form, security: v ? "tls" : "none" })}
                    aria-label={t("upstreams.field.tls")}
                  />
                </div>
              </div>
            ) : null}

            {form.type === "vless" || form.type === "vmess" ? (
              <>
                {form.security !== "none" ? (
                  <Field label={t("upstreams.field.sni")} htmlFor="up-sni" hint={t("upstreams.field.sniHint")}>
                    <Input
                      id="up-sni"
                      className="font-mono"
                      value={form.sni}
                      onChange={(e) => setForm({ ...form, sni: e.target.value })}
                      placeholder="cdn.example.com"
                      spellCheck={false}
                    />
                  </Field>
                ) : null}

                {form.transport === "ws" ? (
                  <div className="grid grid-cols-2 gap-3">
                    <Field label={t("upstreams.field.wsPath")} htmlFor="up-path" hint={t("upstreams.field.wsPathHint")}>
                      <Input
                        id="up-path"
                        className="font-mono"
                        value={form.path}
                        onChange={(e) => setForm({ ...form, path: e.target.value })}
                        placeholder="/ws"
                        spellCheck={false}
                      />
                    </Field>
                    <Field label={t("upstreams.field.wsHost")} htmlFor="up-host-header" hint={t("upstreams.field.wsHostHint")}>
                      <Input
                        id="up-host-header"
                        className="font-mono"
                        value={form.hostHeader}
                        onChange={(e) => setForm({ ...form, hostHeader: e.target.value })}
                        placeholder="cdn.example.com"
                        spellCheck={false}
                      />
                    </Field>
                  </div>
                ) : null}

                {form.transport === "grpc" ? (
                  <Field label={t("upstreams.field.grpcService")} htmlFor="up-service-name" hint={t("upstreams.field.grpcServiceHint")}>
                    <Input
                      id="up-service-name"
                      className="font-mono"
                      value={form.serviceName}
                      onChange={(e) => setForm({ ...form, serviceName: e.target.value })}
                      placeholder="grpc-service"
                      spellCheck={false}
                    />
                  </Field>
                ) : null}
              </>
            ) : null}

            <Field label={t("common.remark")} htmlFor="up-remark">
              <Input
                id="up-remark"
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
              />
            </Field>
            {error ? (
              <p className="text-xs text-destructive" role="alert">
                {error}
              </p>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-16 text-center">
                  {busy ? t("common.saving") : t("common.save")}
                </span>
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("upstreams.import.title")}</DialogTitle>
            <DialogDescription>{t("upstreams.import.desc")}</DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="text">
            <TabsList>
              <TabsTrigger value="text">{t("upstreams.import.tabText")}</TabsTrigger>
              <TabsTrigger value="url">{t("upstreams.import.tabUrl")}</TabsTrigger>
            </TabsList>

            <TabsContent value="text" className="space-y-2 pt-2">
              <Textarea
                aria-label={t("upstreams.import.textLabel")}
                className="min-h-[160px] font-mono text-xs"
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
                placeholder={"http://user:pass@1.2.3.4:8080#US-01\nsocks5://1.2.3.4:1080\nvless://uuid@1.2.3.4:443?security=reality&pbk=…&sid=…&flow=xtls-rprx-vision&sni=www.apple.com#REALITY-01\n1.2.3.4:8080"}
              />
              <Button
                className="w-full"
                disabled={importing || !importText.trim()}
                onClick={() => runImport(false)}
              >
                {importing ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-16 text-center">
                  {importing ? t("upstreams.import.importing") : t("upstreams.import.importText")}
                </span>
              </Button>
            </TabsContent>

            <TabsContent value="url" className="space-y-2 pt-2">
              <Input
                aria-label={t("upstreams.import.urlLabel")}
                className="font-mono text-xs"
                value={importURL}
                onChange={(e) => setImportURL(e.target.value)}
                placeholder="https://example.com/sub?token=…"
                spellCheck={false}
              />
              <Button
                className="w-full"
                disabled={importing || !importURL.trim()}
                onClick={() => runImport(true)}
              >
                {importing ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-16 text-center">
                  {importing ? t("upstreams.import.fetching") : t("upstreams.import.fetchImport")}
                </span>
              </Button>
            </TabsContent>
          </Tabs>

          {result ? (
            <div className="space-y-2 rounded-lg border bg-muted/30 p-3 text-xs" aria-live="polite">
              <div className="flex flex-wrap gap-2">
                <Badge variant="secondary" className="bg-foreground text-background">
                  {t("upstreams.import.added", { n: result.added })}
                </Badge>
                <Badge variant="secondary" className="bg-muted text-foreground">
                  {t("upstreams.import.duplicates", { n: result.duplicates.length })}
                </Badge>
                <Badge variant="secondary" className="border border-foreground/40 font-medium text-foreground">
                  {t("upstreams.import.problems", { n: result.problems.length })}
                </Badge>
                <Badge variant="outline">{t("upstreams.import.total", { n: result.total })}</Badge>
              </div>
              {result.duplicates.length > 0 ? (
                <details>
                  <summary className="cursor-pointer text-muted-foreground">{t("upstreams.import.dupTitle")}</summary>
                  <ul className="mt-1 max-h-24 overflow-y-auto font-mono">
                    {result.duplicates.map((d, i) => (
                      <li key={i}>{d}</li>
                    ))}
                  </ul>
                </details>
              ) : null}
              {result.problems.length > 0 ? (
                <details>
                  <summary className="cursor-pointer text-muted-foreground">{t("upstreams.import.problemTitle")}</summary>
                  <ul className="mt-1 max-h-32 space-y-1 overflow-y-auto">
                    {result.problems.map((d, i) => {
                      const hint = explainProblem(d, lang);
                      return (
                        <li key={i} className="space-y-0.5">
                          <p className="text-foreground">{hint.message}</p>
                          {hint.hint ? <p className="text-muted-foreground">{hint.hint}</p> : null}
                        </li>
                      );
                    })}
                  </ul>
                </details>
              ) : null}
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={() => setImportOpen(false)}>
              {t("common.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("upstreams.delete.title", { name: deleting?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("upstreams.delete.desc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={remove}>{t("common.delete")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={bulkConfirmDelete} onOpenChange={(v) => !v && setBulkConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.bulkDelete", { n: sel.selected.size })}</AlertDialogTitle>
            <AlertDialogDescription>{t("upstreams.bulkDelete.desc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => runBulk("delete")} disabled={bulkBusy}>
              {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
