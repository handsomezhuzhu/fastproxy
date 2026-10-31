import * as React from "react";
import {
  Activity,
  Gauge,
  LayoutDashboard,
  Layers,
  LogOut,
  Network,
  ScrollText,
  Settings as SettingsIcon,
  Users,
  Zap,
} from "lucide-react";
import { api, type Data } from "@/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { CopyWizard, type CopyPreset } from "@/components/CopyWizard";
import { LoginPage } from "@/pages/Login";
import { DashboardPage } from "@/pages/Dashboard";
import { PortsPage } from "@/pages/Ports";
import { AccountsPage } from "@/pages/Accounts";
import { UpstreamsPage } from "@/pages/Upstreams";
import { LogsPage } from "@/pages/Logs";
import { SettingsPage } from "@/pages/Settings";
import { cn } from "@/lib/utils";

export type Page = "dashboard" | "ports" | "accounts" | "upstreams" | "logs" | "settings";

const NAV: { id: Page; label: string; icon: React.ReactNode }[] = [
  { id: "dashboard", label: "仪表盘", icon: <LayoutDashboard className="size-4" aria-hidden="true" /> },
  { id: "ports", label: "端口线路", icon: <Network className="size-4" aria-hidden="true" /> },
  { id: "accounts", label: "账户", icon: <Users className="size-4" aria-hidden="true" /> },
  { id: "upstreams", label: "上游代理", icon: <Layers className="size-4" aria-hidden="true" /> },
  { id: "logs", label: "连接日志", icon: <ScrollText className="size-4" aria-hidden="true" /> },
  { id: "settings", label: "系统设置", icon: <SettingsIcon className="size-4" aria-hidden="true" /> },
];

function readHash(): Page {
  const h = window.location.hash.replace(/^#\/?/, "");
  return (NAV.some((n) => n.id === h) ? h : "dashboard") as Page;
}

export default function App() {
  const [session, setSession] = React.useState<{ auth: boolean } | null>(null);
  const [data, setData] = React.useState<Data | null>(null);
  const [page, setPage] = React.useState<Page>(readHash);
  const [copyOpen, setCopyOpen] = React.useState(false);
  const [copyPreset, setCopyPreset] = React.useState<CopyPreset>(null);

  const reload = React.useCallback(async () => {
    try {
      const d = await api.data();
      setData(d);
    } catch (e: any) {
      if (e?.status === 401) {
        setSession({ auth: false });
        setData(null);
      }
    }
  }, []);

  React.useEffect(() => {
    api
      .session()
      .then((s) => {
        setSession(s);
        if (s.auth) reload();
      })
      .catch(() => setSession({ auth: false }));
  }, [reload]);

  React.useEffect(() => {
    const onHash = () => setPage(readHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // Live counters every 5s.
  React.useEffect(() => {
    if (!session?.auth) return;
    const t = window.setInterval(() => {
      api
        .stats()
        .then((r) => setData((d) => (d ? { ...d, status: r.status } : d)))
        .catch(() => {});
    }, 5000);
    return () => window.clearInterval(t);
  }, [session?.auth]);

  function navigate(next: Page) {
    window.location.hash = `#/${next}`;
    setPage(next);
  }

  function openCopy(preset: CopyPreset) {
    setCopyPreset(preset);
    setCopyOpen(true);
  }

  async function logout() {
    await api.logout().catch(() => {});
    setSession({ auth: false });
    setData(null);
  }

  if (!session) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="w-full max-w-sm space-y-3 p-6" aria-busy="true" aria-label="正在加载">
          <Skeleton className="mx-auto size-12 rounded-xl" />
          <Skeleton className="mx-auto h-4 w-32" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </div>
      </div>
    );
  }

  if (!session.auth) {
    return (
      <>
        <LoginPage onSuccess={() => api.session().then(setSession).then(() => reload())} />
        <Toaster theme="dark" position="bottom-right" richColors closeButton />
      </>
    );
  }

  if (!data) {
    return (
      <div className="flex min-h-screen items-center justify-center px-6" aria-busy="true">
        <div className="w-full max-w-md space-y-3">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </div>
      </div>
    );
  }

  const title = NAV.find((n) => n.id === page)?.label ?? "";

  return (
    <TooltipProvider>
      <div className="flex min-h-screen flex-col bg-background md:flex-row">
        <aside className="flex shrink-0 flex-col border-b md:w-60 md:border-b-0 md:border-r">
          <div className="flex items-center gap-2.5 px-4 py-4">
            <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Zap className="size-4" aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold leading-tight">FastProxy</div>
              <div className="text-xs text-muted-foreground" translate="no">v{data.version}</div>
            </div>
          </div>

          <nav aria-label="主导航" className="flex gap-1 overflow-x-auto px-2 pb-2 md:flex-1 md:flex-col md:overflow-visible">
            {NAV.map((item) => {
              const active = page === item.id;
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => navigate(item.id)}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "flex shrink-0 items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                    active
                      ? "bg-secondary font-medium text-foreground"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  )}
                >
                  {item.icon}
                  <span className="whitespace-nowrap">{item.label}</span>
                </button>
              );
            })}
          </nav>

          <div className="hidden border-t p-3 md:block">
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-start text-muted-foreground"
              onClick={logout}
            >
              <LogOut className="size-4" aria-hidden="true" />
              退出登录
            </Button>
          </div>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b bg-background/85 px-4 backdrop-blur md:px-6">
            <div className="flex min-w-0 items-center gap-2 text-sm">
              <span className="text-muted-foreground">控制台</span>
              <span className="text-muted-foreground/60" aria-hidden="true">/</span>
              <span className="truncate font-medium">{title}</span>
            </div>
            <div className="flex-1" />
            <div className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:flex">
              <Activity className="size-3.5" aria-hidden="true" />
              <span className="tabular-nums">
                {data.status.ports.reduce((sum, p) => sum + p.conns_cur, 0)} 个活跃连接
              </span>
            </div>
            <Button variant="outline" size="sm" onClick={() => openCopy(null)} className="hidden sm:inline-flex">
              <Gauge className="size-4" aria-hidden="true" />
              复制接入
            </Button>
          </header>

          <main className="mx-auto w-full max-w-7xl flex-1 space-y-6 px-4 py-6 md:px-6">
            {page === "dashboard" && (
              <DashboardPage data={data} onCopy={openCopy} onNavigate={navigate} />
            )}
            {page === "ports" && <PortsPage data={data} reload={reload} onCopy={openCopy} />}
            {page === "accounts" && <AccountsPage data={data} reload={reload} onCopy={openCopy} />}
            {page === "upstreams" && <UpstreamsPage data={data} reload={reload} />}
            {page === "logs" && <LogsPage data={data} />}
            {page === "settings" && (
              <SettingsPage
                data={data}
                reload={reload}
                onLoggedOut={() => {
                  setSession({ auth: false });
                  setData(null);
                }}
              />
            )}
          </main>
        </div>

        <CopyWizard open={copyOpen} onOpenChange={setCopyOpen} data={data} preset={copyPreset} />
        <Toaster theme="dark" position="bottom-right" richColors closeButton />
      </div>
    </TooltipProvider>
  );
}
