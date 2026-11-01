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
import { api, type Data, type ImportResult, type Upstream } from "@/api";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  DotBadge,
  EmptyState,
  Field,
  PageHeader,
  PaginationControls,
  SelectAllCheckbox,
  Toolbar,
  useSelection,
} from "@/components/page";
import { timeAgo } from "@/lib/utils";

function HealthCell({ u }: { u: Upstream }) {
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
  const [form, setForm] = React.useState({ name: "", upstream: "", remark: "" });
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Upstream | null>(null);
  const [checking, setChecking] = React.useState<Record<string, boolean>>({});
  const [checkingAll, setCheckingAll] = React.useState(false);

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
        setList(r.entries);
        setTotal(r.total);
      } catch (err: any) {
        // 后端分页接口未上线时不要整页崩溃
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
    setForm({ name: "", upstream: "", remark: "" });
    setError("");
    setOpen(true);
  }
  function openEdit(u: Upstream) {
    setEditing(u);
    const creds = u.username || u.password ? `${u.username}:${u.password}@` : "";
    setForm({
      name: u.name,
      upstream: `${u.type === "socks5" ? "socks5" : "http"}://${creds}${u.host}`,
      remark: u.remark || "",
    });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (editing) await api.updateUpstream(editing.id, form);
      else await api.createUpstream(form);
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

  async function checkOne(u: Upstream) {
    setChecking((c) => ({ ...c, [u.id]: true }));
    try {
      const r = await api.checkUpstream(u.id);
      if (r.ok) {
        toast.success(`${u.name}：正常 · ${r.latency_ms} ms · 出口 ${r.exit_ip || "未知"}`);
      } else {
        toast.error(`${u.name}：${r.error || "探测失败"}`);
      }
      await refreshAll();
    } catch (err: any) {
      toast.error(err?.message || "探测失败");
    } finally {
      setChecking((c) => ({ ...c, [u.id]: false }));
    }
  }

  async function checkAll() {
    setCheckingAll(true);
    try {
      const r = await api.checkAllUpstreams();
      if (r.failed > 0) {
        toast.warning(`检测完成：正常 ${r.ok}，失败 ${r.failed}${r.server_ip ? ` · 本机出口 ${r.server_ip}` : ""}`);
      } else {
        toast.success(`检测完成：${r.checked} 个上游全部正常${r.server_ip ? ` · 本机出口 ${r.server_ip}` : ""}`);
      }
      await refreshAll();
    } catch (err: any) {
      toast.error(err?.message || "检测失败");
    } finally {
      setCheckingAll(false);
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
    setBulkBusy(true);
    try {
      const r = await api.bulkUpstreams(ids, action);
      if (action === "delete") {
        const skipped = r.skipped ?? [];
        const deleted = r.deleted ?? Math.max(0, ids.length - skipped.length);
        if (skipped.length > 0) {
          toast.error(`${skipped.length} 个上游跳过：${skipped[0].reason}`);
          if (deleted > 0) toast.success(`已删除 ${deleted} 个上游`);
        } else {
          toast.success(`已删除 ${deleted} 个上游`);
        }
      } else {
        const checks = r.checks ?? [];
        const ok = checks.filter((c) => c.ok).length;
        const fail = checks.filter((c) => !c.ok).length;
        if (fail > 0) toast.warning(`检测完成：正常 ${ok}，失败 ${fail}`);
        else toast.success(`检测完成：${ok} 个上游全部正常`);
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
        <PageHeader title="上游代理" description="支持 http / socks5；健康检查由定时探测维护。" />
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
        description="支持 http / socks5；健康检查由定时探测维护。"
      />

      {showEmpty ? (
        <EmptyState
          icon={<Layers className="size-5" aria-hidden="true" />}
          title="还没有上游代理"
          description="可粘贴节点文本或订阅链接批量导入，也支持手动添加。"
          action={
            <Button variant="outline" onClick={() => setImportOpen(true)}>
              <Import className="size-4" aria-hidden="true" /> 批量导入
            </Button>
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
            <Button variant="outline" size="sm" onClick={checkAll} disabled={checkingAll}>
              {checkingAll ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <RefreshCw className="size-4" aria-hidden="true" />
              )}
              {checkingAll ? "检测中…" : "检测全部"}
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
              <span className="text-sm tabular-nums">已选 {sel.selected.size} 项 ·</span>
              <Button size="sm" variant="outline" onClick={() => askBulk("check")} disabled={bulkBusy}>
                {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                检测
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
                          <Badge variant="outline">{u.type}</Badge>
                        </TableCell>
                        <TableCell>
                          <div className="font-mono text-xs">{u.host}</div>
                          {u.username ? (
                            <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
                              账号 {u.username}
                            </div>
                          ) : null}
                        </TableCell>
                        <TableCell>
                          <HealthCell u={u} />
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? `编辑上游 ${editing.name}` : "添加上游代理"}</DialogTitle>
            <DialogDescription>自动识别 http / socks5 协议与账号密码。</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="space-y-4">
            <Field label="名称" htmlFor="up-name">
              <Input
                id="up-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="美国-家宽-1"
              />
            </Field>
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
                placeholder="http://user:pass@1.2.3.4:8080 或 socks5://1.2.3.4:1080"
                spellCheck={false}
              />
            </Field>
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
                placeholder={"http://user:pass@1.2.3.4:8080#US-01\nsocks5://1.2.3.4:1080\n1.2.3.4:8080\n1.2.3.4:8080:user:pass"}
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
