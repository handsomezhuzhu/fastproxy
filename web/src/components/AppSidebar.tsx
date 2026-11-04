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
  Zap,
  type LucideIcon,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { useI18n } from "@/i18n";
import type { Page } from "@/App";

export type NavItem = { id: Page; labelKey: string; icon: LucideIcon };

/** Sidebar groups; labels are i18n keys resolved at render time. */
export const NAV_GROUPS: { labelKey: string; items: NavItem[] }[] = [
  {
    labelKey: "nav.group.overview",
    items: [{ id: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard }],
  },
  {
    labelKey: "nav.group.resources",
    items: [
      { id: "ports", labelKey: "nav.ports", icon: Network },
      { id: "accounts", labelKey: "nav.accounts", icon: Users },
      { id: "upstreams", labelKey: "nav.upstreams", icon: Layers },
      { id: "subscriptions", labelKey: "nav.subscriptions", icon: Rss },
      { id: "singbox", labelKey: "nav.singbox", icon: Box },
    ],
  },
  {
    labelKey: "nav.group.system",
    items: [
      { id: "logs", labelKey: "nav.logs", icon: ScrollText },
      { id: "settings", labelKey: "nav.settings", icon: SettingsIcon },
    ],
  },
];

export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap((g) => g.items);

export function AppSidebar({
  version,
  page,
  onNavigate,
  onLogout,
  ...props
}: {
  version: string;
  page: Page;
  onNavigate: (p: Page) => void;
  onLogout: () => void;
} & Omit<React.ComponentProps<typeof Sidebar>, "children">) {
  const { t } = useI18n();
  const { isMobile, setOpenMobile } = useSidebar();
  // Release builds pass the git tag (already prefixed with "v"); dev builds pass "dev".
  const versionLabel = version.startsWith("v") ? version : `v${version}`;

  function go(id: Page) {
    onNavigate(id);
    if (isMobile) setOpenMobile(false);
  }

  return (
    <Sidebar collapsible="icon" variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" onClick={() => go("dashboard")} tooltip="FastProxy">
              <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <Zap className="size-4" aria-hidden="true" />
              </div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-semibold">FastProxy</span>
                <span className="truncate text-xs text-muted-foreground" translate="no">
                  {versionLabel}
                </span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <nav aria-label={t("shell.mainNav")} className="flex flex-col">
          {NAV_GROUPS.map((group) => (
            <SidebarGroup key={group.labelKey}>
              <SidebarGroupLabel>{t(group.labelKey)}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {group.items.map((item) => {
                    const label = t(item.labelKey);
                    const active = page === item.id;
                    return (
                      <SidebarMenuItem key={item.id}>
                        <SidebarMenuButton
                          isActive={active}
                          tooltip={label}
                          aria-current={active ? "page" : undefined}
                          onClick={() => go(item.id)}
                        >
                          <item.icon />
                          <span>{label}</span>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    );
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </nav>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton onClick={onLogout} tooltip={t("shell.logout")}>
              <LogOut />
              <span>{t("shell.logout")}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
