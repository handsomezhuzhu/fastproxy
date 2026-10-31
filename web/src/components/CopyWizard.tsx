import * as React from "react";
import { Copy, Check } from "lucide-react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  Label,
  Select,
  Textarea,
} from "./ui";
import type { Account, Data, Port } from "../api";
import { toast } from "./Toast";
import { cn } from "../lib/utils";

const HOST = window.location.hostname;

type Dim = "port" | "account";

type Format = {
  id: string;
  label: string;
  build: (host: string, port: number, user: string, pass: string) => string;
};

const enc = encodeURIComponent;

const FORMATS: Format[] = [
  {
    id: "http",
    label: "HTTP 代理 URL",
    build: (h, p, u, pw) => `http://${enc(u)}:${enc(pw)}@${h}:${p}`,
  },
  {
    id: "socks5",
    label: "SOCKS5 代理 URL",
    build: (h, p, u, pw) => `socks5h://${enc(u)}:${enc(pw)}@${h}:${p}`,
  },
  {
    id: "curl-http",
    label: "curl 命令（HTTP）",
    build: (h, p, u, pw) => `curl -x "http://${enc(u)}:${enc(pw)}@${h}:${p}" https://api.ipify.org`,
  },
  {
    id: "curl-socks5",
    label: "curl 命令（SOCKS5）",
    build: (h, p, u, pw) => `curl --proxy "socks5h://${enc(u)}:${enc(pw)}@${h}:${p}" https://api.ipify.org`,
  },
  {
    id: "split",
    label: "分字段（主机 / 端口 / 账号 / 密码）",
    build: (h, p, u, pw) => `主机: ${h}\n端口: ${p}\n账号: ${u}\n密码: ${pw}`,
  },
  {
    id: "json",
    label: "JSON 配置",
    build: (h, p, u, pw) =>
      JSON.stringify({ host: h, port: p, username: u, password: pw, protocol: "http" }, null, 2),
  },
];

