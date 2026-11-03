import * as React from "react";
import {
  Box,
  LayoutDashboard,
  Layers,
  LogOut,
  Network,
  Rss,
  ScrollText,
  Settings as SettingsIcon,
  Users,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/i18n";
import type { Page } from "@/App";

export type NavItem = { id: Page; labelKey: string; icon: React.ReactNode };

/** Sidebar groups; labels are i18n keys resolved at render time. */
export const NAV_GROUPS: { labelKey: string; items: NavItem[] }[] = [
  {
    labelKey: "nav.group.overview",
    items: [
      { id: "dashboard", labelKey: "nav.dashboard", icon: <LayoutDashboard className="size-4" aria-hidden="true" /> },
    ],
  },
  {
    labelKey: "nav.group.resources",
    items: [
      { id: "ports", labelKey: "nav.ports", icon: <Network className="size-4" aria-hidden="true" /> },
      { id: "accounts", labelKey: "nav.accounts", icon: <Users className="size-4" aria-hidden="true" /> },
      { id: "upstreams", labelKey: "nav.upstreams", icon: <Layers className="size-4" aria-hidden="true" /> },
      { id: "subscriptions", labelKey: "nav.subscriptions", icon: <Rss className="size-4" aria-hidden="true" /> },
      { id: "singbox", labelKey: "nav.singbox", icon: <Box className="size-4" aria-hidden="true" /> },
    ],
  },
  {
    labelKey: "nav.group.system",
    items: [
      { id: "logs", labelKey: "nav.logs", icon: <ScrollText className="size-4" aria-hidden="true" /> },
      { id: "settings", labelKey: "nav.settings", icon: <SettingsIcon className="size-4" aria-hidden="true" /> },
    ],
  },
];

export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap((g) => g.items);


/**
 * 侧边栏主体（恒定深色，明暗主题下一致）。
 * 容器需自带 bg-neutral-950 text-neutral-100（桌面 aside 与移动端 Sheet 通用）。
 */
export function SidebarContent({
  version,
  page,
  onNavigate,
  onLogout,
}: {
  version: string;
  page: Page;
  onNavigate: (p: Page) => void;
  onLogout: () => void;
}) {
  const t = useI18n().t;
  // Release builds pass the git tag (already prefixed with "v"); dev builds
  // pass "dev". Avoid rendering "vv1.0.0-rc.4".
  const versionLabel = version.startsWith("v") ? version : `v${version}`;
  return (
    <div className="flex h-full flex-col text-sidebar-foreground">
      <div className="px-4 py-5">
        <div className="truncate text-sm font-semibold leading-tight text-sidebar-foreground">FastProxy</div>
        <div className="mt-0.5 text-[13px] text-sidebar-foreground/60" translate="no">
          {versionLabel}
        </div>
      </div>

      <nav aria-label={t("shell.mainNav")} className="flex-1 space-y-5 overflow-y-auto px-3">
        {NAV_GROUPS.map((group) => (
          <div key={group.labelKey}>
            <p className="px-2.5 pb-1.5 text-[13px] font-medium text-sidebar-foreground/50">{t(group.labelKey)}</p>
            <ul className="space-y-0.5">
              {group.items.map((item) => {
                const active = page === item.id;
                return (
                  <li key={item.id}>
                    <button
                      type="button"
                      onClick={() => onNavigate(item.id)}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[15px] transition-colors duration-200",
                        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring/50",
                        active
                          ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                          : "text-sidebar-foreground/65 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                      )}
                    >
                      {item.icon}
                      <span className="truncate">{t(item.labelKey)}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      <div className="p-3">
        <button
          type="button"
          onClick={onLogout}
          className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[15px] text-sidebar-foreground/65 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring/50"
        >
          <LogOut className="size-4" aria-hidden="true" />
          <span className="truncate">{t("shell.logout")}</span>
        </button>
      </div>
    </div>
  );
}
