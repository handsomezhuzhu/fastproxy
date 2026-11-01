import * as React from "react";
import { Copy, Ellipsis, Eye, EyeOff, Loader2, Pencil, Plus, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { api, type Account, type Data } from "@/api";
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
import { Switch } from "@/components/ui/switch";
import {
  EmptyState,
  Field,
  FormSelect,
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

export function AccountsPage({
  data,
  reload,
  onCopy,
}: {
  data: Data;
  reload: () => Promise<void>;
  onCopy: (p: CopyPreset) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Account | null>(null);
  const [form, setForm] = React.useState({
    username: "",
    password: "",
    port_id: "",
    upstream_id: "",
    remark: "",
  });
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Account | null>(null);
  const [revealed, setRevealed] = React.useState<Record<string, boolean>>({});

  // Bulk state
  const sel = useSelection();
  const [bulkAction, setBulkAction] = React.useState<BulkAction | null>(null);
  const [bulkBusy, setBulkBusy] = React.useState(false);
  const [bulkConfirmDelete, setBulkConfirmDelete] = React.useState(false);

  const pag = usePagination(data.accounts);

  const status = Object.fromEntries(data.status.accounts.map((s) => [s.id, s]));
  const upstreamName = (id: string) => data.upstreams.find((u) => u.id === id)?.name ?? "–";
  const portLabel = (id: string) => {
    const p = data.ports.find((x) => x.id === id);
    return p ? `端口 ${p.port}` : "–";
  };

  function openCreate() {
    setEditing(null);
    setForm({ username: "", password: "", port_id: data.ports[0]?.id ?? "", upstream_id: "", remark: "" });
    setError("");
    setOpen(true);
  }
  function openEdit(a: Account) {
    setEditing(a);
    setForm({
      username: a.username,
      password: a.password,
      port_id: a.port_id || "",
      upstream_id: a.upstream_id || "",
      remark: a.remark || "",
    });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!form.port_id) {
      setError("请选择绑定端口");
      return;
    }
    setBusy(true);
    setError("");
    const body = {
      username: form.username.trim(),
      password: form.password,
      port_id: form.port_id,
      upstream_id: form.upstream_id,
      remark: form.remark,
    };
    try {
      if (editing) await api.updateAccount(editing.id, body);
      else await api.createAccount(body);
      toast.success("账户已保存");
      setOpen(false);
      await reload();
    } catch (err: any) {
      setError(err?.message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function toggle(a: Account, enabled: boolean) {
    try {
      await api.toggleAccount(a.id, enabled);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || "操作失败");
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteAccount(deleting.id);
      toast.success(`账户 ${deleting.username} 已删除`);
      setDeleting(null);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || "删除失败");
    }
  }

  const actionLabels: Record<BulkAction, string> = { enable: "启用", disable: "禁用", delete: "删除" };

  async function runBulk(action: BulkAction) {
    const ids = [...sel.selected];
    if (ids.length === 0) return;
    setBulkBusy(true);
    try {
      const r = await api.bulkAccounts(ids, action);
      const failed = r.errors ?? [];
      const done = r.updated ?? Math.max(0, ids.length - failed.length);
      if (failed.length > 0) {
        toast.error(`${failed.length} 个账户${actionLabels[action]}失败：${failed[0].error}`);
        if (done > 0) toast.success(`已${actionLabels[action]} ${done} 个账户`);
      } else {
        toast.success(`已${actionLabels[action]} ${done} 个账户`);
      }
      sel.clear();
      await reload();
    } catch (err: any) {
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

  // 默认账号随端口删除，不参与批量选择
  const selectable = (a: Account) => !a.is_default;
  const pageIDs = pag.pageItems.filter(selectable).map((a) => a.id);
  const pageAllSelected = pageIDs.length > 0 && pageIDs.every((id) => sel.selected.has(id));
  const pageSomeSelected = pageIDs.some((id) => sel.selected.has(id));

  const portItems = data.ports.map((p) => ({ value: p.id, label: `端口 ${p.port}` }));
  const upstreamItems = [
    { value: "", label: "直连（默认）" },
    ...data.upstreams.map((u) => ({ value: u.id, label: `${u.name}（${u.type} · ${u.host}）` })),
  ];

  const enabledCount = data.accounts.filter((a) => a.enabled).length;

  return (
    <div className="space-y-5">
      <PageHeader
        title="账户"
        description="HTTP 与 SOCKS5 共用账号密码；每个端口自带一个默认账号。"
      />

      {data.accounts.length === 0 ? (
        <EmptyState
          icon={<Users className="size-5" aria-hidden="true" />}
          title="还没有账户"
          description="新建端口线路会自动生成默认账号，也可以手动新增。"
          action={
            <Button onClick={openCreate} disabled={data.ports.length === 0}>
              <Plus className="size-4" aria-hidden="true" /> 新增账户
            </Button>
          }
        />
      ) : (
        <>
          <Toolbar count={`${data.accounts.length} 个账户`} detail={`启用 ${enabledCount}`}>
            <Button size="sm" onClick={openCreate} disabled={data.ports.length === 0}>
              <Plus className="size-4" aria-hidden="true" /> 新增账户
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
                <TableHead>用户名</TableHead>
                <TableHead>密码</TableHead>
                <TableHead>端口</TableHead>
                <TableHead>上游</TableHead>
                <TableHead className="text-right">连接</TableHead>
                <TableHead className="text-right">流量</TableHead>
                <TableHead>备注</TableHead>
                <TableHead className="w-12">
                  <span className="sr-only">操作</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pag.pageItems.map((a) => {
                const s = status[a.id];
                const shown = revealed[a.id];
                const checked = sel.selected.has(a.id);
                return (
                  <TableRow key={a.id} data-state={checked ? "selected" : undefined}>
                    <TableCell>
                      <input
                        type="checkbox"
                        className="size-3.5 accent-primary focus-visible:outline-2 focus-visible:outline-ring"
                        checked={checked}
                        disabled={!selectable(a)}
                        title={selectable(a) ? undefined : "默认账号随端口删除"}
                        onChange={(e) => sel.toggle(a.id, e.target.checked)}
                        aria-label={`选择 账户 ${a.username}`}
                      />
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={a.enabled}
                        onCheckedChange={(v: boolean) => toggle(a, v)}
                        aria-label={`账户 ${a.username} 启用开关`}
                      />
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5">
                        <span className="font-medium">{a.username}</span>
                        {a.is_default ? <Badge variant="outline">默认</Badge> : null}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5">
                        <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                          {shown ? a.password : "••••••"}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => setRevealed((r) => ({ ...r, [a.id]: !r[a.id] }))}
                          aria-label={shown ? "隐藏密码" : "显示密码"}
                        >
                          {shown ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                        </Button>
                      </span>
                    </TableCell>
                    <TableCell className="text-xs">
                      {a.is_default ? (
                        <span className="text-muted-foreground">{portLabel(a.port_id)}</span>
                      ) : (
                        portLabel(a.port_id)
                      )}
                    </TableCell>
                    <TableCell className="text-xs">
                      {a.upstream_id ? (
                        upstreamName(a.upstream_id)
                      ) : (
                        <span className="text-muted-foreground">直连</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                      {s?.conns_cur ?? 0} / {s?.conns_total ?? 0}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                      ↑{bytes(s?.bytes_out ?? 0)} ↓{bytes(s?.bytes_in ?? 0)}
                    </TableCell>
                    <TableCell className="max-w-[140px] truncate text-xs text-muted-foreground">
                      {a.remark || "–"}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                          aria-label={`账户 ${a.username} 操作菜单`}
                        >
                          <Ellipsis className="size-4" aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-40">
                          <DropdownMenuItem onClick={() => openEdit(a)}>
                            <Pencil className="size-4" aria-hidden="true" /> 编辑
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onCopy({ dim: "account", id: a.id })}>
                            <Copy className="size-4" aria-hidden="true" /> 复制接入
                          </DropdownMenuItem>
                          {!a.is_default ? (
                            <>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem variant="destructive" onClick={() => setDeleting(a)}>
                                <Trash2 className="size-4" aria-hidden="true" /> 删除
                              </DropdownMenuItem>
                            </>
                          ) : null}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>

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
            <DialogTitle>{editing ? `编辑账户 ${editing.username}` : "新增账户"}</DialogTitle>
            <DialogDescription>HTTP 与 SOCKS5 共用同一账号密码。</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="space-y-4">
            <Field label="用户名（全局唯一）" htmlFor="acc-username">
              <Input
                id="acc-username"
                name="username"
                autoComplete="off"
                spellCheck={false}
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                placeholder="alice"
              />
            </Field>
            <Field label="密码" htmlFor="acc-password" hint="支持 ${VAR} 环境变量引用，明文不落盘。">
              <Input
                id="acc-password"
                name="password"
                autoComplete="new-password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                placeholder="账户密码"
              />
            </Field>
            <Field
              label="绑定端口"
              htmlFor="acc-port"
              hint={editing?.is_default ? "默认账号随端口生成，端口不可修改。" : data.ports.length === 0 ? "请先创建端口线路。" : undefined}
            >
              <FormSelect
                id="acc-port"
                value={form.port_id}
                onValueChange={(v) => setForm({ ...form, port_id: v })}
                items={portItems}
                disabled={!!editing?.is_default}
              />
            </Field>
            <Field label="上游绑定" htmlFor="acc-upstream">
              <FormSelect
                id="acc-upstream"
                value={form.upstream_id}
                onValueChange={(v) => setForm({ ...form, upstream_id: v })}
                items={upstreamItems}
              />
            </Field>
            <Field label="备注" htmlFor="acc-remark">
              <Input
                id="acc-remark"
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
                placeholder="客户A / 用途"
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
              <Button type="submit" disabled={busy || data.ports.length === 0}>
                {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                {busy ? "保存中…" : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除账户 {deleting?.username}？</AlertDialogTitle>
            <AlertDialogDescription>删除后该账户无法再通过任何端口认证。</AlertDialogDescription>
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
            <AlertDialogTitle>批量删除 {sel.selected.size} 个账户？</AlertDialogTitle>
            <AlertDialogDescription>删除后这些账户无法再通过任何端口认证。</AlertDialogDescription>
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
