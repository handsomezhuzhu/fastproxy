import * as React from "react";
import { Loader2, RefreshCw, ScrollText, Trash2 } from "lucide-react";
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
import { bytes, clock } from "@/lib/utils";

function statusVariant(status?: string) {
  const s = status || "ok";
  if (s === "ok") return { label: s, className: "bg-foreground text-background" };
  if (s === "auth-failed") return { label: s, className: "bg-muted text-foreground" };
  return { label: s, className: "border border-foreground/40 font-medium text-foreground" };
}

export function LogsPage({ data }: { data: Data }) {
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
    const t = window.setInterval(() => load(), 4000);
    return () => window.clearInterval(t);
  }, [auto, load]);

  async function clearLogs() {
    setClearing(true);
    try {
      await api.clearLogs();
      toast.success("连接日志已清空");
      setClearOpen(false);
      setPage(1);
      await load();
    } catch (err: any) {
      toast.error(err?.message || "清空失败");
    } finally {
      setClearing(false);
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="连接日志" description="每次连接一条结构化记录，默认保留 7 天。" />

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <FormSelect
          id="log-level"
          aria-label="最低日志级别"
          value={level}
          onValueChange={setLevel}
          items={[
            { value: "debug", label: "全部级别" },
            { value: "info", label: "info 及以上" },
            { value: "warn", label: "warn 及以上" },
            { value: "error", label: "仅 error" },
          ]}
        />
        <FormSelect
          id="log-port"
          aria-label="按端口筛选"
          value={port}
          onValueChange={setPort}
          items={[
            { value: "", label: "全部端口" },
            ...data.ports.map((p) => ({ value: String(p.port), label: `端口 ${p.port}` })),
          ]}
        />
        <FormSelect
          id="log-account"
          aria-label="按账户筛选"
          value={account}
          onValueChange={setAccount}
          items={[
            { value: "", label: "全部账户" },
            ...data.accounts.map((a) => ({ value: a.username, label: a.username })),
          ]}
        />
        <FormSelect
          id="log-limit"
          aria-label="每页条数"
          value={String(pageSize)}
          onValueChange={(v) => {
            setPageSize(Number(v));
            setPage(1);
          }}
          items={[
            { value: "20", label: "每页 20 条" },
            { value: "50", label: "每页 50 条" },
            { value: "100", label: "每页 100 条" },
          ]}
        />
      </div>

      {entries === null ? (
        <div className="space-y-3">
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
          <Toolbar count={`${total} 条记录`} detail={auto ? "每 4 秒自动刷新" : undefined}>
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Switch checked={auto} onCheckedChange={setAuto} aria-label="自动刷新日志" size="sm" />
              自动刷新
            </label>
            <Button variant="outline" size="sm" onClick={load} disabled={loading}>
              {loading ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <RefreshCw className="size-4" aria-hidden="true" />
              )}
              刷新
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setClearOpen(true)}
              disabled={total === 0}
            >
              <Trash2 className="size-4" aria-hidden="true" />
              清空日志
            </Button>
          </Toolbar>

          {entries.length === 0 ? (
            <EmptyState
              icon={<ScrollText className="size-5" aria-hidden="true" />}
              title="暂无日志"
              description="客户端产生连接后，记录会实时出现在这里。"
            />
          ) : (
            <>
              <div className="overflow-hidden rounded-xl border">
                <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>时间</TableHead>
                    <TableHead>级别</TableHead>
                    <TableHead>端口</TableHead>
                    <TableHead>账户</TableHead>
                    <TableHead>客户端</TableHead>
                    <TableHead>协议</TableHead>
                    <TableHead>目标</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">流量</TableHead>
                    <TableHead className="text-right">耗时</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((e, i) => {
                    const st = statusVariant(e.status);
                    return (
                      <TableRow key={`${e.time}-${i}`} className={e.level === "warn" || e.level === "error" ? "bg-muted/40" : ""}>
                        <TableCell className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground">
                          {clock(e.time)}
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
                        <TableCell className="max-w-[220px] truncate font-mono text-xs" title={e.target || e.msg}>
                          {e.target || e.msg || "–"}
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
            <AlertDialogTitle>清空全部连接日志？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除全部 {total} 条结构化记录，无法恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={clearLogs} disabled={clearing}>
              {clearing ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              清空
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
