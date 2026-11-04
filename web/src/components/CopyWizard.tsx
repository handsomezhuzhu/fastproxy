import * as React from "react";
import { Check, Copy } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
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
import { useI18n } from "@/i18n";
import type { T } from "@/i18n";
import { cn } from "@/lib/utils";
import type { Account, Data, Port } from "@/api";

export type CopyPreset = { dim: "port" | "account"; id: string } | null;

const HOST = window.location.hostname;
const enc = encodeURIComponent;

type Dim = "port" | "account";

type Format = {
  id: string;
  labelKey: string;
  build: (port: number, user: string, pass: string, t: T) => string;
};

const FORMATS: Format[] = [
  { id: "http", labelKey: "copy.format.http", build: (p, u, pw) => `http://${enc(u)}:${enc(pw)}@${HOST}:${p}` },
  {
    id: "socks5",
    labelKey: "copy.format.socks5",
    build: (p, u, pw) => `socks5h://${enc(u)}:${enc(pw)}@${HOST}:${p}`,
  },
  {
    id: "curl-http",
    labelKey: "copy.format.curlHttp",
    build: (p, u, pw) => `curl -x "http://${enc(u)}:${enc(pw)}@${HOST}:${p}" https://api.ipify.org`,
  },
  {
    id: "curl-socks5",
    labelKey: "copy.format.curlSocks5",
    build: (p, u, pw) => `curl --proxy "socks5h://${enc(u)}:${enc(pw)}@${HOST}:${p}" https://api.ipify.org`,
  },
  {
    id: "split",
    labelKey: "copy.format.split",
    build: (p, u, pw, t) =>
      [
        `${t("copy.splitLabels.host")}: ${HOST}`,
        `${t("copy.splitLabels.port")}: ${p}`,
        `${t("copy.splitLabels.user")}: ${u}`,
        `${t("copy.splitLabels.pass")}: ${pw}`,
      ].join("\n"),
  },
  {
    id: "json",
    labelKey: "copy.format.json",
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
  const { t } = useI18n();
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
  const text = pairs.map((p) => fmt.build(p.port, p.user, p.pass, t)).join(format === "json" ? ",\n" : "\n\n");

  const portItems = ports.map((p) => ({
    value: p.id,
    label: `${t("copy.portOption", { port: p.port })}${p.remark ? ` · ${p.remark}` : ""}`,
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
          label: t("copy.portOption", { port: p.port }),
        }));

  const allOn = candidates.length > 0 && candidates.every((c) => selected[c.id]);

  async function copy() {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success(t("copy.done", { n: pairs.length }));
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      toast.error(t("common.copyFailed"));
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("copy.title")}</DialogTitle>
          <DialogDescription>{t("copy.desc")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-5">
          <section aria-labelledby="step-1" className="flex flex-col gap-2">
            <Label id="step-1">{t("copy.step1")}</Label>
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
                <TabsTrigger value="port">{t("copy.byPort")}</TabsTrigger>
                <TabsTrigger value="account">{t("copy.byAccount")}</TabsTrigger>
              </TabsList>
            </Tabs>
          </section>

          <section aria-labelledby="step-2" className="flex flex-col gap-2">
            <Label id="step-2">{dim === "port" ? t("copy.step2Port") : t("copy.step2Account")}</Label>
            {dim === "port" ? (
              <FormSelect
                aria-label={t("copy.selectPort")}
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
                aria-label={t("copy.selectAccount")}
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
                  {dim === "port" ? t("copy.availableAccounts") : t("copy.availablePorts")}
                </span>
                <Button
                  type="button"
                  variant="link"
                  size="xs"
                  onClick={() =>
                    setSelected(allOn ? {} : Object.fromEntries(candidates.map((c) => [c.id, true])))
                  }
                >
                  {allOn ? t("copy.clearAll") : t("copy.selectAll")}
                </Button>
              </div>
              <ScrollArea className="max-h-32">
                <div className="grid grid-cols-2 gap-x-3 gap-y-1">
                  {candidates.length === 0 ? (
                    <p className="col-span-2 py-2 text-xs text-muted-foreground">{t("copy.noOptions")}</p>
                  ) : (
                    candidates.map((c) => (
                      <label
                        key={c.id}
                        className="flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-sm hover:bg-muted"
                      >
                        <Checkbox
                          checked={!!selected[c.id]}
                          onCheckedChange={(checked: boolean) =>
                            setSelected((s) => ({ ...s, [c.id]: checked }))
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

          <section aria-labelledby="step-3" className="flex flex-col gap-2">
            <Label id="step-3">{t("copy.step3")}</Label>
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
                  {t(f.labelKey)}
                </button>
              ))}
            </div>
          </section>

          <section className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="copy-preview" aria-live="polite">
                {t("copy.preview", { n: pairs.length, host: HOST })}
              </Label>
              <Button size="sm" variant="ghost" onClick={copy} disabled={!text}>
                {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                {copied ? t("common.copied") : t("common.copy")}
              </Button>
            </div>
            <Textarea
              id="copy-preview"
              readOnly
              rows={7}
              value={text}
              className="font-mono text-xs"
              placeholder={t("copy.previewEmpty")}
            />
          </section>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
          <Button onClick={copy} disabled={!text}>
            <Copy className="size-4" aria-hidden="true" />
            {t("copy.copyAll")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
