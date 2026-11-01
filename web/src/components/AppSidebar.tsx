import * as React from "react";
import {
  LayoutDashboard,
  Layers,
  LogOut,
  Network,
  ScrollText,
  Settings as SettingsIcon,
  ShieldCheck,
  Users,
  Zap,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { Page } from "@/App";

export type NavItem = { id: Page; label: string; icon: React.ReactNode };

export const NAV_GROUPS: { label: string; items: NavItem[] }[] = [
  {
    label: "概览",
    items: [
      { id: "dashboard", label: "仪表盘", icon: <LayoutDashboard className="size-4" aria-hidden="true" /> },
    ],
  },
  {
    label: "资源",
    items: [
      { id: "ports", label: "端口线路", icon: <Network className="size-4" aria-hidden="true" /> },
      { id: "accounts", label: "账户", icon: <Users className="size-4" aria-hidden="true" /> },
      { id: "upstreams", label: "上游代理", icon: <Layers className="size-4" aria-hidden="true" /> },
    ],
  },
  {
    label: "系统",
    items: [
      { id: "logs", label: "连接日志", icon: <ScrollText className="size-4" aria-hidden="true" /> },
      { id: "settings", label: "系统设置", icon: <SettingsIcon className="size-4" aria-hidden="true" /> },
    ],
  },
];

export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap((g) => g.items);

/** Sticky off-canvas sidebar body (shared by the desktop aside and the mobile drawer). */
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
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2.5 px-4 py-5">
        <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <Zap className="size-4" aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold leading-tight">FastProxy</div>
          <div className="text-xs text-muted-foreground" translate="no">
            v{version}
          </div>
        </div>
      </div>

      <nav aria-label="主导航" className="flex-1 space-y-5 overflow-y-auto px-3">
        {NAV_GROUPS.map((group) => (
          <div key={group.label}>
            <p className="px-2.5 pb-1.5 text-xs font-medium text-muted-foreground/80">{group.label}</p>
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
                        "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition-colors",
                        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                        active
                          ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                          : "text-sidebar-foreground/70 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground"
                      )}
                    >
                      {item.icon}
                      <span className="truncate">{item.label}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      <div className="p-3">
        <div className="flex items-center gap-2.5 rounded-lg border bg-sidebar-accent/40 p-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
            <ShieldCheck className="size-4" aria-hidden="true" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium leading-tight">管理员</p>
            <p className="text-xs text-muted-foreground">在线</p>
          </div>
          <button
            type="button"
            onClick={onLogout}
            aria-label="退出登录"
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            <LogOut className="size-4" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );
}
