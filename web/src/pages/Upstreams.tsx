import * as React from "react";
import { Gauge, Import, Pencil, Plus, Trash2, Zap } from "lucide-react";
import { api, type Data, type ImportResult, type Upstream } from "../api";
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
  TBody,
  TD,
  TH,
  THead,
  TR,
  Table,
  Textarea,
} from "../components/ui";
import { toast } from "../components/Toast";

export function UpstreamsPage({ data, reload }: { data: Data; reload: () => Promise<void> }) {
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Upstream | null>(null);
  const [form, setForm] = React.useState({ name: "", upstream: "", remark: "" });
  const [error, setError] = React.useState("");
  const [deleting, setDeleting] = React.useState<Upstream | null>(null);
  const [testing, setTesting] = React.useState<Record<string, string>>({});

  const [importOpen, setImportOpen] = React.useState(false);
  const [importText, setImportText] = React.useState("");
  const [importURL, setImportURL] = React.useState("");
  const [importing, setImporting] = React.useState(false);
  const [result, setResult] = React.useState<ImportResult | null>(null);

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

  async function save() {
    setError("");
    try {
      if (editing) await api.updateUpstream(editing.id, form);
      else await api.createUpstream(form);
      setOpen(false);
      toast("上游已保存");
      await reload();
    } catch (e: any) {
      setError(e?.message || "保存失败");
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteUpstream(deleting.id);
      toast("已删除");
      setDeleting(null);
      await reload();
    } catch (e: any) {
      toast(e?.message || "删除失败", "error");
    }
  }

  async function test(u: Upstream) {
    setTesting((t) => ({ ...t, [u.id]: "测试中…" }));
    try {
      const r = await api.testUpstream(u.id);
      setTesting((t) => ({ ...t, [u.id]: `延迟 ${r.latency_ms}ms` }));
    } catch (e: any) {
      setTesting((t) => ({ ...t, [u.id]: e?.message || "失败" }));
    }
  }

  async function runImport(useURL: boolean) {
    setImporting(true);
    setResult(null);
    try {
      const r = await api.importUpstreams(useURL ? { url: importURL } : { text: importText });
      setResult(r);
      if (r.added > 0) {
        toast(`成功导入 ${r.added} 个上游`);
        await reload();
      } else {
        toast("没有新增上游（可能全部重复或无法识别）", "error");
      }
    } catch (e: any) {
      toast(e?.message || "导入失败", "error");
    } finally {
      setImporting(false);
    }
  }

  const refs = (id: string) =>
    data.ports.filter((p) => p.default_upstream === id).length +
    data.accounts.filter((a) => a.upstream_id === id).length;

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-lg font-semibold">上游代理</h1>
          <p className="text-xs text-muted-foreground">
            支持 http / socks5；密码可用 ${"{VAR}"} 引用环境变量；被引用的上游不可删除
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => { setImportOpen(true); setResult(null); }}>
            <Import className="h-4 w-4" /> 批量导入
          </Button>
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" /> 添加上游
          </Button>
        </div>
      </div>

      <Table>
        <THead>
          <TR>
            <TH>名称</TH>
            <TH>类型</TH>
            <TH>地址</TH>
            <TH>认证</TH>
            <TH>引用</TH>
            <TH>备注</TH>
            <TH>连通性</TH>
            <TH className="text-right">操作</TH>
          </TR>
        </THead>
        <TBody>
          {data.upstreams.length === 0 ? (
            <EmptyRow colSpan={8} text="暂无上游代理，可批量导入或手动添加" />
          ) : (
            data.upstreams.map((u) => (
              <TR key={u.id}>
                <TD className="font-medium">{u.name}</TD>
                <TD>
                  <Badge variant={u.type === "socks5" ? "warning" : "default"}>{u.type}</Badge>
                </TD>
                <TD className="font-mono text-xs">{u.host}</TD>
                <TD className="font-mono text-xs text-muted-foreground">{u.username || "-"}</TD>
                <TD className="text-xs">{refs(u.id)} 处</TD>
                <TD className="max-w-[140px] truncate text-xs text-muted-foreground">{u.remark || "-"}</TD>
                <TD className="text-xs">
                  {testing[u.id] ? (
                    <span className={testing[u.id].startsWith("延迟") ? "text-emerald-400" : "text-muted-foreground"}>
                      {testing[u.id]}
                    </span>
                  ) : (
                    <Button variant="ghost" size="sm" onClick={() => test(u)}>
                      <Zap className="h-3.5 w-3.5" /> 测试
                    </Button>
                  )}
                </TD>
                <TD>
                  <div className="flex justify-end gap-1">
                    <Button variant="ghost" size="sm" onClick={() => openEdit(u)}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeleting(u)}>
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </TD>
              </TR>
            ))
          )}
        </TBody>
      </Table>

      {/* 编辑/新增 */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader title={editing ? `编辑上游「${editing.name}」` : "添加上游代理"} />
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>名称</Label>
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="美国-家宽-1" />
            </div>
            <div className="space-y-1.5">
              <Label>地址</Label>
              <Input
                className="font-mono"
                value={form.upstream}
                onChange={(e) => setForm({ ...form, upstream: e.target.value })}
                placeholder="http://user:${UPSTREAM_PASS}@1.2.3.4:8080 或 socks5://1.2.3.4:1080"
              />
              <p className="text-[11px] text-muted-foreground">
                自动识别协议；支持 ${"{VAR}"} 或 ${"{VAR:-默认值}"} 环境变量引用，密码不落盘
              </p>
            </div>
            <div className="space-y-1.5">
              <Label>备注</Label>
              <Input value={form.remark} onChange={(e) => setForm({ ...form, remark: e.target.value })} />
            </div>
            {error ? <p className="text-xs text-destructive">{error}</p> : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>取消</Button>
            <Button onClick={save}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 批量导入 */}
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent wide>
          <DialogHeader
            title="批量导入上游（Resin 风格）"
            description="支持订阅链接 / URI 行 / IP:PORT / IP:PORT:USER:PASS / base64 订阅 / sing-box JSON / Clash 节点列表，自动去重"
          />
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label>订阅链接（http/https）</Label>
              <div className="flex gap-2">
                <Input
                  className="font-mono text-xs"
                  value={importURL}
                  onChange={(e) => setImportURL(e.target.value)}
                  placeholder="https://example.com/sub?token=..."
                />
                <Button variant="secondary" disabled={importing || !importURL} onClick={() => runImport(true)}>
                  <Gauge className="h-4 w-4" /> 拉取导入
                </Button>
              </div>
            </div>
            <div className="text-center text-[11px] text-muted-foreground">或直接粘贴文本</div>
            <div className="space-y-1.5">
              <Label>节点文本</Label>
              <Textarea
                className="min-h-[160px] font-mono text-xs"
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
                placeholder={`http://user:pass@1.2.3.4:8080#US-01\nsocks5://1.2.3.4:1080\n1.2.3.4:8080\n1.2.3.4:8080:user:pass`}
              />
            </div>
            {result ? (
              <div className="space-y-2 rounded-md border bg-background/40 p-3 text-xs">
                <div className="flex gap-3">
                  <Badge variant="success">新增 {result.added}</Badge>
                  <Badge variant="warning">重复 {result.duplicates.length}</Badge>
                  <Badge variant="destructive">跳过 {result.problems.length}</Badge>
                  <Badge variant="outline">共识别 {result.total}</Badge>
                </div>
                {result.duplicates.length > 0 && (
                  <details>
                    <summary className="cursor-pointer text-muted-foreground">重复项</summary>
                    <ul className="mt-1 max-h-24 overflow-y-auto font-mono text-[11px] text-muted-foreground">
                      {result.duplicates.map((d, i) => <li key={i}>{d}</li>)}
                    </ul>
                  </details>
                )}
                {result.problems.length > 0 && (
                  <details>
                    <summary className="cursor-pointer text-muted-foreground">跳过的行 / 节点</summary>
                    <ul className="mt-1 max-h-24 overflow-y-auto font-mono text-[11px] text-muted-foreground">
                      {result.problems.map((d, i) => <li key={i}>{d}</li>)}
                    </ul>
                  </details>
                )}
              </div>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setImportOpen(false)}>关闭</Button>
              <Button disabled={importing || !importText} onClick={() => runImport(false)}>
                {importing ? "导入中…" : "导入文本"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleting}
        title={`删除上游「${deleting?.name}」？`}
        description="仍被端口或账户引用时无法删除。"
        onConfirm={remove}
        onClose={() => setDeleting(null)}
      />
    </div>
  );
}
