import * as React from "react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold tracking-tight text-balance">{title}</h1>
        {description ? (
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground text-pretty">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
      {icon ? (
        <div className="flex size-11 items-center justify-center rounded-full bg-muted text-muted-foreground">
          {icon}
        </div>
      ) : null}
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        {description ? (
          <p className="mx-auto max-w-sm text-xs text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {action}
    </div>
  );
}

export function StatCard({
  label,
  value,
  hint,
  icon,
}: {
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
  icon?: React.ReactNode;
}) {
  return (
    <Card className="py-0">
      <CardContent className="p-4">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-muted-foreground">{label}</span>
          {icon ? <span className="text-muted-foreground" aria-hidden="true">{icon}</span> : null}
        </div>
        <div className="mt-2 text-2xl font-semibold tabular-nums tracking-tight">{value}</div>
        {hint ? <div className="mt-0.5 text-xs text-muted-foreground">{hint}</div> : null}
      </CardContent>
    </Card>
  );
}

export function Mono({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={cn("font-mono text-xs text-muted-foreground", className)}>{children}</span>;
}

/** Status badge with a small leading dot, consistent across all tables. */
export function DotBadge({
  tone = "ok",
  children,
  className,
  title,
}: {
  tone?: "ok" | "warn" | "fail" | "muted";
  children: React.ReactNode;
  className?: string;
  title?: string;
}) {
  const tones = {
    ok: { badge: "bg-emerald-500/15 text-emerald-400", dot: "bg-emerald-400" },
    warn: { badge: "bg-amber-500/15 text-amber-400", dot: "bg-amber-400" },
    fail: { badge: "bg-destructive/15 text-destructive", dot: "bg-destructive" },
    muted: { badge: "text-muted-foreground", dot: "bg-muted-foreground/60" },
  } as const;
  const t = tones[tone];
  return (
    <Badge variant="secondary" className={cn("gap-1.5", t.badge, className)} title={title}>
      <span className={cn("size-1.5 shrink-0 rounded-full", t.dot)} aria-hidden="true" />
      {children}
    </Badge>
  );
}

/** Uniform list-page toolbar: count summary on the left, filters/actions on the right. */
export function Toolbar({
  count,
  detail,
  children,
}: {
  count: React.ReactNode;
  detail?: React.ReactNode;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-baseline gap-2 text-sm">
        <span className="font-medium tabular-nums">{count}</span>
        {detail ? <span className="text-xs text-muted-foreground">{detail}</span> : null}
      </div>
      {children ? <div className="flex flex-wrap items-center gap-2">{children}</div> : null}
    </div>
  );
}

/** Labelled form field with a consistent vertical rhythm. */
export function Field({
  label,
  htmlFor,
  hint,
  children,
}: {
  label: string;
  htmlFor?: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}

export type SelectOption = { value: string; label: string };

/** Official shadcn Select with string values and a label map. */
export function FormSelect({
  id,
  value,
  onValueChange,
  items,
  className,
  disabled,
  "aria-label": ariaLabel,
}: {
  id?: string;
  value: string;
  onValueChange: (value: string) => void;
  items: SelectOption[];
  className?: string;
  disabled?: boolean;
  "aria-label"?: string;
}) {
  return (
    <Select
      items={items}
      value={value}
      disabled={disabled}
      onValueChange={(v) => onValueChange((v as string | null) ?? "")}
    >
      <SelectTrigger id={id} className={cn("w-full", className)} aria-label={ariaLabel}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
