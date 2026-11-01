import * as React from "react";
import { RefreshCw, ScrollText } from "lucide-react";
import { api, type Data, type LogEntry } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState, FormSelect, PageHeader, Toolbar } from "@/components/page";
import { bytes, clock } from "@/lib/utils";

function statusVariant(status?: string) {
  const s = status || "ok";
  if (s === "ok") return { label: s, className: "bg-emerald-500/15 text-emerald-400" };
  if (s === "auth-failed") return { label: s, className: "bg-amber-500/15 text-amber-400" };
  return { label: s, className: "" };
}

export function LogsPage({ data }: { data: Data }) {
  const [entries, setEntries] = React.useState<LogEntry[] | null>(null);
  const [level, setLevel] = React.useState("debug");
  const [port, setPort] = React.useState("");
  const [account, setAccount] = React.useState("");
  const [limit, setLimit] = React.useState("200");
  const [auto, setAuto] = React.useState(true);
  const [loading, setLoading] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.logs({
        limit: Number(limit),
        level,
        port: port ? Number(port) : undefined,
        account: account || undefined,
      });
      setEntries(r.entries);
    } catch {
      /* 保留旧数据 */
    } finally {
      setLoading(false);
    }
  }, [level, port, account, limit]);

  React.useEffect(() => {
    load();
  }, [load]);

  React.useEffect(() => {
    if (!auto) return;
    const t = window.setInterval(load, 4000);
    return () => window.clearInterval(t);
  }, [auto, load]);

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
          aria-label="日志条数"
          value={limit}
          onValueChange={setLimit}
          items={[
            { value: "100", label: "最近 100 条" },
            { value: "200", label: "最近 200 条" },
            { value: "500", label: "最近 500 条" },
          ]}
        />
      </div>

      <Toolbar
        count={entries === null ? "加载中…" : `${entries.length} 条记录`}
        detail={auto ? "每 4 秒自动刷新" : undefined}
      >
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <Switch checked={auto} onCheckedChange={setAuto} aria-label="自动刷新日志" size="sm" />
          自动刷新
        </label>
        <Button variant="outline" size="sm" onClick={load} disabled={loading}>
          <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} aria-hidden="true" />
          刷新
        </Button>
      </Toolbar>

      {entries === null ? (
        <Card>
          <CardContent className="space-y-2 p-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : entries.length === 0 ? (
        <EmptyState
          icon={<ScrollText className="size-5" aria-hidden="true" />}
          title="暂无日志"
          description="客户端产生连接后，记录会实时出现在这里。"
        />
      ) : (
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
                <TableRow key={`${e.time}-${i}`} className={e.level === "warn" || e.level === "error" ? "bg-destructive/5" : ""}>
                  <TableCell className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground">
                    {clock(e.time)}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={e.level === "error" ? "destructive" : "outline"}
                      className={e.level === "warn" ? "bg-amber-500/15 text-amber-400" : ""}
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
      )}
    </div>
  );
}
