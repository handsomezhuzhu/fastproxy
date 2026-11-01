import * as React from "react";
import {
  Activity,
  Gauge,
  Loader2,
  LogOut,
  Moon,
  PanelLeft,
  RefreshCw,
  Sun,
  Zap,
} from "lucide-react";
import { api, type Data } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarContent, NAV_ITEMS } from "@/components/AppSidebar";
import { CopyWizard, type CopyPreset } from "@/components/CopyWizard";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { LoginPage } from "@/pages/Login";
import { DashboardPage } from "@/pages/Dashboard";
import { PortsPage } from "@/pages/Ports";
import { AccountsPage } from "@/pages/Accounts";
import { UpstreamsPage } from "@/pages/Upstreams";
import { LogsPage } from "@/pages/Logs";
import { SettingsPage } from "@/pages/Settings";

export type Page = "dashboard" | "ports" | "accounts" | "upstreams" | "logs" | "settings";

function readHash(): Page {
  const h = window.location.hash.replace(/^#\/?/, "");
  return (NAV_ITEMS.some((n) => n.id === h) ? h : "dashboard") as Page;
}

// useTheme handles dark/light switching with persistence (shadcn 模板明暗主题).
function useTheme() {
  const [theme, setTheme] = React.useState<"dark" | "light">(() => {
    try {
      return localStorage.getItem("fp-theme") === "light" ? "light" : "dark";
    } catch {
      return "dark";
    }
  });
  React.useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle("dark", theme === "dark");
    root.style.colorScheme = theme;
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute("content", theme === "dark" ? "#171717" : "#ffffff");
    try {
      localStorage.setItem("fp-theme", theme);
    } catch {
      /* storage unavailable */
    }
  }, [theme]);
  return {
    theme,
    toggle: () => setTheme((t) => (t === "dark" ? "light" : "dark")),
  };
}

export default function App() {
  const [session, setSession] = React.useState<{ auth: boolean } | null>(null);
  const [data, setData] = React.useState<Data | null>(null);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [page, setPage] = React.useState<Page>(readHash);
  const [copyOpen, setCopyOpen] = React.useState(false);
  const [copyPreset, setCopyPreset] = React.useState<CopyPreset>(null);
  const { theme, toggle } = useTheme();
  const [navOpen, setNavOpen] = React.useState(false);

  const reload = React.useCallback(async () => {
    try {
      const d = await api.data();
      setData(d);
      setLoadError(null);
    } catch (e: any) {
      if (e?.status === 401) {
        setSession({ auth: false });
        setData(null);
      } else {
        setLoadError(e?.message || "加载配置失败");
      }
    }
  }, []);

  React.useEffect(() => {
    let alive = true;
    api
      .session()
      .then((s) => {
        if (!alive) return;
        setSession(s);
        if (s.auth) reload();
      })
      .catch(() => alive && setSession({ auth: false }));
    return () => {
      alive = false;
    };
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
    setNavOpen(false);
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
    return <BootScreen text="正在连接控制台…" />;
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
    if (loadError) {
      return (
        <div className="flex min-h-screen items-center justify-center p-6">
          <Card className="w-full max-w-md">
            <CardContent className="space-y-4 p-6 text-center">
              <div className="mx-auto flex size-11 items-center justify-center rounded-full bg-destructive/15 text-destructive">
                <RefreshCw className="size-5" aria-hidden="true" />
              </div>
              <div className="space-y-1">
                <h1 className="text-base font-semibold">加载配置失败</h1>
                <p className="break-all text-xs text-muted-foreground">{loadError}</p>
              </div>
              <div className="flex justify-center gap-2">
                <Button onClick={() => reload()}>重试</Button>
                <Button variant="outline" onClick={logout}>
                  <LogOut className="size-4" aria-hidden="true" /> 重新登录
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      );
    }
    return <BootScreen text="正在加载配置…" />;
  }

  const title = NAV_ITEMS.find((n) => n.id === page)?.label ?? "";
  const activeConns = data.status.ports.reduce((sum, p) => sum + p.conns_cur, 0);

  return (
    <TooltipProvider>
      <div className="flex min-h-screen bg-background">
        {/* Desktop sidebar */}
        <aside className="sticky top-0 hidden h-screen w-60 shrink-0 border-r bg-sidebar text-sidebar-foreground md:block">
          <SidebarContent
            version={data.version}
            page={page}
            onNavigate={navigate}
            onLogout={logout}
          />
        </aside>

        {/* Mobile drawer */}
        <Sheet open={navOpen} onOpenChange={setNavOpen}>
          <SheetContent side="left" className="w-60 gap-0 p-0">
            <SheetTitle className="sr-only">导航菜单</SheetTitle>
            <SidebarContent
              version={data.version}
              page={page}
              onNavigate={navigate}
              onLogout={logout}
            />
          </SheetContent>
        </Sheet>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background/90 px-4 backdrop-blur md:px-6">
            <Button
              variant="ghost"
              size="icon-sm"
              className="md:hidden"
              aria-label="打开导航菜单"
              onClick={() => setNavOpen(true)}
            >
              <PanelLeft className="size-4" aria-hidden="true" />
            </Button>
            <Separator
              orientation="vertical"
              className="mx-1 hidden h-5! md:block data-vertical:self-auto!"
            />
            <Breadcrumb className="min-w-0">
              <BreadcrumbList className="flex-nowrap">
                <BreadcrumbItem className="hidden sm:inline-flex">
                  <span className="text-muted-foreground">控制台</span>
                </BreadcrumbItem>
                <BreadcrumbSeparator className="hidden sm:list-item" />
                <BreadcrumbItem>
                  <BreadcrumbPage className="truncate font-medium">{title}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>

            <div className="flex-1" />

            <Button
              variant="ghost"
              size="icon-sm"
              onClick={toggle}
              aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"}
            >
              {theme === "dark" ? (
                <Sun className="size-4" aria-hidden="true" />
              ) : (
                <Moon className="size-4" aria-hidden="true" />
              )}
            </Button>
            <div className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:flex">
              <Activity className="size-3.5" aria-hidden="true" />
              <span className="tabular-nums">{activeConns} 个活跃连接</span>
            </div>
            <Button variant="outline" size="sm" onClick={() => openCopy(null)} className="hidden sm:inline-flex">
              <Gauge className="size-4" aria-hidden="true" />
              复制接入
            </Button>
          </header>

          <main className="mx-auto w-full max-w-7xl flex-1 space-y-6 px-4 py-6 md:px-6">
            <ErrorBoundary key={page}>
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
            </ErrorBoundary>
          </main>
        </div>

        <CopyWizard open={copyOpen} onOpenChange={setCopyOpen} data={data} preset={copyPreset} />
        <Toaster theme={theme} position="bottom-right" richColors closeButton />
      </div>
    </TooltipProvider>
  );
}

function BootScreen({ text }: { text: string }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background">
      <div className="flex size-10 items-center justify-center rounded-xl bg-primary/15 text-primary">
        <Zap className="size-5" aria-hidden="true" />
      </div>
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        {text}
      </div>
    </div>
  );
}
