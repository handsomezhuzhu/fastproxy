import * as React from "react";
import { Activity, ArrowRight, ChevronRight, Layers, Network, Users } from "lucide-react";
import { api, type Data, type LogEntry } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { PageHeader, StatCard, EmptyState } from "@/components/page";
import { cn, clock, bytes } from "@/lib/utils";
import type { Page } from "@/App";
import type { CopyPreset } from "@/components/CopyWizard";

export function StatusBadge({ status }: { status?: string }) {
  const s = status || "ok";
  if (s === "ok") return <Badge variant="secondary">ok</Badge>;
  if (s === "auth-failed")
    return (
      <Badge className="bg-amber-500/15 text-amber-400" variant="secondary">
        auth-failed
      </Badge>
    );
  return <Badge variant="destructive">{s}</Badge>;
}

export function DashboardPage({
  data,
  onCopy,
  onNavigate,
}: {
  data: Data;
  onCopy: (p: CopyPreset) => void;
  onNavigate: (p: Page) => void;
}) {
  const [logs, setLogs] = React.useState<LogEntry[]>([]);

  React.useEffect(() => {
    let alive = true;
    const load = () =>
      api
        .logs({ limit: 8 })
        .then((r) => alive && setLogs(r.entries))
        .catch(() => {});
    load();
    const t = window.setInterval(load, 5000);
    return () => {
      alive = false;
      window.clearInterval(t);
    };
  }, []);

  const listening = data.status.ports.filter((p) => p.listening).length;
  const active = data.status.ports.reduce((s, p) => s + p.conns_cur, 0);
  const traffic = data.status.ports.reduce((s, p) => s + p.bytes_in + p.bytes_out, 0);
  const trafficIn = data.status.ports.reduce((s, p) => s + p.bytes_in, 0);
  const trafficOut = data.status.ports.reduce((s, p) => s + p.bytes_out, 0);

  const firstPort = data.ports.find((p) => p.enabled);
  const firstAcc = data.accounts.find((a) => a.enabled);

  return (
    <div className="space-y-6">
      <PageHeader
        title="仪表盘"
        description="转发状态总览、最近连接与快速上手。"
        actions={
          <Button onClick={() => onCopy(firstPort ? { dim: "port", id: firstPort.id } : firstAcc ? { dim: "account", id: firstAcc.id } : null)}>
            打开复制向导
          </Button>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatCard
          label="端口线路"
          value={
            <span>
              {listening}
              <span className="text-base font-normal text-muted-foreground"> / {data.ports.length}</span>
            </span>
          }
          hint="监听中 / 总数"
          icon={<Network className="size-4" />}
        />
        <StatCard
          label="账户"
          value={
            <span>
              {data.accounts.filter((a) => a.enabled).length}
              <span className="text-base font-normal text-muted-foreground"> / {data.accounts.length}</span>
            </span>
          }
          hint="启用 / 总数"
          icon={<Users className="size-4" />}
        />
        <StatCard label="上游代理" value={data.upstreams.length} hint="已配置" icon={<Layers className="size-4" />} />
        <StatCard label="活跃连接" value={active} hint="当前" icon={<Activity className="size-4" />} />
        <StatCard
          label="累计流量"
          value={bytes(traffic)}
          hint={`↑ ${bytes(trafficOut)} · ↓ ${bytes(trafficIn)}`}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <CardTitle>最近连接</CardTitle>
              <CardDescription>每条连接实时记录，完整历史见「连接日志」。</CardDescription>
            </div>
            <Button variant="ghost" size="sm" onClick={() => onNavigate("logs")}>
              查看全部
              <ArrowRight className="size-4" aria-hidden="true" />
            </Button>
          </CardHeader>
          <CardContent>
            {logs.length === 0 ? (
              <EmptyState
                icon={<Activity className="size-5" aria-hidden="true" />}
                title="暂无连接记录"
                description="配置端口与账户后，客户端连接会出现在这里。"
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>时间</TableHead>
                    <TableHead>端口</TableHead>
                    <TableHead>账户</TableHead>
                    <TableHead>目标</TableHead>
                    <TableHead className="text-right">流量</TableHead>
                    <TableHead>状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {logs.map((l, i) => (
                    <TableRow key={`${l.time}-${i}`}>
                      <TableCell className="whitespace-nowrap text-xs tabular-nums text-muted-foreground">
                        {clock(l.time)}
                      </TableCell>
                      <TableCell className="text-xs tabular-nums">{l.port || "–"}</TableCell>
                      <TableCell className="text-xs">{l.account || "–"}</TableCell>
                      <TableCell className="max-w-[220px] truncate font-mono text-xs text-muted-foreground" title={l.target || l.msg}>
                        {l.target || l.msg || "–"}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                        ↑{bytes(l.bytes_out)} ↓{bytes(l.bytes_in)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={l.status} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>快速开始</CardTitle>
            <CardDescription>三步建立第一条转发线路。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {[
              { n: 1, done: data.upstreams.length > 0, label: "添加上游代理（支持批量导入）", page: "upstreams" as Page },
              { n: 2, done: data.ports.length > 0, label: "新建端口线路并绑定上游", page: "ports" as Page },
              { n: 3, done: data.accounts.length > 0, label: "创建账户（可绑定端口）", page: "accounts" as Page },
            ].map((step) => (
              <button
                key={step.n}
                type="button"
                onClick={() => onNavigate(step.page)}
                className="flex w-full items-center gap-3 rounded-lg p-1.5 text-left transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
              >
                <span
                  className={cn(
                    "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs tabular-nums",
                    step.done ? "border-emerald-500/60 text-emerald-400" : "text-muted-foreground"
                  )}
                  aria-hidden="true"
                >
                  {step.done ? "✓" : step.n}
                </span>
                <span className={cn("min-w-0 flex-1 text-sm", step.done && "text-muted-foreground line-through decoration-muted-foreground/40")}>
                  {step.label}
                </span>
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              </button>
            ))}
            <div className="rounded-lg border bg-muted/30 p-3 text-xs text-muted-foreground">
              客户端地址格式：
              <code className="mx-1 rounded bg-background px-1 py-0.5 font-mono">http://账户:密码@{window.location.hostname}:端口</code>
              同端口同时支持 HTTP 与 SOCKS5。
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
