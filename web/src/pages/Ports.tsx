import * as React from "react";
import { Check, Copy, Ellipsis, Loader2, Network, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type Account, type Data, type Port } from "@/api";
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
import { Switch } from "@/components/ui/switch";
import {
  DotBadge,
  EmptyState,
  Field,
  PageHeader,
  PaginationControls,
  SelectAllCheckbox,
  Toolbar,
  usePagination,
  useSelection,
} from "@/components/page";
import { bytes } from "@/lib/utils";
import type { CopyPreset } from "@/components/CopyWizard";

type BulkAction = "enable" | "disable" | "delete";

export function PortsPage({
  data,
  reload,
  onCopy,
}: {
  data: Data;
  reload: () => Promise<void>;
  onCopy: (p: CopyPreset) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Port | null>(null);
  const [form, setForm] = React.useState({ port: "", remark: "" });
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Port | null>(null);
  const [toggling, setToggling] = React.useState(false);
  const [created, setCreated] = React.useState<{ port: number; account: Account } | null>(null);
  const [copied, setCopied] = React.useState(false);

  // Bulk state
  const sel = useSelection();
  const [bulkAction, setBulkAction] = React.useState<BulkAction | null>(null);
  const [bulkBusy, setBulkBusy] = React.useState(false);
  const [bulkConfirmDelete, setBulkConfirmDelete] = React.useState(false);

  const pag = usePagination(data.ports);

  const status = Object.fromEntries(data.status.ports.map((s) => [s.id, s]));
  const listening = data.ports.filter((p) => status[p.id]?.listening).length;

  function openCreate() {
    setEditing(null);
    setForm({ port: "", remark: "" });
    setError("");
    setOpen(true);
  }
  function openEdit(p: Port) {
    setEditing(p);
    setForm({ port: String(p.port), remark: p.remark || "" });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (editing) {
        const res = await api.updatePort(editing.id, { port: Number(form.port), remark: form.remark });
        if (res.warning) toast.warning(`已保存，但监听失败：${res.warning}`);
        else toast.success("端口线路已保存");
      } else {
        const res = await api.createPort({ port: Number(form.port), remark: form.remark });
        const acc = res.saved?.default_account;
        // saved.port 可能是完整 Port 对象（React error #31 修复：不得直接渲染对象）
        const savedPort = res.saved?.port;
        const portNum =
          savedPort && typeof savedPort === "object"
            ? savedPort.port
            : Number(savedPort ?? form.port);
        if (acc) {
          // 不改变页面状态：保存成功后弹出默认账号提示。
          setCreated({ port: portNum, account: acc });
          setCopied(false);
        } else if (res.warning) {
          toast.warning(`已保存，但监听失败：${res.warning}`);
        } else {
          toast.success("端口线路已创建");
        }
      }
      setOpen(false);
      await reload();
    } catch (err: any) {
      setError(err?.message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function toggle(p: Port, enabled: boolean) {
    setToggling(true);
    try {
      await api.togglePort(p.id, enabled);
      toast.success(enabled ? `端口 ${p.port} 已启用` : `端口 ${p.port} 已停用`);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || "操作失败");
    } finally {
      setToggling(false);
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deletePort(deleting.id);
      toast.success(`端口 ${deleting.port} 已删除`);
      setDeleting(null);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || "删除失败");
    }
  }

  async function copyAccount() {
    if (!created) return;
    const text = `主机: ${window.location.hostname}\n端口: ${created.port}\n账号: ${created.account.username}\n密码: ${created.account.password}`;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success("接入信息已复制");
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      toast.error("复制失败，请手动选择文本复制");
    }
  }

  const actionLabels: Record<BulkAction, string> = { enable: "启用", disable: "禁用", delete: "删除" };

  async function runBulk(action: BulkAction) {
    const ids = [...sel.selected];
    if (ids.length === 0) return;
    setBulkBusy(true);
    try {
      const r = await api.bulkPorts(ids, action);
      const failed = r.errors ?? [];
      const done = r.updated ?? Math.max(0, ids.length - failed.length);
      if (failed.length > 0) {
        toast.error(`${failed.length} 个端口${actionLabels[action]}失败：${failed[0].error}`);
        if (done > 0) toast.success(`已${actionLabels[action]} ${done} 个端口`);
      } else {
        toast.success(`已${actionLabels[action]} ${done} 个端口`);
      }
      sel.clear();
      await reload();
    } catch (err: any) {
      // 后端批量接口未上线（404）或其他错误：友好提示，不清空选择
      toast.error(err?.message || "批量操作失败");
    } finally {
      setBulkBusy(false);
      setBulkConfirmDelete(false);
    }
  }

  function askBulk(action: BulkAction) {
    setBulkAction(action);
    if (action === "delete") setBulkConfirmDelete(true);
    else runBulk(action);
  }

  const pageIDs = pag.pageItems.map((p) => p.id);
  const pageAllSelected = pageIDs.length > 0 && pageIDs.every((id) => sel.selected.has(id));
  const pageSomeSelected = pageIDs.some((id) => sel.selected.has(id));

  return (
    <div className="space-y-5">
      <PageHeader
        title="端口线路"
        description="每个端口同时支持 HTTP 与 SOCKS5 接入，创建时自动生成默认账号。"
      />

      {data.ports.length === 0 ? (
        <EmptyState
          icon={<Network className="size-5" aria-hidden="true" />}
          title="还没有端口线路"
          description="新建第一个监听端口，默认账号一并生成。"
          action={
            <Button onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> 新增线路
            </Button>
          }
        />
      ) : (
        <>
          <Toolbar count={`${data.ports.length} 条线路`} detail={`监听中 ${listening}`}>
            <Button size="sm" onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> 新增线路
            </Button>
          </Toolbar>

          {sel.selected.size > 0 ? (
            <div
              className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2"
              role="status"
            >
              <span className="text-sm tabular-nums">已选 {sel.selected.size} 项 ·</span>
              <Button size="sm" variant="outline" onClick={() => askBulk("enable")} disabled={bulkBusy}>
                启用
              </Button>
              <Button size="sm" variant="outline" onClick={() => askBulk("disable")} disabled={bulkBusy}>
                禁用
              </Button>
              <Button size="sm" variant="destructive" onClick={() => askBulk("delete")} disabled={bulkBusy}>
                {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                删除
              </Button>
              <Button size="sm" variant="ghost" onClick={sel.clear} aria-label="取消选择">
                取消
              </Button>
            </div>
          ) : null}

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
                <TableHead className="w-16">启用</TableHead>
                <TableHead>端口</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">账户</TableHead>
                <TableHead className="text-right">连接</TableHead>
                <TableHead className="text-right">流量</TableHead>
                <TableHead>备注</TableHead>
                <TableHead className="w-12">
                  <span className="sr-only">操作</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pag.pageItems.map((p) => {
                const s = status[p.id];
                const bound = data.accounts.filter((a) => a.port_id === p.id).length;
                const checked = sel.selected.has(p.id);
                return (
                  <TableRow key={p.id} data-state={checked ? "selected" : undefined}>
                    <TableCell>
                      <input
                        type="checkbox"
                        className="size-3.5 accent-primary focus-visible:outline-2 focus-visible:outline-ring"
                        checked={checked}
                        onChange={(e) => sel.toggle(p.id, e.target.checked)}
                        aria-label={`选择 端口 ${p.port}`}
                      />
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={p.enabled}
                        disabled={toggling}
                        onCheckedChange={(v: boolean) => toggle(p, v)}
                        aria-label={`端口 ${p.port} 启用开关`}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="font-medium tabular-nums">{p.port}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {window.location.hostname}:{p.port}
                      </div>
                    </TableCell>
                    <TableCell>
                      {!p.enabled ? (
                        <DotBadge tone="muted">已停用</DotBadge>
                      ) : s?.listening ? (
                        <DotBadge tone="ok">监听中</DotBadge>
                      ) : (
                        <DotBadge tone="fail" title={s?.error}>未监听</DotBadge>
                      )}
                      {s?.error ? (
                        <p className="mt-1 max-w-[220px] truncate text-xs text-destructive" title={s.error}>
                          {s.error}
                        </p>
                      ) : null}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{bound}</TableCell>
                    <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                      {s?.conns_cur ?? 0} / {s?.conns_total ?? 0}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                      ↑{bytes(s?.bytes_out ?? 0)} ↓{bytes(s?.bytes_in ?? 0)}
                    </TableCell>
                    <TableCell className="max-w-[160px] truncate text-xs text-muted-foreground">
                      {p.remark || "–"}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                          aria-label={`端口 ${p.port} 操作菜单`}
                        >
                          <Ellipsis className="size-4" aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-40">
                          <DropdownMenuItem onClick={() => openEdit(p)}>
                            <Pencil className="size-4" aria-hidden="true" /> 编辑
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onCopy({ dim: "port", id: p.id })}>
                            <Copy className="size-4" aria-hidden="true" /> 复制接入
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(p)}>
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
            page={pag.page}
            pageSize={pag.pageSize}
            total={pag.total}
            onChange={pag.onChange}
          />
        </>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? `编辑线路 · 端口 ${editing.port}` : "新增端口线路"}</DialogTitle>
            <DialogDescription>端口仅开放入口，系统自动生成默认账号。</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="space-y-4">
            <Field label="监听端口" htmlFor="port-input">
              <Input
                id="port-input"
                name="port"
                type="number"
                inputMode="numeric"
                min={1}
                max={65535}
                required
                value={form.port}
                onChange={(e) => setForm({ ...form, port: e.target.value })}
                placeholder="10201"
              />
            </Field>
            <Field label="备注" htmlFor="port-remark">
              <Input
                id="port-remark"
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
                placeholder="业务线A / 客户专用"
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

      <Dialog open={!!created} onOpenChange={(v) => !v && setCreated(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>默认账号已生成</DialogTitle>
            <DialogDescription>
              端口 {created?.port} 已创建，请把下面的默认账号交给客户端使用。
            </DialogDescription>
          </DialogHeader>
          {created ? (
            <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
              <div className="flex items-center justify-between gap-4 text-sm">
                <span className="text-muted-foreground">用户名</span>
                <code className="font-mono">{created.account.username}</code>
              </div>
              <div className="flex items-center justify-between gap-4 text-sm">
                <span className="text-muted-foreground">密码</span>
                <code className="font-mono">{created.account.password}</code>
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreated(null)}>
              知道了
            </Button>
            <Button onClick={copyAccount}>
              {copied ? <Check className="size-4" aria-hidden="true" /> : <Copy className="size-4" aria-hidden="true" />}
              {copied ? "已复制" : "一键复制"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除端口 {deleting?.port}？</AlertDialogTitle>
            <AlertDialogDescription>
              默认账号将一并删除；若仍有其他绑定账户需先删除。
            </AlertDialogDescription>
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
            <AlertDialogTitle>批量删除 {sel.selected.size} 个端口？</AlertDialogTitle>
            <AlertDialogDescription>
              其默认账号将一并删除；仍有其他绑定账户的端口会跳过并报错。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => runBulk(bulkAction ?? "delete")} disabled={bulkBusy}>
              {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
