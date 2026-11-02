import * as React from "react";
import {
  Activity,
  ArrowRight,
  ArrowUpDown,
  Box,
  ChevronDown,
  ChevronRight,
  Globe,
  Layers,
  Network,
  RefreshCw,
  Users,
} from "lucide-react";
import { api, type Data, type LogEntry, type MetricPoint, type SingboxStatus } from "@/api";
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
import { useI18n } from "@/i18n";
import { cn, clock, bytes, timeAgo, dateTime } from "@/lib/utils";
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
    metrics === null
      ? null
      : metrics.reduce((s, p) => s + (p.bytes_in || 0) + (p.bytes_out || 0), 0);

  const firstPort = data.ports.find((p) => p.enabled);
  const firstAcc = data.accounts.find((a) => a.enabled);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("dashboard.title")}
        description={t("dashboard.description")}
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
            {t("dashboard.openCopy")}
          </Button>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        <StatCard
          label={t("dashboard.stat.ports")}
          value={
            <span>
              {listening}
              <span className="text-base font-normal text-muted-foreground"> / {data.ports.length}</span>
            </span>
          }
          hint={t("dashboard.stat.portsHint")}
          icon={<Network className="size-4" />}
        />
        <StatCard
          label={t("dashboard.stat.accounts")}
          value={
            <span>
              {enabledAccounts}
              <span className="text-base font-normal text-muted-foreground"> / {data.accounts.length}</span>
            </span>
          }
          hint={t("dashboard.stat.accountsHint")}
          icon={<Users className="size-4" />}
        />
        <StatCard
          label={t("dashboard.stat.upstreams")}
          value={
            <span>
              {healthyUpstreams}
              <span className="text-base font-normal text-muted-foreground"> / {data.upstreams.length}</span>
            </span>
          }
          hint={t("dashboard.stat.upstreamsHint")}
          icon={<Layers className="size-4" />}
        />
        <StatCard
          label={t("dashboard.stat.active")}
          value={active}
          hint={t("dashboard.stat.activeHint")}
          icon={<Activity className="size-4" />}
        />
        <StatCard
          label={t("dashboard.stat.traffic")}
          value={traffic24h === null ? <Skeleton className="mt-1 h-7 w-20" /> : bytes(traffic24h)}
          hint={t("dashboard.stat.trafficHint")}
          icon={<ArrowUpDown className="size-4" />}
        />
        <StatCard
          label={t("dashboard.stat.ip")}
          value={<span className="font-mono text-lg tabular-nums">{data.probe?.server_ip || "–"}</span>}
          hint={
            data.probe?.last_check_at
              ? t("dashboard.stat.ipChecked", {
                  when: timeAgo(new Date(data.probe.last_check_at * 1000).toISOString(), lang),
                })
              : t("dashboard.stat.ipNever")
          }
          icon={<Globe className="size-4" />}
        />
      </div>

      {/* 内置 sing-box 状态卡（v2；旧后端 404 时降级） */}
      <SingboxCard />

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <CardTitle>{t("dashboard.chart.title")}</CardTitle>
              <CardDescription>{t("dashboard.chart.desc")}</CardDescription>
            </div>
            <div className="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <span className="size-2 rounded-sm bg-chart-1" aria-hidden="true" />
                {t("dashboard.chart.conns")}
              </span>
              <span className="flex items-center gap-1.5">
                <span className="h-0.5 w-3 border-t-2 border-dashed border-chart-2" aria-hidden="true" />
                {t("dashboard.chart.traffic")}
              </span>
            </div>
          </CardHeader>
          <CardContent>
            {metrics === null ? (
              <Skeleton className="h-[220px] w-full" />
            ) : metrics.length === 0 ? (
              <div className="flex h-[220px] items-center justify-center text-sm text-muted-foreground">
                {t("dashboard.chart.empty")}
              </div>
            ) : (
              <MetricsChart points={metrics} />
            )}
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("dashboard.quick.title")}</CardTitle>
              <CardDescription>{t("dashboard.quick.desc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {[
                {
                  n: 1,
                  done: data.upstreams.length > 0,
                  label: t("dashboard.quick.step1"),
                  page: "upstreams" as Page,
                },
                {
                  n: 2,
                  done: data.ports.length > 0,
                  label: t("dashboard.quick.step2"),
                  page: "ports" as Page,
                },
                {
                  n: 3,
                  done: data.accounts.some((a) => a.enabled),
                  label: t("dashboard.quick.step3"),
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
                      "flex size-6 shrink-0 items-center justify-center rounded-md border text-xs tabular-nums",
                      step.done ? "border-foreground/60 font-medium text-foreground" : "text-muted-foreground"
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
                <CardTitle>{t("dashboard.logs.title")}</CardTitle>
                <CardDescription>{t("dashboard.logs.desc")}</CardDescription>
              </div>
              <Button variant="ghost" size="sm" onClick={() => onNavigate("logs")}>
                {t("dashboard.logs.viewAll")}
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
                  <p className="text-sm font-medium">{t("dashboard.logs.empty")}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{t("dashboard.logs.emptyDesc")}</p>
                </div>
              ) : (
                <ul className="divide-y">
                  {logs.slice(0, 6).map((l, i) => (
                    <li key={`${l.time}-${i}`} className="flex items-center gap-3 py-2">
                      <span className="w-16 shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
                        {clock(l.time, lang)}
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

/** "unavailable" = 后端还没有 /api/singbox/status（404/405）；"error" = 其他失败。 */
type SingboxState = "loading" | "ready" | "unavailable" | "error";

export function SingboxCard() {
  const { t, lang } = useI18n();
  const [state, setState] = React.useState<SingboxState>("loading");
  const [status, setStatus] = React.useState<SingboxStatus | null>(null);
  const [errorMsg, setErrorMsg] = React.useState("");
  const [expanded, setExpanded] = React.useState(false);
  const [refreshing, setRefreshing] = React.useState(false);

  const load = React.useCallback(async () => {
    setRefreshing(true);
    try {
      const s = await api.singboxStatus();
      setStatus(s);
      setState("ready");
      setErrorMsg("");
    } catch (e: any) {
      const code = typeof e?.status === "number" ? e.status : 0;
      // 404/405 = 后端尚未合并该接口（或本地 dev 代理返回 HTML 404）：降级，不崩溃、不刷报错。
      if (code === 404 || code === 405 || code === 501) {
        setState("unavailable");
      } else {
        setState((prev) => (prev === "ready" && status ? prev : "error"));
        setErrorMsg(e?.message || "");
      }
    } finally {
      setRefreshing(false);
    }
  }, [status]);

  React.useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const logs = status?.logs ?? [];
  const shownLogs = expanded ? logs : logs.slice(-5);
  const upCount = status?.upstreams?.length ?? 0;

  return (
    <Card data-testid="singbox-card" aria-labelledby="singbox-title">
      <CardHeader className="flex-row items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <Box className="size-4" aria-hidden="true" />
          </div>
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle id="singbox-title">{t("dashboard.singbox.title")}</CardTitle>
              {state === "ready" && status ? (
                status.running ? (
                  <DotBadge tone="ok">{t("dashboard.singbox.running")}</DotBadge>
                ) : (
                  <DotBadge tone="fail">{t("dashboard.singbox.stopped")}</DotBadge>
                )
              ) : null}
            </div>
            <CardDescription>{t("dashboard.singbox.desc")}</CardDescription>
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={load}
          disabled={refreshing}
          aria-label={t("dashboard.singbox.refresh")}
          title={t("dashboard.singbox.refresh")}
        >
          <RefreshCw className={cn("size-4", refreshing && "animate-spin")} aria-hidden="true" />
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {state === "loading" ? (
          <div className="space-y-2">
            <Skeleton className="h-4 w-64" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : state === "unavailable" ? (
          <div className="rounded-lg border border-dashed px-3 py-4 text-center">
            <p className="text-sm font-medium">{t("dashboard.singbox.unavailable")}</p>
            <p className="mx-auto mt-1 max-w-md text-xs text-muted-foreground">
              {t("dashboard.singbox.unavailableDesc")}
            </p>
          </div>
        ) : state === "error" ? (
          <div className="rounded-lg border border-dashed px-3 py-4 text-center">
            <p className="text-sm font-medium">{t("dashboard.singbox.error")}</p>
            {errorMsg ? <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{errorMsg}</p> : null}
          </div>
        ) : status ? (
          <>
            <dl className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <div className="space-y-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.upstreams")}</dt>
                <dd className="text-sm font-medium tabular-nums">{upCount}</dd>
              </div>
              <div className="space-y-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.reloads")}</dt>
                <dd className="text-sm font-medium tabular-nums">{status.reload_count ?? 0}</dd>
              </div>
              <div className="space-y-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.lastReload")}</dt>
                <dd className="text-sm font-medium">
                  {status.last_reload_at
                    ? timeAgo(new Date(status.last_reload_at * 1000).toISOString(), lang)
                    : t("dashboard.singbox.never")}
                </dd>
              </div>
              <div className="space-y-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.startedAt")}</dt>
                <dd className="text-sm font-medium" title={dateTime(status.started_at ?? 0, lang)}>
                  {status.started_at
                    ? timeAgo(new Date(status.started_at * 1000).toISOString(), lang)
                    : "–"}
                </dd>
              </div>
            </dl>

            {status.last_error ? (
              <p className="break-all rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 font-mono text-xs text-destructive">
                <span className="font-sans font-medium">{t("dashboard.singbox.lastError")}：</span>
                {status.last_error}
              </p>
            ) : null}

            <div className="space-y-1.5">
              <div className="flex items-center justify-between gap-2">
                <p className="text-xs font-medium text-muted-foreground">{t("dashboard.singbox.logs")}</p>
                {logs.length > 5 ? (
                  <Button variant="ghost" size="xs" onClick={() => setExpanded((v) => !v)} aria-expanded={expanded}>
                    <ChevronDown
                      className={cn("size-3 transition-transform", expanded && "rotate-180")}
                      aria-hidden="true"
                    />
                    {expanded ? t("dashboard.singbox.showLess") : t("dashboard.singbox.showAll", { n: logs.length })}
                  </Button>
                ) : null}
              </div>
              {shownLogs.length === 0 ? (
                <p className="text-xs text-muted-foreground">{t("dashboard.singbox.noLogs")}</p>
              ) : (
                <pre
                  className="max-h-48 overflow-auto rounded-lg border bg-muted/30 p-3 font-mono text-xs whitespace-pre-wrap break-all"
                  aria-live="polite"
                >
                  {shownLogs.join("\n")}
                </pre>
              )}
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
