import * as React from "react";
import { ArrowRight, Activity, Network, Users, Layers, Gauge } from "lucide-react";
import { api, type Data, type LogEntry } from "../api";
import { Badge, Button, Card, CardContent, CardDescription, CardHeader, CardTitle, TBody, TD, TH, THead, TR, Table } from "../components/ui";
import { ago, bytes } from "../lib/utils";
import type { CopyPreset, Page } from "../App";

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

  const firstAcc = data.accounts.find((a) => a.enabled);
  const firstPort = data.ports.find((p) => p.enabled);

  const cards = [
    { label: "端口线路", value: `${listening}/${data.ports.length}`, sub: "监听中 / 总数", icon: <Network className="h-4 w-4" /> },
    { label: "账户", value: `${data.accounts.filter((a) => a.enabled).length}/${data.accounts.length}`, sub: "启用 / 总数", icon: <Users className="h-4 w-4" /> },
    { label: "上游代理", value: String(data.upstreams.length), sub: "已配置", icon: <Layers className="h-4 w-4" /> },
    { label: "活跃连接", value: String(active), sub: "当前", icon: <Activity className="h-4 w-4" /> },
    { label: "累计流量", value: bytes(traffic), sub: "所有端口", icon: <Gauge className="h-4 w-4" /> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-lg font-semibold">仪表盘</h1>
          <p className="text-xs text-muted-foreground">转发状态总览与最近连接</p>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        {cards.map((c) => (
          <Card key={c.label}>
            <CardContent className="p-4">
              <div className="flex items-center justify-between text-muted-foreground">
                <span className="text-[11px]">{c.label}</span>
                {c.icon}
              </div>
              <div className="mt-2 text-xl font-semibold">{c.value}</div>
              <div className="text-[11px] text-muted-foreground">{c.sub}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center justify-between">
            <div>
              <CardTitle>最近连接</CardTitle>
              <CardDescription>每次连接一条记录，完整日志见「连接日志」</CardDescription>
            </div>
            <Button variant="ghost" size="sm" onClick={() => onNavigate("logs")}>
              查看全部 <ArrowRight className="h-3.5 w-3.5" />
            </Button>
          </CardHeader>
          <CardContent className="pt-0">
            <Table>
              <THead>
                <TR>
                  <TH>时间</TH>
                  <TH>端口</TH>
                  <TH>账户</TH>
                  <TH>目标</TH>
                  <TH>流量</TH>
                  <TH>状态</TH>
                </TR>
              </THead>
              <TBody>
                {logs.length === 0 ? (
                  <TR>
                    <TD colSpan={6} className="py-8 text-center text-xs text-muted-foreground">
                      暂无连接记录
                    </TD>
                  </TR>
                ) : (
                  logs.map((l, i) => (
                    <TR key={i}>
                      <TD className="whitespace-nowrap text-xs text-muted-foreground">{ago(l.time)}</TD>
                      <TD>{l.port || "-"}</TD>
                      <TD>{l.account || "-"}</TD>
                      <TD className="max-w-[220px] truncate font-mono text-[11px] text-muted-foreground">
                        {l.target || l.msg || "-"}
                      </TD>
                      <TD className="whitespace-nowrap text-xs">
                        ↑{bytes(l.bytes_out)} ↓{bytes(l.bytes_in)}
                      </TD>
                      <TD>
                        <StatusBadge status={l.status} />
                      </TD>
                    </TR>
                  ))
                )}
              </TBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>快速开始</CardTitle>
            <CardDescription>三分钟完成第一条转发线路</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-xs text-muted-foreground">
            <Step n={1} done={data.upstreams.length > 0}>
              添加上游代理（「上游代理」页，支持批量导入）
            </Step>
            <Step n={2} done={data.ports.length > 0}>
              新建端口线路并绑定默认上游（或留空直连）
            </Step>
            <Step n={3} done={data.accounts.length > 0}>
              创建账户，可绑定端口或全局可用
            </Step>
            <div className="rounded-md border bg-background/40 p-3">
              <p className="mb-2">最快方式：直接复制接入信息</p>
              <Button
                size="sm"
                className="w-full"
                onClick={() =>
                  onCopy(
                    firstPort
                      ? { dim: "port", id: firstPort.id }
                      : firstAcc
                      ? { dim: "account", id: firstAcc.id }
                      : null
                  )
                }
                disabled={!firstPort && !firstAcc}
              >
                打开复制向导
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Step({ n, done, children }: { n: number; done: boolean; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-2">
      <span
        className={
          "mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border text-[11px] " +
          (done ? "border-emerald-500/60 text-emerald-400" : "text-muted-foreground")
        }
      >
        {done ? "✓" : n}
      </span>
      <span className={done ? "line-through decoration-muted-foreground/40" : ""}>{children}</span>
    </div>
  );
}

export function StatusBadge({ status }: { status?: string }) {
  const s = status || "ok";
  const kind =
    s === "ok"
      ? "success"
      : s === "auth-failed"
      ? "warning"
      : "destructive";
  return <Badge variant={kind as any}>{s}</Badge>;
}
