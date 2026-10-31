import * as React from "react";
import { RefreshCw } from "lucide-react";
import { api, type Data, type LogEntry } from "../api";
import {
  Badge,
  Button,
  EmptyRow,
  Label,
  Select,
  TBody,
  TD,
  TH,
  THead,
  TR,
  Table,
} from "../components/ui";
import { bytes } from "../lib/utils";

const LEVELS: Record<string, string> = {
  ok: "success",
  "auth-failed": "warning",
};

export function LogsPage({ data }: { data: Data }) {
  const [entries, setEntries] = React.useState<LogEntry[]>([]);
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
        limit: parseInt(limit, 10),
        level,
        port: port ? parseInt(port, 10) : undefined,
        account: account || undefined,
      });
      setEntries(r.entries);
    } catch {
      /* ignore */
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
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold">连接日志</h1>
          <p className="text-xs text-muted-foreground">
            每次连接一条结构化记录（JSONL 同时写入 CONFIG 目录的 access.log，8MB 自动轮转）
          </p>
        </div>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <input type="checkbox" checked={auto} onChange={(e) => setAuto(e.target.checked)} />
            自动刷新
          </label>
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className="h-3.5 w-3.5" /> 刷新
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <div className="space-y-1.5">
          <Label>最低级别</Label>
          <Select value={level} onChange={(e) => setLevel(e.target.value)}>
            <option value="debug">全部</option>
            <option value="info">info 及以上</option>
            <option value="warn">warn 及以上</option>
            <option value="error">error</option>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label>端口</Label>
          <Select value={port} onChange={(e) => setPort(e.target.value)}>
            <option value="">全部端口</option>
            {data.ports.map((p) => (
              <option key={p.id} value={String(p.port)}>
                端口 {p.port}
              </option>
            ))}
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label>账户</Label>
          <Select value={account} onChange={(e) => setAccount(e.target.value)}>
            <option value="">全部账户</option>
            {data.accounts.map((a) => (
              <option key={a.id} value={a.username}>
                {a.username}
              </option>
            ))}
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label>条数</Label>
          <Select value={limit} onChange={(e) => setLimit(e.target.value)}>
            <option value="100">100</option>
            <option value="200">200</option>
            <option value="500">500</option>
          </Select>
        </div>
      </div>

      <Table>
        <THead>
          <TR>
            <TH className="whitespace-nowrap">时间</TH>
            <TH>级别</TH>
            <TH>端口</TH>
            <TH>账户</TH>
            <TH>客户端</TH>
            <TH>协议</TH>
            <TH>目标</TH>
            <TH>状态</TH>
            <TH>流量</TH>
            <TH>耗时</TH>
          </TR>
        </THead>
        <TBody>
          {entries.length === 0 ? (
            <EmptyRow colSpan={10} text="暂无日志（产生连接后自动出现）" />
          ) : (
            entries.map((e, i) => (
              <TR key={i} className={e.level === "warn" || e.level === "error" ? "bg-destructive/5" : ""}>
                <TD className="whitespace-nowrap text-[11px] text-muted-foreground">
                  {new Date(e.time).toLocaleTimeString("zh-CN", { hour12: false })}
                </TD>
                <TD>
                  <Badge variant={e.level === "error" ? "destructive" : e.level === "warn" ? "warning" : "outline"}>
                    {e.level}
                  </Badge>
                </TD>
                <TD>{e.port || "-"}</TD>
                <TD>{e.account || "-"}</TD>
                <TD className="font-mono text-[11px] text-muted-foreground">{e.client || "-"}</TD>
                <TD className="text-xs">{e.proto || "-"}</TD>
                <TD className="max-w-[220px] truncate font-mono text-[11px]" title={e.target || e.msg}>
                  {e.target || e.msg || "-"}
                </TD>
                <TD>
                  {e.status ? (
                    <Badge variant={(LEVELS[e.status] || "destructive") as any}>{e.status}</Badge>
                  ) : (
                    <span className="text-xs text-muted-foreground">-</span>
                  )}
                </TD>
                <TD className="whitespace-nowrap text-[11px]">↑{bytes(e.bytes_out)} ↓{bytes(e.bytes_in)}</TD>
                <TD className="whitespace-nowrap text-[11px] text-muted-foreground">{e.duration_ms}ms</TD>
              </TR>
            ))
          )}
        </TBody>
      </Table>
    </div>
  );
}
