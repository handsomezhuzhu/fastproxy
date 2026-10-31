import * as React from "react";
import { Copy, Ellipsis, Network, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type Data, type Port } from "@/api";
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
import { Field, FormSelect, PageHeader, EmptyState } from "@/components/page";
import { bytes } from "@/lib/utils";
import type { CopyPreset } from "@/components/CopyWizard";

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
  const [form, setForm] = React.useState({ port: "", default_upstream: "", remark: "" });
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Port | null>(null);

  const status = Object.fromEntries(data.status.ports.map((s) => [s.id, s]));
  const upstreamName = (id: string) => data.upstreams.find((u) => u.id === id)?.name ?? "直连";

  function openCreate() {
    setEditing(null);
    setForm({ port: "", default_upstream: "", remark: "" });
    setError("");
    setOpen(true);
  }
  function openEdit(p: Port) {
    setEditing(p);
    setForm({
      port: String(p.port),
      default_upstream: p.default_upstream || "",
      remark: p.remark || "",
    });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const body = {
      port: Number(form.port),
      default_upstream: form.default_upstream,
      remark: form.remark,
    };
    try {
      const res = editing ? await api.updatePort(editing.id, body) : await api.createPort(body);
      if (res.warning) toast.warning(`已保存，但监听失败：${res.warning}`);
      else toast.success("端口线路已保存");
      setOpen(false);
      await reload();
    } catch (err: any) {
      setError(err?.message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function toggle(p: Port, enabled: boolean) {
    try {
      await api.togglePort(p.id, enabled);
      toast.success(enabled ? `端口 ${p.port} 已启用` : `端口 ${p.port} 已停用`);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || "操作失败");
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

  const upstreamItems = [
    { value: "", label: "直连（不使用上游）" },
    ...data.upstreams.map((u) => ({ value: u.id, label: `${u.name}（${u.type} · ${u.host}）` })),
  ];

  return (
    <div className="space-y-5">
      <PageHeader
        title="端口线路"
        description={
          <>
            每个端口绑定一个默认上游；客户端可用 HTTP 或 SOCKS5 接入{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
              {window.location.hostname}:端口
            </code>
            。
          </>
        }
        actions={
          <Button onClick={openCreate}>
            <Plus className="size-4" aria-hidden="true" /> 新增线路
          </Button>
        }
      />

      {data.ports.length === 0 ? (
        <EmptyState
          icon={<Network className="size-5" aria-hidden="true" />}
          title="还没有端口线路"
          description="新建第一个监听端口，并绑定默认上游或直连。"
          action={<Button onClick={openCreate}>新增线路</Button>}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">启用</TableHead>
              <TableHead>端口</TableHead>
              <TableHead>默认上游</TableHead>
              <TableHead className="text-right">账户</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="text-right">连接</TableHead>
              <TableHead className="text-right">流量</TableHead>
              <TableHead>备注</TableHead>
              <TableHead className="w-12">
                <span className="sr-only">操作</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.ports.map((p) => {
              const s = status[p.id];
              const bound = data.accounts.filter((a) => a.port_id === p.id).length;
              return (
                <TableRow key={p.id}>
                  <TableCell>
                    <Switch
                      checked={p.enabled}
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
                  <TableCell>{p.default_upstream ? upstreamName(p.default_upstream) : <span className="text-muted-foreground">直连</span>}</TableCell>
                  <TableCell className="text-right tabular-nums">{bound}</TableCell>
                  <TableCell>
                    {!p.enabled ? (
                      <Badge variant="outline">已停用</Badge>
                    ) : s?.listening ? (
                      <Badge className="bg-emerald-500/15 text-emerald-400" variant="secondary">
                        监听中
                      </Badge>
                    ) : (
                      <Badge variant="destructive">未监听</Badge>
                    )}
                    {s?.error ? (
                      <p className="mt-1 max-w-[220px] truncate text-xs text-destructive" title={s.error}>
                        {s.error}
                      </p>
                    ) : null}
                  </TableCell>
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
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? `编辑线路 · 端口 ${editing.port}` : "新增端口线路"}</DialogTitle>
            <DialogDescription>监听端口需落在服务器/容器已放行的端口范围内。</DialogDescription>
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
            <Field label="默认上游代理" htmlFor="port-upstream" hint="账户可单独覆盖上游，实现按用户分流。">
              <FormSelect
                id="port-upstream"
                value={form.default_upstream}
                onValueChange={(v) => setForm({ ...form, default_upstream: v })}
                items={upstreamItems}
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
            <AlertDialogTitle>删除端口 {deleting?.port}？</AlertDialogTitle>
            <AlertDialogDescription>
              绑定到该端口的账户将变为全局可用，正在建立的连接会中断。
            </AlertDialogDescription>
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
