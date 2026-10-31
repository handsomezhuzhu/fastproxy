import * as React from "react";
import {
  LayoutDashboard,
  Network,
  Users,
  Layers,
  ScrollText,
  Settings as SettingsIcon,
  LogOut,
  Zap,
} from "lucide-react";
import { api, type Data } from "./api";
import { Button } from "./components/ui";
import { ToastHost, toast } from "./components/Toast";
import { CopyWizard } from "./components/CopyWizard";
import { LoginPage } from "./pages/Login";
import { DashboardPage } from "./pages/Dashboard";
import { PortsPage } from "./pages/Ports";
import { AccountsPage } from "./pages/Accounts";
import { UpstreamsPage } from "./pages/Upstreams";
import { LogsPage } from "./pages/Logs";
import { SettingsPage } from "./pages/Settings";
import { cn } from "./lib/utils";

export type Page = "dashboard" | "ports" | "accounts" | "upstreams" | "logs" | "settings";

export type CopyPreset = { dim: "port" | "account"; id: string } | null;

export default function App() {
  const [session, setSession] = React.useState<{ auth: boolean; version?: string } | null>(null);
  const [data, setData] = React.useState<Data | null>(null);
  const [page, setPage] = React.useState<Page>("dashboard");
  const [copyOpen, setCopyOpen] = React.useState(false);
  const [copyPreset, setCopyPreset] = React.useState<CopyPreset>(null);

  const reload = React.useCallback(async () => {
    try {
      const d = await api.data();
      setData(d);
      setSession((s) => ({ auth: true, version: d.version || s?.version }));
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
      <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">
        正在加载…
      </div>
    );
  }

  if (!session.auth) {
    return (
      <>
        <LoginPage
          onSuccess={() => {
            api.session().then((s) => {
              setSession(s);
              reload();
            });
          }}
        />
        <ToastHost />
      </>
    );
  }

  if (!data) {
    return (
      <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">
        正在加载配置…
      </div>
    );
  }

  const nav: { id: Page; label: string; icon: React.ReactNode }[] = [
    { id: "dashboard", label: "仪表盘", icon: <LayoutDashboard className="h-4 w-4" /> },
    { id: "ports", label: "端口线路", icon: <Network className="h-4 w-4" /> },
    { id: "accounts", label: "账户", icon: <Users className="h-4 w-4" /> },
    { id: "upstreams", label: "上游代理", icon: <Layers className="h-4 w-4" /> },
    { id: "logs", label: "连接日志", icon: <ScrollText className="h-4 w-4" /> },
    { id: "settings", label: "系统设置", icon: <SettingsIcon className="h-4 w-4" /> },
  ];

  return (
    <div className="flex min-h-screen flex-col md:flex-row">
      <aside className="flex shrink-0 flex-col border-b md:w-56 md:border-b-0 md:border-r">
        <div className="flex items-center gap-2 px-5 py-4">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/15">
            <Zap className="h-4 w-4 text-primary" />
          </div>
          <div>
            <div className="text-sm font-semibold leading-tight">FastProxy</div>
            <div className="text-[11px] text-muted-foreground">v{data.version}</div>
          </div>
        </div>
        <nav className="flex gap-1 overflow-x-auto px-2 pb-2 md:flex-1 md:flex-col md:overflow-visible">
          {nav.map((item) => (
            <button
              key={item.id}
              onClick={() => setPage(item.id)}
              className={cn(
                "flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-left text-[13px] transition-colors",
                page === item.id
                  ? "bg-primary/15 font-medium text-primary"
                  : "text-muted-foreground hover:bg-accent hover:text-foreground"
              )}
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </nav>
        <div className="hidden border-t p-3 md:block">
          <Button variant="ghost" size="sm" className="w-full justify-start text-muted-foreground" onClick={logout}>
            <LogOut className="h-4 w-4" /> 退出登录
          </Button>
        </div>
      </aside>

      <main className="min-w-0 flex-1">
        <div className="mx-auto max-w-6xl space-y-5 p-5 md:p-7">
          {page === "dashboard" && (
            <DashboardPage
              data={data}
              onCopy={openCopy}
              onNavigate={setPage}
            />
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
                toast("密码已修改，请重新登录");
              }}
            />
          )}
        </div>
      </main>

      <CopyWizard open={copyOpen} onOpenChange={setCopyOpen} data={data} preset={copyPreset} />
      <ToastHost />
    </div>
  );
}
