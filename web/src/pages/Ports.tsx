import * as React from "react";
import { Copy, Pencil, Plus, Trash2 } from "lucide-react";
import { api, type Data, type Port } from "../api";
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
  const [deleting, setDeleting] = React.useState<Port | null>(null);

  const st = Object.fromEntries(data.status.ports.map((s) => [s.id, s]));

  function openCreate() {
    setEditing(null);
    setForm({ port: "", default_upstream: "", remark: "" });
    setError("");
    setOpen(true);
  }

  function openEdit(p: Port) {
    setEditing(p);
    setForm({ port: String(p.port), default_upstream: p.default_upstream || "", remark: p.remark || "" });
    setError("");
    setOpen(true);
  }

  async function save() {
    setError("");
    const body = {
      port: parseInt(form.port, 10),
      default_upstream: form.default_upstream,
      remark: form.remark,
    };
    try {
      if (editing) await api.updatePort(editing.id, body);
      else await api.createPort(body);
      setOpen(false);
      toast("端口线路已保存");
      await reload();
    } catch (e: any) {
      setError(e?.message || "保存失败");
    }
  }

  async function toggle(p: Port, enabled: boolean) {
    try {
      await api.togglePort(p.id, enabled);
      toast(enabled ? "已启用" : "已停用");
      await reload();
    } catch (e: any) {
      toast(e?.message || "操作失败", "error");
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deletePort(deleting.id);
      toast("已删除");
      setDeleting(null);
      await reload();
    } catch (e: any) {
      toast(e?.message || "删除失败", "error");
    }
  }

  const upName = (id: string) => data.upstreams.find((u) => u.id === id)?.name || "直连";

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-lg font-semibold">端口线路</h1>
          <p className="text-xs text-muted-foreground">
            每个端口绑定一个默认上游；客户端可用 HTTP 或 SOCKS5 连接 <code className="text-[11px]">{window.location.hostname}:端口</code>
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" /> 新增线路
        </Button>
      </div>

      <Table>
        <THead>
          <TR>
            <TH className="w-14">启用</TH>
            <TH>端口</TH>
            <TH>默认上游</TH>
            <TH>账户</TH>
            <TH>状态</TH>
            <TH>连接</TH>
            <TH>流量</TH>
            <TH>备注</TH>
            <TH className="text-right">操作</TH>
          </TR>
        </THead>
        <TBody>
          {data.ports.length === 0 ? (
            <EmptyRow colSpan={9} text="暂无端口线路，点击右上角「新增线路」" />
          ) : (
            data.ports.map((p) => {
              const s = st[p.id];
              const bound = data.accounts.filter((a) => a.port_id === p.id).length;
              return (
                <TR key={p.id}>
                  <TD>
                    <Switch checked={p.enabled} onCheckedChange={(v) => toggle(p, v)} />
                  </TD>
                  <TD>
                    <div className="font-medium">{p.port}</div>
                    <div className="font-mono text-[11px] text-muted-foreground">
                      {window.location.hostname}:{p.port}
                    </div>
                  </TD>
                  <TD>{p.default_upstream ? upName(p.default_upstream) : <span className="text-muted-foreground">直连</span>}</TD>
                  <TD>{bound} 个</TD>
                  <TD>
                    {p.enabled ? (
                      s?.listening ? (
                        <Badge variant="success">监听中</Badge>
                      ) : (
                        <Badge variant="destructive">未监听</Badge>
                      )
                    ) : (
                      <Badge variant="outline">已停用</Badge>
                    )}
                    {s?.error ? <div className="mt-1 max-w-[220px] text-[11px] text-destructive">{s.error}</div> : null}
                  </TD>
                  <TD className="whitespace-nowrap text-xs">
                    {s?.conns_cur ?? 0} / {s?.conns_total ?? 0}
                  </TD>
                  <TD className="whitespace-nowrap text-xs">
                    ↑{bytes(s?.bytes_out ?? 0)} ↓{bytes(s?.bytes_in ?? 0)}
                  </TD>
                  <TD className="max-w-[160px] truncate text-xs text-muted-foreground">{p.remark || "-"}</TD>
                  <TD>
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => onCopy({ dim: "port", id: p.id })}>
                        <Copy className="h-3.5 w-3.5" /> 复制
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => openEdit(p)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeleting(p)}>
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
            title={editing ? `编辑线路（端口 ${editing.port}）` : "新增端口线路"}
            description="监听端口必须在容器/服务器放行的端口范围内"
          />
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>监听端口</Label>
              <Input
                type="number"
                min={1}
                max={65535}
                value={form.port}
                onChange={(e) => setForm({ ...form, port: e.target.value })}
                placeholder="10201"
              />
            </div>
            <div className="space-y-1.5">
              <Label>默认上游代理</Label>
              <Select
                value={form.default_upstream}
                onChange={(e) => setForm({ ...form, default_upstream: e.target.value })}
              >
                <option value="">直连（不使用上游）</option>
                {data.upstreams.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}（{u.type} {u.host}）
                  </option>
                ))}
              </Select>
              <p className="text-[11px] text-muted-foreground">
                账户可单独覆盖上游，实现「按用户分流」
              </p>
            </div>
            <div className="space-y-1.5">
              <Label>备注</Label>
              <Input
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
                placeholder="业务线A / 客户专用"
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
        title={`删除端口 ${deleting?.port}？`}
        description="绑定到该端口的账户将变为全局可用，正在建立的连接会中断。"
        onConfirm={remove}
        onClose={() => setDeleting(null)}
      />
    </div>
  );
}
