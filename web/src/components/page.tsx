import * as React from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
        <h1 className="text-balance text-xl font-semibold tracking-tight md:text-2xl">{title}</h1>
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
        <div className="flex size-11 items-center justify-center rounded-md bg-muted text-muted-foreground">
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
    <Card className="border-l-4 border-l-neutral-900 py-0 shadow-sm transition-shadow duration-200 hover:shadow-md dark:border-l-neutral-100">
      <CardContent className="p-4 md:p-6">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">{label}</span>
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

/** Status badge with a small leading dot, consistent across all tables (monochrome mapping). */
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
    ok: { badge: "bg-foreground text-background", dot: "bg-background" },
    warn: { badge: "bg-muted text-foreground", dot: "bg-foreground/50" },
    fail: { badge: "border border-foreground/40 font-medium text-foreground", dot: "bg-foreground" },
    muted: { badge: "text-muted-foreground", dot: "bg-muted-foreground/60" },
  } as const;
  const t = tones[tone];
  return (
    <Badge variant="secondary" className={cn("gap-1.5", t.badge, className)} title={title}>
      <span className={cn("size-1.5 shrink-0 rounded-sm", t.dot)} aria-hidden="true" />
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
      <div className="flex min-w-0 items-baseline gap-2 text-sm">
        <span className="font-medium tabular-nums">{count}</span>
        {detail ? <span className="truncate text-xs text-muted-foreground">{detail}</span> : null}
      </div>
      {children ? <div className="flex flex-wrap items-center gap-2">{children}</div> : null}
    </div>
  );
}

/** Header checkbox supporting the indeterminate state (native input with ref sync). */
export function SelectAllCheckbox({
  checked,
  indeterminate = false,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  indeterminate?: boolean;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  const ref = React.useRef<HTMLInputElement>(null);
  React.useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate && !checked;
  }, [indeterminate, checked]);
  return (
    <input
      ref={ref}
      type="checkbox"
      className="size-3.5 accent-primary focus-visible:outline-2 focus-visible:outline-ring"
      checked={checked}
      onChange={(e) => onCheckedChange(e.target.checked)}
      aria-label={label}
    />
  );
}

/**
 * Cross-page row selection. The Set is intentionally NOT reset on page change,
 * so bulk actions can span multiple pages (rely on the bulk bar's count).
 */
export function useSelection() {
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const toggle = React.useCallback((id: string, checked: boolean) => {
    setSelected((s) => {
      const next = new Set(s);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);
  const toggleMany = React.useCallback((ids: string[], checked: boolean) => {
    setSelected((s) => {
      const next = new Set(s);
      for (const id of ids) {
        if (checked) next.add(id);
        else next.delete(id);
      }
      return next;
    });
  }, []);
  const clear = React.useCallback(() => setSelected(new Set()), []);
  return { selected, toggle, toggleMany, clear };
}

/** Client-side slicing over a fully loaded array. Page clamps when the list shrinks. */
export function usePagination<T>(items: T[] | null | undefined, initialSize = 20) {
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(initialSize);
  const list = items ?? [];
  const total = list.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const cur = Math.min(page, pageCount);
  const start = (cur - 1) * pageSize;
  const onChange = React.useCallback((nextPage: number, nextSize: number) => {
    setPage(nextPage);
    setPageSize(nextSize);
  }, []);
  return {
    pageItems: list.slice(start, start + pageSize),
    page: cur,
    pageSize,
    total,
    onChange,
  };
}

/** Page-size Select + "第 X / Y 页" + prev/next icon buttons. Shows the row range ("第 41-60 / 共 241 条"). */
export function PaginationControls({
  page,
  pageSize,
  total,
  onChange,
}: {
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number, pageSize: number) => void;
}) {
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = Math.min(total, page * pageSize);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-xs tabular-nums text-muted-foreground">
        第 {from}-{to} / 共 {total} 条
      </p>
      <div className="flex items-center gap-2">
        <Select
          items={["20", "50", "100"].map((v) => ({ value: v, label: `${v} / 页` }))}
          value={String(pageSize)}
          onValueChange={(v) => onChange(1, Number(v))}
        >
          <SelectTrigger size="sm" className="w-24" aria-label="每页条数">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {["20", "50", "100"].map((v) => (
              <SelectItem key={v} value={v}>
                {v} / 页
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-xs tabular-nums text-muted-foreground">
          第 {page} / {pageCount} 页
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => onChange(page - 1, pageSize)}
          disabled={page <= 1}
          aria-label="上一页"
        >
          <ChevronLeft className="size-4" aria-hidden="true" />
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => onChange(page + 1, pageSize)}
          disabled={page >= pageCount}
          aria-label="下一页"
        >
          <ChevronRight className="size-4" aria-hidden="true" />
        </Button>
      </div>
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
