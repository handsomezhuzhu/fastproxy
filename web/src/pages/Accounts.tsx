import * as React from "react";
import { Copy, Eye, EyeOff, Pencil, Plus, Trash2 } from "lucide-react";
import { api, type Account, type Data } from "../api";
import {
  Badge,
  Button,
  ConfirmDialog,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  EmptyRow,
  Input,
  Label,
  Select,
  Switch,
  TBody,
  TD,
  TH,
  THead,
  TR,
  Table,
} from "../components/ui";
import { toast } from "../components/Toast";
import { bytes } from "../lib/utils";
import type { CopyPreset } from "../App";

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
  const [deleting, setDeleting] = React.useState<Account | null>(null);
  const [revealed, setRevealed] = React.useState<Record<string, boolean>>({});

  const st = Object.fromEntries(data.status.accounts.map((s) => [s.id, s]));

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

  async function save() {
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
      setOpen(false);
      toast("账户已保存");
      await reload();
    } catch (e: any) {
      setError(e?.message || "保存失败");
    }
  }

  async function toggle(a: Account, enabled: boolean) {
    try {
      await api.toggleAccount(a.id, enabled);
      await reload();
    } catch (e: any) {
      toast(e?.message || "操作失败", "error");
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteAccount(deleting.id);
      toast("已删除");
      setDeleting(null);
      await reload();
    } catch (e: any) {
      toast(e?.message || "删除失败", "error");
    }
  }

  const upName = (id: string) => data.upstreams.find((u) => u.id === id)?.name || "-";
  const portName = (id: string) => {
    if (!id) return "全部端口";
    const p = data.ports.find((x) => x.id === id);
    return p ? `端口 ${p.port}` : "-";
  };

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-lg font-semibold">账户</h1>
          <p className="text-xs text-muted-foreground">
            用户名全局唯一；绑定端口后仅该端口可用；可覆盖上游实现按用户分流
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" /> 新增账户
        </Button>
      </div>

      <Table>
        <THead>
          <TR>
            <TH className="w-14">启用</TH>
            <TH>用户名</TH>
            <TH>密码</TH>
            <TH>端口</TH>
            <TH>上游</TH>
            <TH>连接</TH>
            <TH>流量</TH>
            <TH>备注</TH>
            <TH className="text-right">操作</TH>
          </TR>
        </THead>
        <TBody>
          {data.accounts.length === 0 ? (
            <EmptyRow colSpan={9} text="暂无账户，点击右上角「新增账户」" />
          ) : (
            data.accounts.map((a) => {
              const s = st[a.id];
              return (
                <TR key={a.id}>
                  <TD>
                    <Switch checked={a.enabled} onCheckedChange={(v) => toggle(a, v)} />
                  </TD>
                  <TD className="font-medium">{a.username}</TD>
                  <TD>
                    <div className="flex items-center gap-1">
                      <code className="rounded bg-secondary px-1.5 py-0.5 font-mono text-[11px]">
                        {revealed[a.id] ? a.password : "••••••"}
                      </code>
                      <button
                        className="text-muted-foreground hover:text-foreground"
                        onClick={() => setRevealed((r) => ({ ...r, [a.id]: !r[a.id] }))}
                      >
                        {revealed[a.id] ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                      </button>
                    </div>
                  </TD>
                  <TD className="text-xs">{portName(a.port_id)}</TD>
                  <TD className="text-xs">
                    {a.upstream_id ? upName(a.upstream_id) : <span className="text-muted-foreground">跟随端口</span>}
                  </TD>
                  <TD className="whitespace-nowrap text-xs">
                    {s?.conns_cur ?? 0} / {s?.conns_total ?? 0}
                  </TD>
                  <TD className="whitespace-nowrap text-xs">
                    ↑{bytes(s?.bytes_out ?? 0)} ↓{bytes(s?.bytes_in ?? 0)}
                  </TD>
                  <TD className="max-w-[140px] truncate text-xs text-muted-foreground">{a.remark || "-"}</TD>
                  <TD>
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => onCopy({ dim: "account", id: a.id })}>
                        <Copy className="h-3.5 w-3.5" /> 复制
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => openEdit(a)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeleting(a)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TD>
                </TR>
              );
            })
          )}
        </TBody>
      </Table>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader
            title={editing ? `编辑账户 ${editing.username}` : "新增账户"}
            description="支持 HTTP 和 SOCKS5 两种代理协议认证"
          />
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>用户名（全局唯一）</Label>
              <Input
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                placeholder="alice"
              />
            </div>
            <div className="space-y-1.5">
              <Label>密码</Label>
              <Input
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                placeholder="支持 ${VAR} 环境变量引用"
              />
            </div>
            <div className="space-y-1.5">
              <Label>绑定端口</Label>
              <Select value={form.port_id} onChange={(e) => setForm({ ...form, port_id: e.target.value })}>
                <option value="">全部端口可用</option>
                {data.ports.map((p) => (
                  <option key={p.id} value={p.id}>
                    端口 {p.port}
                  </option>
                ))}
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>上游绑定</Label>
              <Select value={form.upstream_id} onChange={(e) => setForm({ ...form, upstream_id: e.target.value })}>
                <option value="">跟随端口默认上游</option>
                {data.upstreams.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}（{u.type} {u.host}）
                  </option>
                ))}
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>备注</Label>
              <Input
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
                placeholder="客户A / 用途"
              />
            </div>
            {error ? <p className="text-xs text-destructive">{error}</p> : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>取消</Button>
            <Button onClick={save}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleting}
        title={`删除账户 ${deleting?.username}？`}
        description="删除后该账户将无法再通过任何端口认证。"
        onConfirm={remove}
        onClose={() => setDeleting(null)}
      />
    </div>
  );
}
