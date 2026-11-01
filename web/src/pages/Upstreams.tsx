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
import {
  api,
  type Data,
  type ImportResult,
  type Upstream,
  type UpstreamInput,
} from "@/api";
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
import { timeAgo } from "@/lib/utils";

function HealthCell({ u, checking }: { u: Upstream; checking?: boolean }) {
  if (checking) {
    return (
      <DotBadge tone="muted">
        <Loader2 className="size-3 animate-spin" aria-hidden="true" />
        检测中…
      </DotBadge>
    );
  }
  if (!u.last_check_at) {
    return <DotBadge tone="muted">未检测</DotBadge>;
  }
  const checkedAt = new Date(u.last_check_at * 1000).toISOString();
  if (u.health === "ok") {
    return (
      <div className="space-y-0.5">
        <div className="flex items-center gap-1.5">
          <DotBadge tone="ok">正常</DotBadge>
          <span className="text-xs tabular-nums text-muted-foreground">{u.latency_ms ?? 0} ms</span>
        </div>
        <p className="text-xs text-muted-foreground">{timeAgo(checkedAt)}检测</p>
      </div>
    );
  }
  return (
    <div className="space-y-0.5">
      <DotBadge tone="fail" title={u.last_error || "探测失败"}>
        <AlertTriangle className="size-3" aria-hidden="true" />
        失败
      </DotBadge>
      <p className="max-w-[180px] truncate text-xs text-muted-foreground" title={u.last_error}>
        {u.last_error || "探测失败"} · {timeAgo(checkedAt)}
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
  tls: boolean;
  sni: string;
  remark: string;
  // sing-box 出站字段（VLESS / VMess）
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
  tls: true,
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

const SECURITY_OPTIONS = [
  { value: "none", label: "无加密" },
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

const FLOW_OPTIONS = [
  { value: "", label: "空（无 Vision）" },
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
  if (u.type !== "vless" && u.type !== "vmess") return null;
  const marks: string[] = [];
  if (u.type === "vless") {
    if (u.security === "reality" || u.pbk) marks.push("REALITY");
    if (u.flow) marks.push("Vision");
  }
  if (u.transport === "ws") marks.push("WS");
  if (u.transport === "grpc") marks.push("gRPC");
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

export function UpstreamsPage({ data, reload }: { data: Data; reload: () => Promise<void> }) {
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
    const t = window.setTimeout(() => {
      setQ(keyword.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(t);
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
        toast.error(err?.message || "加载上游列表失败");
      } finally {
        setRefreshing(false);
      }
    },
    [page, pageSize, q]
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
      tls: Boolean(u.tls),
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
      tls: typeof p.tls === "boolean" ? p.tls : f.tls,
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
    // REALITY 自带 TLS：链接里 security=reality 时同步打开 TLS 开关，避免保存时被清空。
    if (security === "reality") setForm((f) => ({ ...f, tls: true }));
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
      toast.success("已解析并填入表单");
    } catch (err: any) {
      // 后端未合并该接口时会是 404/405；统一在表单里给出可读提示，不崩溃。
      const status = typeof err?.status === "number" ? err.status : 0;
      const msg =
        status === 404 || status === 405
          ? "当前后端不支持分享链接解析（/api/upstreams/parse），请手动填写字段。"
          : err?.message || "解析失败";
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
      // REALITY 自带 TLS，按 security 归一化开关，避免出现 security=tls + tls=false。
      const security = asSecurity(form.security);
      const tls = security === "reality" ? true : form.tls;
      body.host = form.host.trim();
      body.uuid = form.uuid.trim();
      body.security = security;
      body.tls = tls;
      body.sni = tls ? form.sni.trim() : "";
      body.pbk = security === "reality" ? form.pbk.trim() : "";
      body.sid = security === "reality" ? form.sid.trim() : "";
      body.fingerprint = security === "reality" ? form.fingerprint || "chrome" : "";
      body.flow = form.flow;
      body.transport = form.transport;
      body.path = form.transport === "ws" ? form.path.trim() : "";
      body.host_header = form.transport === "ws" ? form.hostHeader.trim() : "";
      body.service_name = form.transport === "grpc" ? form.serviceName.trim() : "";
    } else {
      body.host = form.host.trim();
      body.uuid = form.uuid.trim();
      body.security = form.tls ? "tls" : "none";
      body.tls = form.tls;
      body.sni = form.tls ? form.sni.trim() : "";
      body.transport = form.transport;
      body.path = form.transport === "ws" ? form.path.trim() : "";
      body.host_header = form.transport === "ws" ? form.hostHeader.trim() : "";
      body.service_name = form.transport === "grpc" ? form.serviceName.trim() : "";
      body.vmess_security = form.vmessSecurity || "auto";
    }
    try {
      if (editing) await api.updateUpstream(editing.id, body);
      else await api.createUpstream(body);
      toast.success("上游已保存");
      setOpen(false);
      await refreshAll();
    } catch (err: any) {
      setError(err?.message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteUpstream(deleting.id);
      toast.success(`上游「${deleting.name}」已删除`);
      setDeleting(null);
      await refreshAll();
    } catch (err: any) {
      toast.error(err?.message || "删除失败");
    }
  }

  // Merge one check result into the visible list immediately (no full reload),
  // so a sequential bulk check updates rows as each result arrives.
  function applyCheckResult(
    id: string,
    r: { ok?: boolean; latency_ms?: number; exit_ip?: string; error?: string }
  ) {
    setList((prev) =>
      prev
        ? prev.map((u) =>
            u.id === id
              ? {
                  ...u,
                  health: r.ok ? ("ok" as const) : ("fail" as const),
                  latency_ms: r.ok ? r.latency_ms ?? 0 : 0,
                  exit_ip: r.ok ? r.exit_ip || "" : "",
                  last_error: r.ok ? "" : r.error || "探测失败",
                  last_check_at: Math.floor(Date.now() / 1000),
                }
              : u
          )
        : prev
    );
  }

  async function checkOne(u: Upstream) {
    setChecking((c) => ({ ...c, [u.id]: true }));
    try {
      const r = await api.checkUpstream(u.id);
      applyCheckResult(u.id, r);
      if (r.ok) {
        toast.success(`${u.name}：正常 · ${r.latency_ms} ms · 出口 ${r.exit_ip || "未知"}`);
      } else {
        toast.error(`${u.name}：${r.error || "探测失败"}`);
      }
      await reload();
    } catch (err: any) {
      applyCheckResult(u.id, { ok: false, error: err?.message || "探测失败" });
      toast.error(err?.message || "探测失败");
    } finally {
      setChecking((c) => ({ ...c, [u.id]: false }));
    }
  }

  // 顺序检测：一个完成后再测下一个，每完成一个立刻把结果写回当前行。
  async function checkSequentially(ids: string[]) {
    if (ids.length === 0) {
      toast.info("没有可检测的上游");
      return;
    }
    setBulkBusy(true);
    setCheckingAll(true);
    setCheckProgress({ done: 0, total: ids.length });
    let ok = 0;
    let fail = 0;
    for (let i = 0; i < ids.length; i++) {
      const id = ids[i];
      setChecking((c) => ({ ...c, [id]: true }));
      try {
        const r = await api.checkUpstream(id);
        applyCheckResult(id, r);
        if (r.ok) ok++;
        else fail++;
      } catch (err: any) {
        applyCheckResult(id, { ok: false, error: err?.message || "探测失败" });
        fail++;
      } finally {
        setChecking((c) => ({ ...c, [id]: false }));
        setCheckProgress({ done: i + 1, total: ids.length });
      }
      // 让浏览器有机会渲染最新一行，再继续下一个
      if (i + 1 < ids.length) {
        await new Promise((res) => window.setTimeout(res, 120));
      }
    }
    setBulkBusy(false);
    setCheckingAll(false);
    if (fail > 0) toast.warning(`检测完成：正常 ${ok}，失败 ${fail}`);
    else toast.success(`检测完成：${ok} 个上游全部正常`);
    await refreshAll();
  }

  async function checkAll() {
    try {
      const all = await api.upstreamsList({ limit: 1000, offset: 0 });
      await checkSequentially(all.entries.map((u) => u.id));
    } catch (err: any) {
      toast.error(err?.message || "检测失败");
    }
  }

  async function runImport(useURL: boolean) {
    setImporting(true);
    setResult(null);
    try {
      const r = await api.importUpstreams(useURL ? { url: importURL } : { text: importText });
      setResult(r);
      if (r.added > 0) {
        toast.success(`成功导入 ${r.added} 个上游`);
        await refreshAll();
      } else {
        toast.warning("没有新增上游：可能全部重复或格式无法识别");
      }
    } catch (err: any) {
      toast.error(err?.message || "导入失败");
    } finally {
      setImporting(false);
    }
  }

  async function runBulk(action: BulkAction) {
    const ids = [...sel.selected];
    if (ids.length === 0) return;
    if (action === "check") {
      await checkSequentially(ids);
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
        toast.error(`${skipped.length} 个上游跳过：${skipped[0].reason}`);
        if (deleted > 0) toast.success(`已删除 ${deleted} 个上游`);
      } else {
        toast.success(`已删除 ${deleted} 个上游`);
      }
      sel.clear();
      await refreshAll();
    } catch (err: any) {
      toast.error(err?.message || "批量操作失败");
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
        <PageHeader
          title="上游代理"
          description="支持 HTTP / SOCKS5 / Shadowsocks / VLESS（含 REALITY）/ VMess。"
        />
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
      <PageHeader
        title="上游代理"
        description="支持 HTTP / SOCKS5 / Shadowsocks / VLESS（含 REALITY）/ VMess。"
      />

      {showEmpty ? (
        <EmptyState
          icon={<Layers className="size-5" aria-hidden="true" />}
          title="还没有上游代理"
          description="可粘贴节点文本或订阅链接批量导入，也支持手动添加。"
          action={
            <div className="flex flex-wrap items-center justify-center gap-2">
              <Button variant="outline" onClick={() => setImportOpen(true)}>
                <Import className="size-4" aria-hidden="true" /> 批量导入
              </Button>
              <Button onClick={openCreate}>
                <Plus className="size-4" aria-hidden="true" /> 添加上游
              </Button>
            </div>
          }
        />
      ) : (
        <>
          <Toolbar
            count={`${total} 个上游`}
            detail={q ? `按「${q}」筛选` : refreshing ? "刷新中…" : undefined}
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
                placeholder="搜索名称或地址…"
                aria-label="搜索上游"
              />
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={checkAll}
              disabled={checkingAll || bulkBusy}
            >
              {checkingAll ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <RefreshCw className="size-4" aria-hidden="true" />
              )}
              <span className="inline-block min-w-20 text-center tabular-nums">
                {checkingAll ? `检测中 ${checkProgress.done}/${checkProgress.total}` : "检测全部"}
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
              <Import className="size-4" aria-hidden="true" /> 批量导入
            </Button>
            <Button size="sm" onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> 添加上游
            </Button>
          </Toolbar>

          {sel.selected.size > 0 ? (
            <div
              className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2"
              role="status"
            >
              <span className="text-sm tabular-nums">
                {checkingAll
                  ? `检测中 ${checkProgress.done}/${checkProgress.total} ·`
                  : `已选 ${sel.selected.size} 项 ·`}
              </span>
              <Button size="sm" variant="outline" onClick={() => askBulk("check")} disabled={bulkBusy}>
                {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-8 text-center">检测</span>
              </Button>
              <Button size="sm" variant="destructive" onClick={() => askBulk("delete")} disabled={bulkBusy}>
                删除
              </Button>
              <Button size="sm" variant="ghost" onClick={sel.clear} aria-label="取消选择">
                取消
              </Button>
            </div>
          ) : null}

          {(list ?? []).length === 0 ? (
            <EmptyState
              icon={<Search className="size-5" aria-hidden="true" />}
              title="没有匹配的上游"
              description={`换个关键字试试，或直接添加新上游。`}
              action={
                <Button size="sm" variant="outline" onClick={() => setKeyword("")}>
                  清空搜索
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
                        label="全选当前页"
                      />
                    </TableHead>
                    <TableHead>名称</TableHead>
                    <TableHead>类型</TableHead>
                    <TableHead>地址</TableHead>
                    <TableHead>健康</TableHead>
                    <TableHead>出口 IP</TableHead>
                    <TableHead className="text-right">引用</TableHead>
                    <TableHead className="w-12">
                      <span className="sr-only">操作</span>
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
                            aria-label={`选择 上游 ${u.name}`}
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
                              {u.method || "加密方式未设置"}
                            </div>
                          ) : u.type === "vless" || u.type === "vmess" ? (
                            <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
                              {u.uuid ? `UUID ${u.uuid.slice(0, 8)}…` : "UUID 未设置"}
                              {u.type === "vless" && u.security === "reality"
                                ? " · REALITY"
                                : u.tls
                                  ? " · TLS"
                                  : ""}
                              {u.transport && u.transport !== "tcp"
                                ? ` · ${u.transport === "ws" ? "WS" : "gRPC"} ${u.transport === "ws" ? u.path || "/" : u.service_name || ""}`.trimEnd()
                                : ""}
                            </div>
                          ) : u.username ? (
                            <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
                              账号 {u.username}
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
                              aria-label={`上游 ${u.name} 操作菜单`}
                            >
                              {checking[u.id] ? (
                                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                              ) : (
                                <Ellipsis className="size-4" aria-hidden="true" />
                              )}
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-40">
                              <DropdownMenuItem onClick={() => checkOne(u)} disabled={checking[u.id]}>
                                <Zap className="size-4" aria-hidden="true" /> 立即检测
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => openEdit(u)}>
                                <Pencil className="size-4" aria-hidden="true" /> 编辑
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem variant="destructive" onClick={() => setDeleting(u)}>
                                <Trash2 className="size-4" aria-hidden="true" /> 删除
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
            <DialogTitle>{editing ? `编辑上游 ${editing.name}` : "添加上游代理"}</DialogTitle>
            <DialogDescription>
              支持 HTTP / SOCKS5 / Shadowsocks / VLESS（REALITY / Vision / WS / gRPC）与 VMess。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <Field label="名称" htmlFor="up-name">
                <Input
                  id="up-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="美国-家宽-1"
                />
              </Field>
              <Field label="协议" htmlFor="up-type">
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
                <Label htmlFor="up-share">粘贴分享链接（可选）</Label>
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
                    解析并填入
                  </Button>
                </div>
                {parseError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {parseError}
                  </p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    支持 vless://、vmess://、ss://、http://、socks5://，解析后仍可手动修改。
                  </p>
                )}
              </div>
            ) : null}

            {form.type === "http" || form.type === "socks5" ? (
              <>
                <Field
                  label="地址"
                  htmlFor="up-address"
                  hint="支持 ${VAR} 或 ${VAR:-默认值} 环境变量引用，密码不落盘。"
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
                <p className="text-xs text-muted-foreground">
                  也可以直接粘贴 `ss://`、`vless://`、`vmess://` 分享链接，保存时自动识别协议并解析字段。
                </p>
              </>
            ) : null}

            {isTunnelForm(form.type) ? (
              <Field label="服务器地址" htmlFor="up-host" hint="host:port，例如 1.2.3.4:8388。">
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
                <Field label="加密方式" htmlFor="up-method">
                  <FormSelect
                    id="up-method"
                    value={form.method}
                    onValueChange={(v) => setForm({ ...form, method: v })}
                    items={SS_METHODS}
                  />
                </Field>
                <Field label="密码" htmlFor="up-ss-password">
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
                <Field label="UUID" htmlFor="up-uuid">
                  <Input
                    id="up-uuid"
                    className="font-mono"
                    value={form.uuid}
                    onChange={(e) => setForm({ ...form, uuid: e.target.value })}
                    placeholder="b831381d-6324-4d53-ad4f-8cda48b30811"
                    spellCheck={false}
                  />
                </Field>
                <Field label="传输方式" htmlFor="up-transport">
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
                  <Field label="安全层" htmlFor="up-security">
                    <FormSelect
                      id="up-security"
                      value={form.security}
                      onValueChange={(v) => setForm({ ...form, security: asSecurity(v) })}
                      items={SECURITY_OPTIONS}
                    />
                  </Field>
                  <Field label="Flow" htmlFor="up-flow" hint="XTLS Vision 需要服务端开启对应流控。">
                    <FormSelect
                      id="up-flow"
                      value={form.flow}
                      onValueChange={(v) => setForm({ ...form, flow: v })}
                      items={
                        FLOW_OPTIONS.some((o) => o.value === form.flow)
                          ? FLOW_OPTIONS
                          : [...FLOW_OPTIONS, { value: form.flow, label: form.flow }]
                      }
                    />
                  </Field>
                </div>

                {form.security === "reality" ? (
                  <div className="grid grid-cols-3 gap-3">
                    <Field label="公钥 pbk" htmlFor="up-pbk">
                      <Input
                        id="up-pbk"
                        className="font-mono"
                        value={form.pbk}
                        onChange={(e) => setForm({ ...form, pbk: e.target.value })}
                        placeholder="REALITY 公钥"
                        spellCheck={false}
                      />
                    </Field>
                    <Field label="Short ID" htmlFor="up-sid">
                      <Input
                        id="up-sid"
                        className="font-mono"
                        value={form.sid}
                        onChange={(e) => setForm({ ...form, sid: e.target.value })}
                        placeholder="sid"
                        spellCheck={false}
                      />
                    </Field>
                    <Field label="指纹" htmlFor="up-fingerprint">
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
                      <div className="text-sm font-medium">启用 TLS</div>
                      <p className="text-xs text-muted-foreground">
                        严格校验服务端证书，SNI 留空时使用服务器主机名。
                      </p>
                    </div>
                    <Switch
                      checked={form.tls}
                      onCheckedChange={(v) => setForm({ ...form, tls: Boolean(v) })}
                      aria-label="启用 TLS"
                    />
                  </div>
                )}
              </>
            ) : null}

            {form.type === "vmess" ? (
              <div className="grid grid-cols-2 gap-3">
                <Field label="安全层" htmlFor="up-vmess-security">
                  <FormSelect
                    id="up-vmess-security"
                    value={form.vmessSecurity}
                    onValueChange={(v) => setForm({ ...form, vmessSecurity: v })}
                    items={VMESS_SECURITY_OPTIONS}
                  />
                </Field>
                <div className="flex items-center justify-between rounded-lg border px-3 py-2">
                  <div>
                    <div className="text-sm font-medium">启用 TLS</div>
                    <p className="text-xs text-muted-foreground">由 sing-box 出站发起 VMess 连接。</p>
                  </div>
                  <Switch
                    checked={form.tls}
                    onCheckedChange={(v) => setForm({ ...form, tls: Boolean(v) })}
                    aria-label="启用 TLS"
                  />
                </div>
              </div>
            ) : null}

            {form.type === "vless" || form.type === "vmess" ? (
              <>
                {form.tls || form.security === "reality" ? (
                  <Field label="SNI" htmlFor="up-sni" hint="留空则使用服务器主机名；IP 地址不会发送 SNI。">
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
                    <Field label="WS Path" htmlFor="up-path" hint="默认 /。">
                      <Input
                        id="up-path"
                        className="font-mono"
                        value={form.path}
                        onChange={(e) => setForm({ ...form, path: e.target.value })}
                        placeholder="/ws"
                        spellCheck={false}
                      />
                    </Field>
                    <Field label="WS Host 头" htmlFor="up-host-header" hint="留空则不额外携带 Host。">
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
                  <Field label="gRPC Service Name" htmlFor="up-service-name" hint="留空则使用空 serviceName。">
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

            <Field label="备注" htmlFor="up-remark">
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
                取消
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-16 text-center">{busy ? "保存中…" : "保存"}</span>
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>批量导入上游</DialogTitle>
            <DialogDescription>
              支持 URI 行、IP:PORT、IP:PORT:USER:PASS、base64 订阅、sing-box JSON、Clash 节点列表；自动去重。
              节点支持 VLESS（REALITY / XTLS Vision / WS / gRPC）、VMess（WS / gRPC）、Shadowsocks。
            </DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="text">
            <TabsList>
              <TabsTrigger value="text">粘贴文本</TabsTrigger>
              <TabsTrigger value="url">订阅链接</TabsTrigger>
            </TabsList>

            <TabsContent value="text" className="space-y-2 pt-2">
              <Textarea
                aria-label="节点文本"
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
                <span className="inline-block min-w-16 text-center">{importing ? "导入中…" : "导入文本"}</span>
              </Button>
            </TabsContent>

            <TabsContent value="url" className="space-y-2 pt-2">
              <Input
                aria-label="订阅链接"
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
                <span className="inline-block min-w-16 text-center">{importing ? "拉取中…" : "拉取并导入"}</span>
              </Button>
            </TabsContent>
          </Tabs>

          {result ? (
            <div className="space-y-2 rounded-lg border bg-muted/30 p-3 text-xs" aria-live="polite">
              <div className="flex flex-wrap gap-2">
                <Badge variant="secondary" className="bg-foreground text-background">
                  新增 {result.added}
                </Badge>
                <Badge variant="secondary" className="bg-muted text-foreground">
                  重复 {result.duplicates.length}
                </Badge>
                <Badge variant="secondary" className="border border-foreground/40 font-medium text-foreground">
                  跳过 {result.problems.length}
                </Badge>
                <Badge variant="outline">识别 {result.total}</Badge>
              </div>
              {result.duplicates.length > 0 ? (
                <details>
                  <summary className="cursor-pointer text-muted-foreground">重复项</summary>
                  <ul className="mt-1 max-h-24 overflow-y-auto font-mono">
                    {result.duplicates.map((d, i) => (
                      <li key={i}>{d}</li>
                    ))}
                  </ul>
                </details>
              ) : null}
              {result.problems.length > 0 ? (
                <details>
                  <summary className="cursor-pointer text-muted-foreground">跳过明细</summary>
                  <ul className="mt-1 max-h-24 overflow-y-auto font-mono">
                    {result.problems.map((d, i) => (
                      <li key={i}>{d}</li>
                    ))}
                  </ul>
                </details>
              ) : null}
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={() => setImportOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除上游「{deleting?.name}」？</AlertDialogTitle>
            <AlertDialogDescription>仍被账户引用时无法删除。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={remove}>删除</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={bulkConfirmDelete} onOpenChange={(v) => !v && setBulkConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>批量删除 {sel.selected.size} 个上游？</AlertDialogTitle>
            <AlertDialogDescription>仍被账户引用的上游会跳过并在结果中列出原因。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => runBulk("delete")} disabled={bulkBusy}>
              {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
