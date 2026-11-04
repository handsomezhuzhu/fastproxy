import * as React from "react";
import {
  Activity,
  ArrowRight,
  ArrowUpDown,
  CircleCheck,
  CircleDashed,
  ChevronRight,
  Globe,
  Layers,
  Network,
  ScrollText,
  Users,
} from "lucide-react";
import { api, type Data, type LogEntry, type MetricPoint } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import { DotBadge, EmptyState, PageHeader, StatCard } from "@/components/page";
import { MetricsChart } from "@/components/MetricsChart";
import { useI18n } from "@/i18n";
import { clock, bytes, timeAgo } from "@/lib/utils";
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
  const { t, lang } = useI18n();
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
    const timer = window.setInterval(load, 5000);
    return () => {
      alive = false;
      window.clearInterval(timer);
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
    metrics === null ? null : metrics.reduce((s, p) => s + (p.bytes_in || 0) + (p.bytes_out || 0), 0);

  const firstPort = data.ports.find((p) => p.enabled);
  const firstAcc = data.accounts.find((a) => a.enabled);
  const copyFirst = () =>
    onCopy(
      firstPort ? { dim: "port", id: firstPort.id } : firstAcc ? { dim: "account", id: firstAcc.id } : null
    );

  const ratio = (a: number, b: number) => (
    <span>
      {a}
      <span className="text-base font-normal text-muted-foreground"> / {b}</span>
    </span>
  );

  const steps = [
    { n: 1, done: data.upstreams.length > 0, label: t("dashboard.quick.step1"), run: () => onNavigate("upstreams") },
    { n: 2, done: data.ports.length > 0, label: t("dashboard.quick.step2"), run: () => onNavigate("ports") },
    { n: 3, done: data.accounts.some((a) => a.enabled), label: t("dashboard.quick.step3"), run: copyFirst },
  ];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("dashboard.title")}
        description={t("dashboard.description")}
        actions={<Button onClick={copyFirst}>{t("dashboard.openCopy")}</Button>}
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6">
        <StatCard
          label={t("dashboard.stat.ports")}
          value={ratio(listening, data.ports.length)}
          hint={t("dashboard.stat.portsHint")}
          icon={<Network />}
        />
        <StatCard
          label={t("dashboard.stat.accounts")}
          value={ratio(enabledAccounts, data.accounts.length)}
          hint={t("dashboard.stat.accountsHint")}
          icon={<Users />}
        />
        <StatCard
          label={t("dashboard.stat.upstreams")}
          value={ratio(healthyUpstreams, data.upstreams.length)}
          hint={t("dashboard.stat.upstreamsHint")}
          icon={<Layers />}
        />
        <StatCard
          label={t("dashboard.stat.active")}
          value={active}
          hint={t("dashboard.stat.activeHint")}
          icon={<Activity />}
        />
        <StatCard
          label={t("dashboard.stat.traffic")}
          value={traffic24h === null ? <Skeleton className="h-8 w-20" /> : bytes(traffic24h)}
          hint={t("dashboard.stat.trafficHint")}
          icon={<ArrowUpDown />}
        />
        <StatCard
          label={t("dashboard.stat.ip")}
          value={
            <span className="font-mono text-xl font-semibold tracking-tight tabular-nums @[240px]/card:text-2xl">
              {data.probe?.server_ip || "–"}
            </span>
          }
          hint={
            data.probe?.last_check_at
              ? t("dashboard.stat.ipChecked", {
                  when: timeAgo(new Date(data.probe.last_check_at * 1000).toISOString(), lang),
                })
              : t("dashboard.stat.ipNever")
          }
          icon={<Globe />}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="@container/card lg:col-span-2">
          <CardHeader>
            <CardTitle>{t("dashboard.chart.title")}</CardTitle>
            <CardDescription>{t("dashboard.chart.desc")}</CardDescription>
          </CardHeader>
          <CardContent className="px-2 sm:px-5">
            {metrics === null ? (
              <Skeleton className="h-[240px] w-full" />
            ) : metrics.length === 0 ? (
              <div className="flex h-[240px] items-center justify-center text-sm text-muted-foreground">
                {t("dashboard.chart.empty")}
              </div>
            ) : (
              <MetricsChart points={metrics} />
            )}
          </CardContent>
        </Card>

        <Card className="flex flex-col">
          <CardHeader>
            <CardTitle>{t("dashboard.quick.title")}</CardTitle>
            <CardDescription>{t("dashboard.quick.desc")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-1 flex-col">
            <ItemGroup className="flex flex-1 flex-col justify-between gap-2.5">
              {steps.map((step) => (
                <Item
                  key={step.n}
                  variant="outline"
                  size="sm"
                  render={<button type="button" onClick={step.run} />}
                  className="w-full flex-1 text-left transition-colors hover:bg-muted/50"
                >
                  <ItemMedia variant="icon">
                    {step.done ? (
                      <CircleCheck className="text-emerald-600 dark:text-emerald-400" />
                    ) : (
                      <CircleDashed className="text-muted-foreground" />
                    )}
                  </ItemMedia>
                  <ItemContent>
                    <ItemTitle className="font-medium">{step.label}</ItemTitle>
                  </ItemContent>
                  <ItemActions>
                    <ChevronRight className="size-4 text-muted-foreground" aria-hidden="true" />
                  </ItemActions>
                </Item>
              ))}
            </ItemGroup>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.logs.title")}</CardTitle>
          <CardDescription>{t("dashboard.logs.desc")}</CardDescription>
          <CardAction>
            <Button variant="outline" size="sm" onClick={() => onNavigate("logs")}>
              {t("dashboard.logs.viewAll")}
              <ArrowRight data-icon="inline-end" />
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          {logs === null ? (
            <div className="flex flex-col gap-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </div>
          ) : logs.length === 0 ? (
            <EmptyState
              icon={<ScrollText />}
              title={t("dashboard.logs.empty")}
              description={t("dashboard.logs.emptyDesc")}
            />
          ) : (
            <div className="overflow-hidden rounded-lg border">
              <Table>
                <TableBody>
                  {logs.slice(0, 6).map((l, i) => (
                    <TableRow key={`${l.time}-${i}`}>
                      <TableCell className="w-28 font-mono text-xs tabular-nums text-muted-foreground">
                        {clock(l.time, lang)}
                      </TableCell>
                      <TableCell className="w-20">
                        <Badge
                          variant={l.level === "error" ? "secondary" : "outline"}
                          className={
                            l.level === "error"
                              ? "border border-foreground/40 font-medium text-foreground"
                              : l.level === "warn"
                                ? "bg-muted text-foreground"
                                : "text-muted-foreground"
                          }
                        >
                          {l.level}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-0 truncate" title={l.target || l.msg}>
                        {l.target ? <span className="font-mono text-xs">{l.target}</span> : l.msg || "–"}
                      </TableCell>
                      <TableCell className="w-32 text-right">
                        {l.status ? (
                          <DotBadge tone={l.status === "ok" ? "ok" : l.status === "auth-failed" ? "warn" : "fail"}>
                            {l.status}
                          </DotBadge>
                        ) : (
                          <span className="text-xs text-muted-foreground">–</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