export function CopyWizard({
  open,
  onOpenChange,
  data,
  preset,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  data: Data;
  preset?: { dim: Dim; id: string } | null;
}) {
  const [dim, setDim] = React.useState<Dim>("port");
  const [portID, setPortID] = React.useState("");
  const [accountID, setAccountID] = React.useState("");
  const [selected, setSelected] = React.useState<Record<string, boolean>>({});
  const [format, setFormat] = React.useState("http");
  const [copied, setCopied] = React.useState(false);

  const ports = data.ports.filter((p) => p.enabled);
  const accounts = data.accounts.filter((a) => a.enabled);

  React.useEffect(() => {
    if (!open) return;
    const d = preset?.dim ?? "port";
    setDim(d);
    setCopied(false);
    if (d === "port") {
      const p = preset?.id || ports[0]?.id || "";
      setPortID(p);
      setAccountID("");
      const target = ports.find((x) => x.id === p);
      setSelected(Object.fromEntries(eligibleAccounts(target).map((a) => [a.id, true])));
    } else {
      const a = preset?.id || accounts[0]?.id || "";
      setAccountID(a);
      setPortID("");
      const target = accounts.find((x) => x.id === a);
      setSelected(Object.fromEntries(eligiblePorts(target).map((p) => [p.id, true])));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, preset]);

  function eligibleAccounts(port?: Port): Account[] {
    if (!port) return [];
    return accounts.filter((a) => a.port_id === "" || a.port_id === port.id);
  }

  function eligiblePorts(account?: Account): Port[] {
    if (!account) return [];
    return ports.filter((p) => account.port_id === "" || account.port_id === p.id);
  }

  function switchDim(next: Dim) {
    setDim(next);
    setCopied(false);
    if (next === "port") {
      const p = ports[0]?.id || "";
      setPortID(p);
      const target = ports.find((x) => x.id === p);
      setSelected(Object.fromEntries(eligibleAccounts(target).map((a) => [a.id, true])));
    } else {
      const a = accounts[0]?.id || "";
      setAccountID(a);
      const target = accounts.find((x) => x.id === a);
      setSelected(Object.fromEntries(eligiblePorts(target).map((p) => [p.id, true])));
    }
  }

  const fmt = FORMATS.find((f) => f.id === format)!;

  const lines = React.useMemo(() => {
    const out: string[] = [];
    if (dim === "port") {
      const port = ports.find((p) => p.id === portID);
      if (!port) return out;
      for (const acc of eligibleAccounts(port)) {
        if (!selected[acc.id]) continue;
        out.push(fmt.build(HOST, port.port, acc.username, acc.password));
      }
    } else {
      const acc = accounts.find((a) => a.id === accountID);
      if (!acc) return out;
      for (const port of eligiblePorts(acc)) {
        if (!selected[port.id]) continue;
        out.push(fmt.build(HOST, port.port, acc.username, acc.password));
      }
    }
    return out;
  }, [dim, portID, accountID, selected, format, data]);

  const text = lines.join(format === "json" ? ",\n" : "\n\n");

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast(`已复制 ${lines.length} 条`);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      toast("复制失败，请手动复制", "error");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide>
        <DialogHeader
          title="复制接入信息"
          description="选择维度 → 选择对象 → 选择格式，一键复制给客户端使用"
        />

        <div className="space-y-5">
          <div>
            <Label className="mb-2 block">第 1 步 · 复制维度</Label>
            <div className="inline-flex overflow-hidden rounded-md border">
              {(
                [
                  ["port", "按端口"],
                  ["account", "按账户"],
                ] as [Dim, string][]
              ).map(([value, label]) => (
                <button
                  key={value}
                  onClick={() => switchDim(value)}
                  className={cn(
                    "px-4 py-1.5 text-xs",
                    dim === value ? "bg-primary text-primary-foreground" : "hover:bg-accent"
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <Label className="mb-2 block">
              第 2 步 · 选择{dim === "port" ? "端口" : "账户"}
            </Label>
            <div className="flex gap-2">
              {dim === "port" ? (
                <Select
                  value={portID}
                  onChange={(e) => {
                    const id = e.target.value;
                    setPortID(id);
                    const target = ports.find((x) => x.id === id);
                    setSelected(Object.fromEntries(eligibleAccounts(target).map((a) => [a.id, true])));
                  }}
                >
                  {ports.map((p) => (
                    <option key={p.id} value={p.id}>
                      端口 {p.port} {p.remark ? `(${p.remark})` : ""}
                    </option>
                  ))}
                </Select>
              ) : (
                <Select
                  value={accountID}
                  onChange={(e) => {
                    const id = e.target.value;
                    setAccountID(id);
                    const target = accounts.find((x) => x.id === id);
                    setSelected(Object.fromEntries(eligiblePorts(target).map((p) => [p.id, true])));
                  }}
                >
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.username} {a.remark ? `(${a.remark})` : ""}
                    </option>
                  ))}
                </Select>
              )}
            </div>

            <div className="mt-3 space-y-2 rounded-md border bg-background/40 p-3">
              <div className="flex items-center justify-between">
                <Label>
                  {dim === "port" ? "可用账户（可多选）" : "可用端口（可多选）"}
                </Label>
                <button
                  className="text-[11px] text-primary hover:underline"
                  onClick={() => {
                    const items = dim === "port" ? eligibleAccounts(ports.find((p) => p.id === portID)) : eligiblePorts(accounts.find((a) => a.id === accountID));
                    const allOn = items.every((x) => selected[x.id]);
                    setSelected(allOn ? {} : Object.fromEntries(items.map((x) => [x.id, true])));
                  }}
                >
                  全选 / 反选
                </button>
              </div>
              <div className="grid max-h-32 grid-cols-2 gap-1 overflow-y-auto text-xs">
                {(dim === "port" ? eligibleAccounts(ports.find((p) => p.id === portID)) : eligiblePorts(accounts.find((a) => a.id === accountID))).map((item: any) => (
                  <label key={item.id} className="flex cursor-pointer items-center gap-2 rounded px-1 py-1 hover:bg-accent/60">
                    <input
                      type="checkbox"
                      className="accent-[hsl(var(--primary))]"
                      checked={!!selected[item.id]}
                      onChange={(e) => setSelected((s) => ({ ...s, [item.id]: e.target.checked }))}
                    />
                    {dim === "port" ? `${item.username}` : `端口 ${item.port}`}
                  </label>
                ))}
                {(dim === "port" ? eligibleAccounts(ports.find((p) => p.id === portID)) : eligiblePorts(accounts.find((a) => a.id === accountID))).length === 0 && (
                  <span className="text-muted-foreground">无可选项</span>
                )}
              </div>
            </div>
          </div>

          <div>
            <Label className="mb-2 block">第 3 步 · 复制格式</Label>
            <div className="grid grid-cols-2 gap-1.5">
              {FORMATS.map((f) => (
                <button
                  key={f.id}
                  onClick={() => {
                    setFormat(f.id);
                    setCopied(false);
                  }}
                  className={cn(
                    "rounded-md border px-3 py-2 text-left text-xs transition",
                    format === f.id ? "border-primary bg-primary/10 text-foreground" : "hover:bg-accent"
                  )}
                >
                  {f.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between">
              <Label>预览（{lines.length} 条，主机 {HOST}）</Label>
              <Button variant="ghost" size="sm" onClick={copy} disabled={!text}>
                {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                {copied ? "已复制" : "复制"}
              </Button>
            </div>
            <Textarea readOnly value={text} rows={7} className="font-mono text-xs" />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>
          <Button onClick={copy} disabled={!text}>
            <Copy className="h-4 w-4" /> 复制全部
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
