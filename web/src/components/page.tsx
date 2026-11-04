import * as React from "react";
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Field as UIField,
  FieldDescription,
  FieldLabel,
} from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useT } from "@/i18n";

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
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div className="flex min-w-0 flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight text-balance">{title}</h1>
        {description ? (
          <p className="max-w-2xl text-sm leading-relaxed text-muted-foreground text-pretty">
            {description}
          </p>
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
    <Empty className="border border-dashed py-12">
      <EmptyHeader>
        {icon ? <EmptyMedia variant="icon">{icon}</EmptyMedia> : null}
        <EmptyTitle>{title}</EmptyTitle>
        {description ? <EmptyDescription>{description}</EmptyDescription> : null}
      </EmptyHeader>
      {action ? <EmptyContent>{action}</EmptyContent> : null}
    </Empty>
  );
}

/** Metric card modelled on the shadcn dashboard-01 "section cards". */
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
    <Card className="@container/card gap-2 py-4 [--card-spacing:--spacing(5)]">
      <CardHeader className="gap-1.5">
        <CardDescription className="text-xs font-medium">{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tracking-tight tabular-nums @[240px]/card:text-3xl">
          {value}
        </CardTitle>
        {icon ? (
          <CardAction>
            <span
              className="flex size-8 items-center justify-center rounded-md bg-muted/60 text-muted-foreground [&>svg]:size-4"
              aria-hidden="true"
            >
              {icon}
            </span>
          </CardAction>
        ) : null}
      </CardHeader>
      {hint ? (
        <CardContent className="pt-0 text-xs text-muted-foreground">
          <p className="truncate">{hint}</p>
        </CardContent>
      ) : null}
    </Card>
  );
}

export function Mono({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={cn("font-mono text-xs text-muted-foreground", className)}>{children}</span>;
}

const TONE_VARIANT = {
  ok: "secondary",
  warn: "outline",
  fail: "destructive",
  muted: "outline",
} as const;

const TONE_DOT = {
  ok: "bg-emerald-500",
  warn: "bg-amber-500",
  fail: "bg-destructive",
  muted: "bg-muted-foreground/50",
} as const;

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
  return (
    <Badge
      variant={TONE_VARIANT[tone]}
      className={cn(tone === "muted" && "text-muted-foreground", className)}
      title={title}
    >
      <span className={cn("size-1.5 shrink-0 rounded-full", TONE_DOT[tone])} aria-hidden="true" />
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
    <div className="flex min-h-9 flex-wrap items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-2 text-sm">
        <span className="font-medium tabular-nums">{count}</span>
        {detail ? (
          <Badge variant="outline" className="text-muted-foreground">
            {detail}
          </Badge>
        ) : null}
      </div>
      {children ? <div className="flex flex-wrap items-center gap-2">{children}</div> : null}
    </div>
  );
}

/** Header checkbox supporting the indeterminate state. */
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
  return (
    <Checkbox
      checked={checked}
      indeterminate={indeterminate && !checked}
      onCheckedChange={(v) => onCheckedChange(!!v)}
      aria-label={label}
    />
  );
}

/** Row-level selection checkbox. */
export function RowCheckbox({
  checked,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  return <Checkbox checked={checked} onCheckedChange={(v) => onCheckedChange(!!v)} aria-label={label} />;
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

/** Data-table pagination in the style of the shadcn tasks/dashboard example. */
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
  const t = useT();
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = Math.min(total, page * pageSize);
  const sizes = ["20", "50", "100"];
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 px-1">
      <p className="text-sm tabular-nums text-muted-foreground">
        {t("pagination.range", { from, to, total })}
      </p>
      <div className="flex flex-wrap items-center gap-4 lg:gap-6">
        <Select
          items={sizes.map((v) => ({ value: v, label: t("pagination.perPage", { n: v }) }))}
          value={String(pageSize)}
          onValueChange={(v) => onChange(1, Number(v))}
        >
          <SelectTrigger size="sm" className="w-24" aria-label={t("pagination.pageSize")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {sizes.map((v) => (
                <SelectItem key={v} value={v}>
                  {t("pagination.perPage", { n: v })}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <span className="text-sm font-medium tabular-nums">
          {t("pagination.page", { page, pages: pageCount })}
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon-sm"
            className="hidden lg:inline-flex"
            onClick={() => onChange(1, pageSize)}
            disabled={page <= 1}
            aria-label={t("pagination.prev") + " «"}
          >
            <ChevronsLeft />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            onClick={() => onChange(page - 1, pageSize)}
            disabled={page <= 1}
            aria-label={t("pagination.prev")}
          >
            <ChevronLeft />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            onClick={() => onChange(page + 1, pageSize)}
            disabled={page >= pageCount}
            aria-label={t("pagination.next")}
          >
            <ChevronRight />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            className="hidden lg:inline-flex"
            onClick={() => onChange(pageCount, pageSize)}
            disabled={page >= pageCount}
            aria-label={t("pagination.next") + " »"}
          >
            <ChevronsRight />
          </Button>
        </div>
      </div>
    </div>
  );
}

/** Labelled form field built on the shadcn Field primitives. */
export function Field({
  label,
  htmlFor,
  hint,
  invalid,
  children,
}: {
  label: string;
  htmlFor?: string;
  hint?: string;
  invalid?: boolean;
  children: React.ReactNode;
}) {
  return (
    <UIField data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={htmlFor}>{label}</FieldLabel>
      {children}
      {hint ? <FieldDescription>{hint}</FieldDescription> : null}
    </UIField>
  );
}

/** Wraps a data table in the bordered container used by shadcn data-table examples. */
export function TableFrame({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("overflow-hidden rounded-lg border bg-card", className)}>{children}</div>;
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
  size,
  "aria-label": ariaLabel,
}: {
  id?: string;
  value: string;
  onValueChange: (value: string) => void;
  items: SelectOption[];
  className?: string;
  disabled?: boolean;
  size?: "sm" | "default";
  "aria-label"?: string;
}) {
  return (
    <Select
      items={items}
      value={value}
      disabled={disabled}
      onValueChange={(v) => onValueChange((v as string | null) ?? "")}
    >
      <SelectTrigger id={id} size={size} className={cn("w-full", className)} aria-label={ariaLabel}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
