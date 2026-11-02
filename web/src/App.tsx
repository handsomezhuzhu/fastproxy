import * as React from "react";
import {
  Activity,
  Gauge,
  Loader2,
  LogOut,
  PanelLeft,
  RefreshCw,
  Zap,
} from "lucide-react";
import { api } from "@/api";
import type { Data } from "@/api";
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
import { I18nProvider, useI18n } from "@/i18n";
import { ThemeSwitch } from "@/components/ThemeSwitch";
import { AuthLanguageToggle } from "@/components/LanguageSwitch";
import { ThemeProvider, useTheme } from "@/lib/theme-context";
import { LoginPage } from "@/pages/Login";
import { DashboardPage } from "@/pages/Dashboard";
import { PortsPage } from "@/pages/Ports";
import { AccountsPage } from "@/pages/Accounts";
import { UpstreamsPage } from "@/pages/Upstreams";
import { SubscriptionsPage } from "@/pages/Subscriptions";
import { LogsPage } from "@/pages/Logs";
import { SettingsPage } from "@/pages/Settings";

export type Page = "dashboard" | "ports" | "accounts" | "upstreams" | "subscriptions" | "logs" | "settings";

function readHash(): Page {
  const h = window.location.hash.replace(/^#\/?/, "");
  return (NAV_ITEMS.some((n) => n.id === h) ? h : "dashboard") as Page;
}

export default function App() {
  return (
    <I18nProvider>
      <ThemeProvider>
        <AppShell />
      </ThemeProvider>
    </I18nProvider>
  );
}

function AppShell() {
  const { t } = useI18n();
  const [session, setSession] = React.useState<{ auth: boolean } | null>(null);
  const [data, setData] = React.useState<Data | null>(null);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [page, setPage] = React.useState<Page>(readHash);
  const [copyOpen, setCopyOpen] = React.useState(false);
  const [copyPreset, setCopyPreset] = React.useState<CopyPreset>(null);
  const { resolved } = useTheme();
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
        setLoadError(e?.message || t("shell.loadFailed"));
      }
    }
  }, [t]);

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
    return <BootScreen text={t("shell.booting")} />;
  }

  if (!session.auth) {
    return (
      <>
        <LoginPage onSuccess={() => api.session().then(setSession).then(() => reload())} />
        <Toaster theme={resolved} position="bottom-right" richColors closeButton />
      </>
    );
  }

  if (!data) {
    if (loadError) {
      return (
        <div className="flex min-h-screen items-center justify-center p-6">
          <Card className="w-full max-w-md">
            <CardContent className="space-y-4 p-6 text-center">
              <div className="mx-auto flex size-11 items-center justify-center rounded-md bg-destructive/15 text-destructive">
                <RefreshCw className="size-5" aria-hidden="true" />
              </div>
              <div className="space-y-1">
                <h1 className="text-base font-semibold">{t("shell.loadFailed")}</h1>
                <p className="break-all text-xs text-muted-foreground">{loadError}</p>
              </div>
              <div className="flex justify-center gap-2">
                <Button onClick={() => reload()}>{t("common.retry")}</Button>
                <Button variant="outline" onClick={logout}>
                  <LogOut className="size-4" aria-hidden="true" /> {t("shell.relogin")}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      );
    }
    return <BootScreen text={t("shell.loadingConfig")} />;
  }

  const title = t(NAV_ITEMS.find((n) => n.id === page)?.labelKey ?? "nav.dashboard");
  const activeConns = data.status.ports.reduce((sum, p) => sum + p.conns_cur, 0);

  return (
    <TooltipProvider>
      <div className="flex min-h-screen bg-background">
        {/* Desktop sidebar（恒定深色，品牌一致性） */}
        <aside className="sticky top-0 hidden h-screen w-64 shrink-0 border-r border-neutral-800 bg-neutral-950 text-neutral-100 md:block">
          <SidebarContent
            version={data.version}
            page={page}
            onNavigate={navigate}
            onLogout={logout}
          />
        </aside>

        {/* Mobile drawer */}
        <Sheet open={navOpen} onOpenChange={setNavOpen}>
          <SheetContent side="left" className="w-64 gap-0 border-neutral-800 bg-neutral-950 p-0 text-neutral-100">
            <SheetTitle className="sr-only">{t("shell.navMenu")}</SheetTitle>
            <SidebarContent
              version={data.version}
              page={page}
              onNavigate={navigate}
              onLogout={logout}
            />
          </SheetContent>
        </Sheet>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background/85 px-4 backdrop-blur md:px-6">
            <Button
              variant="ghost"
              size="icon-sm"
              className="md:hidden"
              aria-label={t("shell.openNav")}
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
                  <span className="text-muted-foreground">{t("shell.console")}</span>
                </BreadcrumbItem>
                <BreadcrumbSeparator className="hidden sm:list-item" />
                <BreadcrumbItem>
                  <BreadcrumbPage className="truncate font-medium">{title}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>

            <div className="flex-1" />

            <div className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:flex">
              <Activity className="size-3.5" aria-hidden="true" />
              <span className="tabular-nums">{t("shell.activeConns", { n: activeConns })}</span>
            </div>
            <Button variant="outline" size="sm" onClick={() => openCopy(null)} className="hidden sm:inline-flex">
              <Gauge className="size-4" aria-hidden="true" />
              {t("shell.copyAccess")}
            </Button>

            {/* 右上角：三态主题 + 语言 */}
            <div className="flex items-center gap-2">
              <ThemeSwitch />
              <AuthLanguageToggle />
            </div>
          </header>

          <main className="mx-auto w-full max-w-[1440px] flex-1 space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
            <ErrorBoundary key={page}>
              {page === "dashboard" && (
                <DashboardPage data={data} onCopy={openCopy} onNavigate={navigate} />
              )}
              {page === "ports" && <PortsPage data={data} reload={reload} onCopy={openCopy} />}
              {page === "accounts" && <AccountsPage data={data} reload={reload} onCopy={openCopy} />}
              {page === "upstreams" && <UpstreamsPage data={data} reload={reload} />}
        {page === "subscriptions" && <SubscriptionsPage reload={reload} />}
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
        <Toaster theme={resolved} position="bottom-right" richColors closeButton />
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
