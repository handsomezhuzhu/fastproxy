import * as React from "react";
import { RefreshCw, ScrollText, Trash2 } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { toast } from "sonner";
import { api, type Data, type LogEntry } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState, FormSelect, PageHeader, PaginationControls, Toolbar } from "@/components/page";
import { useI18n } from "@/i18n";
import { bytes, clock } from "@/lib/utils";

function statusVariant(status?: string) {
  const s = status || "ok";
  if (s === "ok") return { label: s, className: "bg-foreground text-background" };
  if (s === "auth-failed") return { label: s, className: "bg-muted text-foreground" };
  return { label: s, className: "border border-foreground/40 font-medium text-foreground" };
}

export function LogsPage({ data }: { data: Data }) {
  const { t, lang } = useI18n();
  // Server-side pagination state
  const [entries, setEntries] = React.useState<LogEntry[] | null>(null);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(50);

  const [level, setLevel] = React.useState("debug");
  const [port, setPort] = React.useState("");
  const [account, setAccount] = React.useState("");
  const [auto, setAuto] = React.useState(true);
  const [loading, setLoading] = React.useState(false);
  const [clearOpen, setClearOpen] = React.useState(false);
  const [clearing, setClearing] = React.useState(false);

  // 筛选变化时回到第一页
  React.useEffect(() => {
    setPage(1);
  }, [level, port, account]);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.logs({
        limit: pageSize,
        offset: (page - 1) * pageSize,
        level,
        port: port ? Number(port) : undefined,
        account: account || undefined,
      });
      setEntries(r.entries);
      setTotal(r.total);
    } catch {
      /* 保留旧数据 */
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, level, port, account]);

  React.useEffect(() => {
    load();
  }, [load]);

  React.useEffect(() => {
    if (!auto) return;
    const timer = window.setInterval(() => load(), 4000);
    return () => window.clearInterval(timer);
  }, [auto, load]);

  async function clearLogs() {
    setClearing(true);
    try {
      await api.clearLogs();
      toast.success(t("logs.cleared"));
      setClearOpen(false);
      setPage(1);
      await load();
    } catch (err: any) {
      toast.error(err?.message || t("logs.clearFailed"));
    } finally {
      setClearing(false);
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader title={t("logs.title")} description={t("logs.description")} />

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <FormSelect
          id="log-level"
          aria-label={t("logs.levelLabel")}
          value={level}
          onValueChange={setLevel}
          items={[
            { value: "debug", label: t("logs.level.all") },
            { value: "info", label: t("logs.level.info") },
            { value: "warn", label: t("logs.level.warn") },
            { value: "error", label: t("logs.level.error") },
          ]}
        />
        <FormSelect
          id="log-port"
          aria-label={t("logs.filterPort")}
          value={port}
          onValueChange={setPort}
          items={[
            { value: "", label: t("logs.filterPortAll") },
            ...data.ports.map((p) => ({ value: String(p.port), label: t("copy.portOption", { port: p.port }) })),
          ]}
        />
        <FormSelect
          id="log-account"
          aria-label={t("logs.filterAccount")}
          value={account}
          onValueChange={setAccount}
          items={[
            { value: "", label: t("logs.filterAccountAll") },
            ...data.accounts.map((a) => ({ value: a.username, label: a.username })),
          ]}
        />
        <FormSelect
          id="log-limit"
          aria-label={t("logs.pageSize")}
          value={String(pageSize)}
          onValueChange={(v) => {
            setPageSize(Number(v));
            setPage(1);
          }}
          items={[
            { value: "20", label: t("logs.perPage", { n: 20 }) },
            { value: "50", label: t("logs.perPage", { n: 50 }) },
            { value: "100", label: t("logs.perPage", { n: 100 }) },
          ]}
        />
      </div>

      {entries === null ? (
        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between gap-3">
            <Skeleton className="h-5 w-32" />
            <div className="flex gap-2">
              <Skeleton className="h-7 w-20" />
              <Skeleton className="h-7 w-20" />
            </div>
          </div>
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-9 w-full" />
          ))}
        </div>
      ) : (
        <>
          <Toolbar
            count={t("logs.count", { n: total })}
            detail={auto ? t("logs.autoDetail") : undefined}
          >
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Switch checked={auto} onCheckedChange={setAuto} aria-label={t("logs.autoLabel")} size="sm" />
              {t("logs.auto")}
            </label>
            <Button variant="outline" size="sm" onClick={load} disabled={loading}>
              {loading ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <RefreshCw className="size-4" aria-hidden="true" />
              )}
              {t("common.refresh")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setClearOpen(true)}
              disabled={total === 0}
            >
              <Trash2 className="size-4" aria-hidden="true" />
              {t("logs.clear")}
            </Button>
          </Toolbar>

          {entries.length === 0 ? (
            <EmptyState
              icon={<ScrollText className="size-5" aria-hidden="true" />}
              title={t("logs.empty.title")}
              description={t("logs.empty.desc")}
            />
          ) : (
            <>
              <div className="overflow-hidden rounded-lg border">
                <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("logs.col.time")}</TableHead>
                    <TableHead>{t("logs.col.level")}</TableHead>
                    <TableHead>{t("common.port")}</TableHead>
                    <TableHead>{t("common.account")}</TableHead>
                    <TableHead>{t("logs.col.client")}</TableHead>
                    <TableHead>{t("logs.col.proto")}</TableHead>
                    <TableHead>{t("logs.col.target")}</TableHead>
                    <TableHead>{t("logs.col.status")}</TableHead>
                    <TableHead className="text-right">{t("logs.col.traffic")}</TableHead>
                    <TableHead className="text-right">{t("logs.col.duration")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((e, i) => {
                    const st = statusVariant(e.status);
                    return (
                      <TableRow key={`${e.time}-${i}`} className={e.level === "warn" || e.level === "error" ? "bg-muted/40" : ""}>
                        <TableCell className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground">
                          {clock(e.time, lang)}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={e.level === "error" ? "secondary" : "outline"}
                            className={
                              e.level === "error"
                                ? "border border-foreground/40 font-medium text-foreground"
                                : e.level === "warn"
                                  ? "bg-muted text-foreground"
                                  : ""
                            }
                          >
                            {e.level}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs tabular-nums">{e.port || "–"}</TableCell>
                        <TableCell className="text-xs">{e.account || "–"}</TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">{e.client || "–"}</TableCell>
                        <TableCell className="text-xs">{e.proto || "–"}</TableCell>
                        <TableCell className="max-w-[220px] truncate text-xs" title={e.target || e.msg}>
                          {e.target ? (
                            <span className="font-mono">{e.target}</span>
                          ) : (
                            e.msg || "–"
                          )}
                        </TableCell>
                        <TableCell>
                          {e.status ? (
                            <Badge variant="secondary" className={st.className}>
                              {st.label}
                            </Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground">–</span>
                          )}
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                          ↑{bytes(e.bytes_out)} ↓{bytes(e.bytes_in)}
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-right text-xs tabular-nums text-muted-foreground">
                          {e.duration_ms} ms
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
                </Table>
              </div>
              <PaginationControls
                page={page}
                pageSize={pageSize}
                total={total}
                onChange={(p, s) => {
                  setPage(p);
                  setPageSize(s);
                }}
              />
            </>
          )}
        </>
      )}

      <AlertDialog open={clearOpen} onOpenChange={setClearOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("logs.clear.title")}</AlertDialogTitle>
            <AlertDialogDescription>{t("logs.clear.desc", { n: total })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={clearLogs} disabled={clearing}>
              {clearing ? <Spinner data-icon="inline-start" /> : null}
              {t("logs.clear.action")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
