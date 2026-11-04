import * as React from "react";
import { Box, ChevronDown, RefreshCw } from "lucide-react";
import { api, type SingboxStatus } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { DotBadge } from "@/components/page";
import { useI18n } from "@/i18n";
import { cn, timeAgo, dateTime } from "@/lib/utils";

/** "unavailable" = backend has no /api/singbox/status (404/405); "error" = fetch failed. */
type SingboxState = "loading" | "ready" | "unavailable" | "error";

function useSingboxStatus() {
  const [state, setState] = React.useState<SingboxState>("loading");
  const [status, setStatus] = React.useState<SingboxStatus | null>(null);
  const [errorMsg, setErrorMsg] = React.useState("");
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

  return { state, status, errorMsg, refreshing, load };
}

/** Full status card, used by the dedicated sing-box page. */
export function SingboxCard({ titleKey = "dashboard.singbox.title" }: { titleKey?: string }) {
  const { t, lang } = useI18n();
  const { state, status, errorMsg, refreshing, load } = useSingboxStatus();
  const [expanded, setExpanded] = React.useState(false);

  const logs = status?.logs ?? [];
  const shownLogs = expanded ? logs : logs.slice(-5);
  const upCount = status?.upstreams?.length ?? 0;

  return (
    <Card data-testid="singbox-panel" aria-labelledby="singbox-title">
      <CardHeader className="items-start gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <Box className="size-4" aria-hidden="true" />
          </div>
          <div className="flex flex-col min-w-0 gap-1">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle id="singbox-title">{t(titleKey)}</CardTitle>
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
        <CardAction>
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
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {state === "loading" ? (
          <div className="flex flex-col gap-2">
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
              <div className="flex flex-col gap-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.upstreams")}</dt>
                <dd className="text-sm font-medium tabular-nums">{upCount}</dd>
              </div>
              <div className="flex flex-col gap-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.reloads")}</dt>
                <dd className="text-sm font-medium tabular-nums">{status.reload_count ?? 0}</dd>
              </div>
              <div className="flex flex-col gap-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.lastReload")}</dt>
                <dd className="text-sm font-medium">
                  {status.last_reload_at
                    ? timeAgo(new Date(status.last_reload_at * 1000).toISOString(), lang)
                    : t("dashboard.singbox.never")}
                </dd>
              </div>
              <div className="flex flex-col gap-0.5">
                <dt className="text-xs text-muted-foreground">{t("dashboard.singbox.startedAt")}</dt>
                <dd className="text-sm font-medium" title={dateTime(status.started_at ?? 0, lang)}>
                  {status.started_at
                    ? timeAgo(new Date(status.started_at * 1000).toISOString(), lang)
                    : "—"}
                </dd>
              </div>
            </dl>

            {status.last_error ? (
              <p className="break-all rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 font-mono text-xs text-destructive">
                <span className="font-sans font-medium">{t("dashboard.singbox.lastError")}：</span>
                {status.last_error}
              </p>
            ) : null}

            <div className="flex flex-col gap-1.5">
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
