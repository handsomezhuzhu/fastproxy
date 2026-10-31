import * as React from "react";
import { Copy, Ellipsis, Eye, EyeOff, Pencil, Plus, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { api, type Account, type Data } from "@/api";
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
import { EmptyState, Field, FormSelect, PageHeader } from "@/components/page";
import { bytes } from "@/lib/utils";
import type { CopyPreset } from "@/components/CopyWizard";

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

  const status = Object.fromEntries(data.status.accounts.map((s) => [s.id, s]));
  const upstreamName = (id: string) => data.upstreams.find((u) => u.id === id)?.name ?? "–";
  const portLabel = (id: string) => {
    if (!id) return "全部端口";
    const p = data.ports.find((x) => x.id === id);
    return p ? `端口 ${p.port}` : "–";
  };

  function openCreate() {
    setEditing(null);
    setForm({ username: "", password: "", port_id: "", upstream_id: "", remark: "" });
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

  const portItems = [
    { value: "", label: "全部端口可用" },
    ...data.ports.map((p) => ({ value: p.id, label: `端口 ${p.port}` })),
  ];
  const upstreamItems = [
    { value: "", label: "跟随端口默认上游" },
    ...data.upstreams.map((u) => ({ value: u.id, label: `${u.name}（${u.type} · ${u.host}）` })),
  ];

  return (
    <div className="space-y-5">
      <PageHeader
        title="账户"
        description="用户名全局唯一；绑定端口后仅该端口可用；可覆盖上游，实现按用户分流。"
        actions={
          <Button onClick={openCreate}>
            <Plus className="size-4" aria-hidden="true" /> 新增账户
          </Button>
        }
      />

      {data.accounts.length === 0 ? (
        <EmptyState
          icon={<Users className="size-5" aria-hidden="true" />}
          title="还没有账户"
          description="创建账户后即可通过端口认证，HTTP 与 SOCKS5 通用。"
          action={<Button onClick={openCreate}>新增账户</Button>}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
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
            {data.accounts.map((a) => {
              const s = status[a.id];
              const shown = revealed[a.id];
              return (
                <TableRow key={a.id}>
                  <TableCell>
                    <Switch
                      checked={a.enabled}
                      onCheckedChange={(v: boolean) => toggle(a, v)}
                      aria-label={`账户 ${a.username} 启用开关`}
                    />
                  </TableCell>
                  <TableCell className="font-medium">{a.username}</TableCell>
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
                  <TableCell className="text-xs">{portLabel(a.port_id)}</TableCell>
                  <TableCell className="text-xs">
                    {a.upstream_id ? upstreamName(a.upstream_id) : <span className="text-muted-foreground">跟随端口</span>}
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
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onClick={() => setDeleting(a)}>
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
            <Field label="绑定端口" htmlFor="acc-port">
              <FormSelect
                id="acc-port"
                value={form.port_id}
                onValueChange={(v) => setForm({ ...form, port_id: v })}
                items={portItems}
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
            {error ? <p className="text-xs text-destructive" role="alert">{error}</p> : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                取消
              </Button>
              <Button type="submit" disabled={busy}>
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
    </div>
  );
}
