import * as React from "react";
import {
  Activity,
  ArrowRight,
  ArrowUpDown,
  ChevronRight,
  Globe,
  Layers,
  Network,
  Users,
} from "lucide-react";
import { api, type Data, type LogEntry, type MetricPoint } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { DotBadge, PageHeader, StatCard } from "@/components/page";
import { MetricsChart } from "@/components/MetricsChart";
import { cn, clock, bytes, timeAgo } from "@/lib/utils";
import type { Page } from "@/App";
import type { CopyPreset } from "@/components/CopyWizard";

export function DashboardPage({
  data,
  onCopy,
  onNavigate,
}: {
  data: Data;
  onCopy: (p: CopyPreset) => void;
  onNavigate: (p: Page) => void;
}) {
  const [logs, setLogs] = React.useState<LogEntry[] | null>(null);
  const [metrics, setMetrics] = React.useState<MetricPoint[] | null>(null);

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

  React.useEffect(() => {
    let alive = true;
    api
      .metricsHours()
      .then((r) => alive && setMetrics(r.points))
      .catch(() => alive && setMetrics([]));
    return () => {
      alive = false;
    };
  }, []);

  const listening = data.status.ports.filter((p) => p.listening).length;
  const enabledAccounts = data.accounts.filter((a) => a.enabled).length;
  const healthyUpstreams = data.upstreams.filter((u) => u.health === "ok").length;
  const active = data.status.ports.reduce((s, p) => s + p.conns_cur, 0);
  const traffic24h =
    metrics === null
      ? null
      : metrics.reduce((s, p) => s + (p.bytes_in || 0) + (p.bytes_out || 0), 0);

  const firstPort = data.ports.find((p) => p.enabled);
  const firstAcc = data.accounts.find((a) => a.enabled);

  return (
    <div className="space-y-6">
      <PageHeader
        title="仪表盘"
        description="转发状态总览与快速上手。"
        actions={
          <Button
            onClick={() =>
              onCopy(
                firstPort
                  ? { dim: "port", id: firstPort.id }
                  : firstAcc
                    ? { dim: "account", id: firstAcc.id }
                    : null
              )
            }
          >
            打开复制向导
          </Button>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
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
              {enabledAccounts}
              <span className="text-base font-normal text-muted-foreground"> / {data.accounts.length}</span>
            </span>
          }
          hint="启用 / 总数"
          icon={<Users className="size-4" />}
        />
        <StatCard
          label="上游健康"
          value={
            <span>
              {healthyUpstreams}
              <span className="text-base font-normal text-muted-foreground"> / {data.upstreams.length}</span>
            </span>
          }
          hint="正常 / 总数"
          icon={<Layers className="size-4" />}
        />
        <StatCard label="活跃连接" value={active} hint="当前" icon={<Activity className="size-4" />} />
        <StatCard
          label="24h 流量"
          value={traffic24h === null ? <Skeleton className="mt-1 h-7 w-20" /> : bytes(traffic24h)}
          hint="近 24 小时进出合计"
          icon={<ArrowUpDown className="size-4" />}
        />
        <StatCard
          label="本机出口 IP"
          value={<span className="font-mono text-lg tabular-nums">{data.probe?.server_ip || "–"}</span>}
          hint={
            data.probe?.last_check_at
              ? `检测于 ${timeAgo(new Date(data.probe.last_check_at * 1000).toISOString())}`
              : "尚未检测"
          }
          icon={<Globe className="size-4" />}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <CardTitle>近 24 小时</CardTitle>
              <CardDescription>连接数与字节传输趋势（各自归一化）。</CardDescription>
            </div>
            <div className="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <span className="size-2 rounded-full bg-chart-1" aria-hidden="true" />
                连接数
              </span>
              <span className="flex items-center gap-1.5">
                <span className="size-2 rounded-full bg-chart-2" aria-hidden="true" />
                流量
              </span>
            </div>
          </CardHeader>
          <CardContent>
            {metrics === null ? (
              <Skeleton className="h-[220px] w-full" />
            ) : metrics.length === 0 ? (
              <div className="flex h-[220px] items-center justify-center text-sm text-muted-foreground">
                暂无统计数据，稍后再看…
              </div>
            ) : (
              <MetricsChart points={metrics} />
            )}
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>快速开始</CardTitle>
              <CardDescription>三步建立第一条转发线路。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {[
                {
                  n: 1,
                  done: data.upstreams.length > 0,
                  label: "添加上游代理（支持批量导入）",
                  page: "upstreams" as Page,
                },
                {
                  n: 2,
                  done: data.ports.length > 0,
                  label: "新建端口线路（自动生成默认账号）",
                  page: "ports" as Page,
                },
                {
                  n: 3,
                  done: data.accounts.some((a) => a.enabled),
                  label: "复制接入信息给客户端",
                  action: "copy" as const,
                },
              ].map((step) => (
                <button
                  key={step.n}
                  type="button"
                  onClick={() =>
                    step.action === "copy"
                      ? onCopy(
                          firstPort
                            ? { dim: "port", id: firstPort.id }
                            : firstAcc
                              ? { dim: "account", id: firstAcc.id }
                              : null
                        )
                      : onNavigate(step.page!)
                  }
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
                  <span
                    className={cn(
                      "min-w-0 flex-1 text-sm",
                      step.done && "text-muted-foreground line-through decoration-muted-foreground/40"
                    )}
                  >
                    {step.label}
                  </span>
                  <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                </button>
              ))}
            </CardContent>
          </Card>

          <Card className="flex-1">
            <CardHeader className="flex-row items-start justify-between gap-3">
              <div className="space-y-1">
                <CardTitle>最近连接</CardTitle>
                <CardDescription>完整历史见「连接日志」。</CardDescription>
              </div>
              <Button variant="ghost" size="sm" onClick={() => onNavigate("logs")}>
                查看全部
                <ArrowRight className="size-4" aria-hidden="true" />
              </Button>
            </CardHeader>
            <CardContent>
              {logs === null ? (
                <div className="space-y-2">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <Skeleton key={i} className="h-7 w-full" />
                  ))}
                </div>
              ) : logs.length === 0 ? (
                <div className="py-6 text-center">
                  <p className="text-sm font-medium">暂无连接记录</p>
                  <p className="mt-1 text-xs text-muted-foreground">配置端口与账户后，客户端连接会出现在这里。</p>
                </div>
              ) : (
                <ul className="divide-y">
                  {logs.slice(0, 6).map((l, i) => (
                    <li key={`${l.time}-${i}`} className="flex items-center gap-3 py-2">
                      <span className="w-16 shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
                        {clock(l.time)}
                      </span>
                      <span className="min-w-0 flex-1 truncate font-mono text-xs" title={l.target || l.msg}>
                        {l.target || l.msg || "–"}
                      </span>
                      {l.status ? (
                        <DotBadge tone={l.status === "ok" ? "ok" : l.status === "auth-failed" ? "warn" : "fail"}>
                          {l.status}
                        </DotBadge>
                      ) : (
                        <span className="text-xs text-muted-foreground">–</span>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
