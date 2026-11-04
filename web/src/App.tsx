import * as React from "react";
import { Activity, Gauge, LogOut, RefreshCw, Zap } from "lucide-react";
import { api } from "@/api";
import type { Data } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { Spinner } from "@/components/ui/spinner";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppSidebar, NAV_ITEMS } from "@/components/AppSidebar";
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
import { SingboxPage } from "@/pages/Singbox";
import { LogsPage } from "@/pages/Logs";
import { SettingsPage } from "@/pages/Settings";

export type Page = "dashboard" | "ports" | "accounts" | "upstreams" | "subscriptions" | "singbox" | "logs" | "settings";

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
    const timer = window.setInterval(() => {
      api
        .stats()
        .then((r) => setData((d) => (d ? { ...d, status: r.status } : d)))
        .catch(() => {});
    }, 5000);
    return () => window.clearInterval(timer);
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

  const toaster = <Toaster theme={resolved} position="bottom-right" richColors closeButton />;

  if (!session) {
    return <BootScreen text={t("shell.booting")} />;
  }

  if (!session.auth) {
    return (
      <>
        <LoginPage onSuccess={() => api.session().then(setSession).then(() => reload())} />
        {toaster}
      </>
    );
  }

  if (!data) {
    if (loadError) {
      return (
        <div className="flex min-h-svh items-center justify-center p-6">
          <Empty className="max-w-md border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <RefreshCw />
              </EmptyMedia>
              <EmptyTitle>{t("shell.loadFailed")}</EmptyTitle>
              <EmptyDescription className="break-all">{loadError}</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <div className="flex gap-2">
                <Button onClick={() => reload()}>{t("common.retry")}</Button>
                <Button variant="outline" onClick={logout}>
                  <LogOut data-icon="inline-start" />
                  {t("shell.relogin")}
                </Button>
              </div>
            </EmptyContent>
          </Empty>
        </div>
      );
    }
    return <BootScreen text={t("shell.loadingConfig")} />;
  }

  const title = t(NAV_ITEMS.find((n) => n.id === page)?.labelKey ?? "nav.dashboard");
  const activeConns = data.status.ports.reduce((sum, p) => sum + p.conns_cur, 0);

  return (
    <TooltipProvider>
      <SidebarProvider>
        <AppSidebar version={data.version} page={page} onNavigate={navigate} onLogout={logout} />
        <SidebarInset>
          <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 rounded-t-xl border-b bg-background/90 px-4 backdrop-blur lg:px-6">
            <SidebarTrigger className="-ml-1" aria-label={t("shell.openNav")} />
            <Separator orientation="vertical" className="mx-2 data-[orientation=vertical]:h-4" />
            <Breadcrumb className="min-w-0">
              <BreadcrumbList className="flex-nowrap">
                <BreadcrumbItem className="hidden sm:inline-flex">{t("shell.console")}</BreadcrumbItem>
                <BreadcrumbSeparator className="hidden sm:list-item" />
                <BreadcrumbItem>
                  <BreadcrumbPage className="truncate">{title}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>

            <div className="ml-auto flex items-center gap-2">
              <Badge variant="outline" className="hidden tabular-nums text-muted-foreground sm:inline-flex">
                <Activity aria-hidden="true" />
                {t("shell.activeConns", { n: activeConns })}
              </Badge>
              <Button variant="outline" size="sm" onClick={() => openCopy(null)} className="hidden sm:inline-flex">
                <Gauge data-icon="inline-start" />
                {t("shell.copyAccess")}
              </Button>
              <Separator orientation="vertical" className="mx-1 hidden data-[orientation=vertical]:h-4 md:block" />
              <ThemeSwitch />
              <AuthLanguageToggle />
            </div>
          </header>

          <div className="mx-auto flex w-full max-w-[1440px] flex-1 flex-col gap-4 p-4 md:gap-6 lg:p-6">
            <ErrorBoundary key={page}>
              {page === "dashboard" && <DashboardPage data={data} onCopy={openCopy} onNavigate={navigate} />}
              {page === "ports" && <PortsPage data={data} reload={reload} onCopy={openCopy} />}
              {page === "accounts" && <AccountsPage data={data} reload={reload} onCopy={openCopy} />}
              {page === "upstreams" && <UpstreamsPage data={data} reload={reload} />}
              {page === "subscriptions" && <SubscriptionsPage reload={reload} />}
              {page === "singbox" && <SingboxPage />}
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
          </div>
        </SidebarInset>

        <CopyWizard open={copyOpen} onOpenChange={setCopyOpen} data={data} preset={copyPreset} />
        {toaster}
      </SidebarProvider>
    </TooltipProvider>
  );
}

function BootScreen({ text }: { text: string }) {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4 bg-background">
      <div className="flex size-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
        <Zap className="size-5" aria-hidden="true" />
      </div>
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner />
        {text}
      </div>
    </div>
  );
}
