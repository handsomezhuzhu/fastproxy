import * as React from "react";
import { Check, Copy } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { FormSelect } from "@/components/page";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import type { Account, Data, Port } from "@/api";

export type CopyPreset = { dim: "port" | "account"; id: string } | null;

const HOST = window.location.hostname;
const enc = encodeURIComponent;

type Dim = "port" | "account";

type Format = {
  id: string;
  label: string;
  build: (port: number, user: string, pass: string) => string;
};

const FORMATS: Format[] = [
  { id: "http", label: "HTTP 代理 URL", build: (p, u, pw) => `http://${enc(u)}:${enc(pw)}@${HOST}:${p}` },
  {
    id: "socks5",
    label: "SOCKS5 代理 URL",
    build: (p, u, pw) => `socks5h://${enc(u)}:${enc(pw)}@${HOST}:${p}`,
  },
  {
    id: "curl-http",
    label: "curl（HTTP）",
    build: (p, u, pw) => `curl -x "http://${enc(u)}:${enc(pw)}@${HOST}:${p}" https://api.ipify.org`,
  },
  {
    id: "curl-socks5",
    label: "curl（SOCKS5）",
    build: (p, u, pw) =>
      `curl --proxy "socks5h://${enc(u)}:${enc(pw)}@${HOST}:${p}" https://api.ipify.org`,
  },
  {
    id: "split",
    label: "分字段",
    build: (p, u, pw) => `主机: ${HOST}\n端口: ${p}\n账号: ${u}\n密码: ${pw}`,
  },
  {
    id: "json",
    label: "JSON 配置",
    build: (p, u, pw) =>
      JSON.stringify({ host: HOST, port: p, username: u, password: pw, protocol: "http" }, null, 2),
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
  preset: CopyPreset;
}) {
  const ports = React.useMemo(() => data.ports.filter((p) => p.enabled), [data.ports]);
  const accounts = React.useMemo(() => data.accounts.filter((a) => a.enabled), [data.accounts]);

  const [dim, setDim] = React.useState<Dim>("port");
  const [portID, setPortID] = React.useState("");
  const [accountID, setAccountID] = React.useState("");
  const [selected, setSelected] = React.useState<Record<string, boolean>>({});
  const [format, setFormat] = React.useState("http");
  const [copied, setCopied] = React.useState(false);

  const eligibleAccounts = React.useCallback(
    // 账户必绑定端口；默认账号（is_default）同样作为候选出现。
    (port?: Port) => (port ? accounts.filter((a) => a.port_id === port.id) : []),
    [accounts]
  );
  const eligiblePorts = React.useCallback(
    // 按账户维度仅其绑定的那一个端口可用。
    (account?: Account) => (account ? ports.filter((p) => p.id === account.port_id) : []),
    [ports]
  );

  const applyDefault = React.useCallback(
    (next: Dim, presetID?: string) => {
      if (next === "port") {
        const p = presetID || ports[0]?.id || "";
        setPortID(p);
        const target = ports.find((x) => x.id === p);
        setSelected(Object.fromEntries(eligibleAccounts(target).map((a) => [a.id, true])));
      } else {
        const a = presetID || accounts[0]?.id || "";
        setAccountID(a);
        const target = accounts.find((x) => x.id === a);
        setSelected(Object.fromEntries(eligiblePorts(target).map((p) => [p.id, true])));
      }
    },
    [accounts, ports, eligibleAccounts, eligiblePorts]
  );

  React.useEffect(() => {
    if (!open) return;
    setCopied(false);
    const next = preset?.dim ?? "port";
    setDim(next);
    applyDefault(next, preset?.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, preset]);

  const pairs = React.useMemo(() => {
    const out: { port: number; user: string; pass: string }[] = [];
    if (dim === "port") {
      const port = ports.find((p) => p.id === portID);
      if (!port) return out;
      for (const acc of eligibleAccounts(port)) {
        if (selected[acc.id]) out.push({ port: port.port, user: acc.username, pass: acc.password });
      }
    } else {
      const acc = accounts.find((a) => a.id === accountID);
      if (!acc) return out;
      for (const port of eligiblePorts(acc)) {
        if (selected[port.id]) out.push({ port: port.port, user: acc.username, pass: acc.password });
      }
    }
    return out;
  }, [dim, portID, accountID, selected, ports, accounts, eligibleAccounts, eligiblePorts]);

  const fmt = FORMATS.find((f) => f.id === format) ?? FORMATS[0];
  const text = pairs.map((p) => fmt.build(p.port, p.user, p.pass)).join(format === "json" ? ",\n" : "\n\n");

  const portItems = ports.map((p) => ({
    value: p.id,
    label: `端口 ${p.port}${p.remark ? ` · ${p.remark}` : ""}`,
  }));
  const accountItems = accounts.map((a) => ({
    value: a.id,
    label: `${a.username}${a.remark ? ` · ${a.remark}` : ""}`,
  }));

  const candidates: { id: string; label: string }[] =
    dim === "port"
      ? eligibleAccounts(ports.find((p) => p.id === portID)).map((a) => ({
          id: a.id,
          label: a.username,
        }))
      : eligiblePorts(accounts.find((x) => x.id === accountID)).map((p) => ({
          id: p.id,
          label: `端口 ${p.port}`,
        }));

  const allOn = candidates.length > 0 && candidates.every((c) => selected[c.id]);

  async function copy() {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success(`已复制 ${pairs.length} 条接入信息`);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      toast.error("复制失败，请手动选择文本复制");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>复制接入信息</DialogTitle>
          <DialogDescription>
            选择维度 → 选择对象 → 选择格式，一键复制给客户端使用。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <section aria-labelledby="step-1" className="space-y-2">
            <Label id="step-1">第 1 步 · 复制维度</Label>
            <Tabs
              value={dim}
              onValueChange={(v) => {
                const next = (v as Dim) ?? "port";
                setDim(next);
                setCopied(false);
                applyDefault(next);
              }}
            >
              <TabsList>
                <TabsTrigger value="port">按端口</TabsTrigger>
                <TabsTrigger value="account">按账户</TabsTrigger>
              </TabsList>
            </Tabs>
          </section>

          <section aria-labelledby="step-2" className="space-y-2">
            <Label id="step-2">第 2 步 · 选择{dim === "port" ? "端口" : "账户"}</Label>
            {dim === "port" ? (
              <FormSelect
                aria-label="选择端口"
                value={portID}
                items={portItems}
                onValueChange={(id) => {
                  setPortID(id);
                  const target = ports.find((x) => x.id === id);
                  setSelected(Object.fromEntries(eligibleAccounts(target).map((a) => [a.id, true])));
                }}
              />
            ) : (
              <FormSelect
                aria-label="选择账户"
                value={accountID}
                items={accountItems}
                onValueChange={(id) => {
                  setAccountID(id);
                  const target = accounts.find((x) => x.id === id);
                  setSelected(Object.fromEntries(eligiblePorts(target).map((p) => [p.id, true])));
                }}
              />
            )}

            <div className="rounded-lg border bg-muted/30 p-3">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  {dim === "port" ? "可用账户（可多选）" : "可用端口（可多选）"}
                </span>
                <Button
                  type="button"
                  variant="link"
                  size="xs"
                  onClick={() =>
                    setSelected(allOn ? {} : Object.fromEntries(candidates.map((c) => [c.id, true])))
                  }
                >
                  {allOn ? "取消全选" : "全选"}
                </Button>
              </div>
              <ScrollArea className="max-h-32">
                <div className="grid grid-cols-2 gap-x-3 gap-y-1">
                  {candidates.length === 0 ? (
                    <p className="col-span-2 py-2 text-xs text-muted-foreground">暂无可选项</p>
                  ) : (
                    candidates.map((c) => (
                      <label
                        key={c.id}
                        className="flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-sm hover:bg-muted"
                      >
                        <input
                          type="checkbox"
                          className="size-3.5 accent-primary"
                          checked={!!selected[c.id]}
                          onChange={(e) =>
                            setSelected((s) => ({ ...s, [c.id]: e.target.checked }))
                          }
                        />
                        <span className="truncate">{c.label}</span>
                      </label>
                    ))
                  )}
                </div>
              </ScrollArea>
            </div>
          </section>

          <section aria-labelledby="step-3" className="space-y-2">
            <Label id="step-3">第 3 步 · 复制格式</Label>
            <div className="grid grid-cols-2 gap-2">
              {FORMATS.map((f) => (
                <button
                  key={f.id}
                  type="button"
                  aria-pressed={format === f.id}
                  onClick={() => {
                    setFormat(f.id);
                    setCopied(false);
                  }}
                  className={cn(
                    "rounded-lg border px-3 py-2 text-left text-sm transition-colors",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                    format === f.id
                      ? "border-primary/60 bg-primary/10"
                      : "hover:bg-muted"
                  )}
                >
                  {f.label}
                </button>
              ))}
            </div>
          </section>

          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="copy-preview" aria-live="polite">
                预览（{pairs.length} 条 · 主机 {HOST}）
              </Label>
              <Button size="sm" variant="ghost" onClick={copy} disabled={!text}>
                {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                {copied ? "已复制" : "复制"}
              </Button>
            </div>
            <Textarea
              id="copy-preview"
              readOnly
              rows={7}
              value={text}
              className="font-mono text-xs"
              placeholder="选择端口与账户后生成接入信息…"
            />
          </section>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
          <Button onClick={copy} disabled={!text}>
            <Copy className="size-4" aria-hidden="true" />
            复制全部
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
