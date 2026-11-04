import * as React from "react";
import { Ellipsis, Pencil, Plus, RefreshCw, Rss, Trash2 } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { toast } from "sonner";
import { api, type Subscription, type SubscriptionInput } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Switch } from "@/components/ui/switch";
import { DotBadge, EmptyState, Field, PageHeader, Toolbar } from "@/components/page";
import { useI18n, type Lang } from "@/i18n";
import { explainErrorLine } from "@/lib/errors";
import { timeAgo } from "@/lib/utils";

type FormState = { name: string; url: string; enabled: boolean; intervalMin: string };
const EMPTY_FORM: FormState = { name: "", url: "", enabled: true, intervalMin: "60" };

function subscriptionStatus(s: Subscription, t: (k: string, v?: any) => string, lang: Lang) {
  if (s.last_error) {
    return (
      <div className="flex flex-col gap-0.5">
        <DotBadge tone="fail">{t("subscriptions.status.failed")}</DotBadge>
        <p className="max-w-[220px] truncate text-xs text-destructive" title={s.last_error}>
          {s.last_error}
        </p>
      </div>
    );
  }
  if (!s.last_fetch_at) return <DotBadge tone="muted">{t("subscriptions.status.never")}</DotBadge>;
  return (
    <div className="flex flex-col gap-0.5">
      <DotBadge tone="ok">{t("subscriptions.status.ok")}</DotBadge>
      <p className="text-xs text-muted-foreground">
        {timeAgo(new Date(s.last_fetch_at * 1000).toISOString(), lang)}
      </p>
    </div>
  );
}

export function SubscriptionsPage({ reload }: { reload: () => Promise<void> }) {
  const { t, lang } = useI18n();
  const [items, setItems] = React.useState<Subscription[] | null>(null);
  const [busy, setBusy] = React.useState<Record<string, boolean>>({});
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Subscription | null>(null);
  const [form, setForm] = React.useState<FormState>(EMPTY_FORM);
  const [error, setError] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Subscription | null>(null);

  const load = React.useCallback(async () => {
    try {
      const r = await api.subscriptions();
      setItems(r.items ?? []);
    } catch (err: any) {
      setItems((prev) => prev ?? []);
      toast.error(explainErrorLine(err?.message, lang) || t("subscriptions.loadFailed"));
    }
  }, [t, lang]);

  React.useEffect(() => {
    load();
  }, [load]);

  function openCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setError("");
    setOpen(true);
  }
  function openEdit(s: Subscription) {
    setEditing(s);
    setForm({
      name: s.name,
      url: s.url,
      enabled: s.enabled,
      intervalMin: String(Math.max(5, Math.round(s.interval_sec / 60))),
    });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const minutes = Number(form.intervalMin);
    if (!Number.isFinite(minutes) || minutes < 5 || minutes > 1440) {
      setError(t("subscriptions.error.interval"));
      return;
    }
    const body: SubscriptionInput = {
      name: form.name.trim(),
      url: form.url.trim(),
      enabled: form.enabled,
      interval_sec: Math.round(minutes * 60),
    };
    if (!body.name) {
      setError(t("subscriptions.error.name"));
      return;
    }
    if (!/^https?:\/\//i.test(body.url)) {
      setError(t("subscriptions.error.url"));
      return;
    }
    setSaving(true);
    try {
      if (editing) await api.updateSubscription(editing.id, body);
      else await api.createSubscription(body);
      toast.success(t("subscriptions.saved"));
      setOpen(false);
      await load();
      await reload();
    } catch (err: any) {
      setError(explainErrorLine(err?.message, lang) || t("subscriptions.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  async function refreshOne(s: Subscription) {
    setBusy((b) => ({ ...b, [s.id]: true }));
    try {
      const r = await api.refreshSubscription(s.id);
      if (r.result.error) {
        toast.error(`${s.name}: ${explainErrorLine(r.result.error, lang) || r.result.error}`);
      } else {
        toast.success(
          t("subscriptions.refreshed", {
            added: r.result.added,
            updated: r.result.updated,
            removed: r.result.removed,
          })
        );
      }
      await load();
      await reload();
    } catch (err: any) {
      toast.error(explainErrorLine(err?.message, lang) || t("subscriptions.refreshFailed"));
    } finally {
      setBusy((b) => ({ ...b, [s.id]: false }));
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteSubscription(deleting.id, true);
      toast.success(t("subscriptions.deleted", { name: deleting.name }));
      setDeleting(null);
      await load();
      await reload();
    } catch (err: any) {
      toast.error(explainErrorLine(err?.message, lang) || t("subscriptions.deleteFailed"));
    }
  }

  if (items === null) {
    return (
      <div className="flex flex-col gap-5">
        <PageHeader title={t("subscriptions.title")} description={t("subscriptions.desc")} />
        <div className="flex items-center justify-center py-12">
          <Spinner className="size-5 text-muted-foreground" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader title={t("subscriptions.title")} description={t("subscriptions.desc")} />

      {items.length === 0 ? (
        <EmptyState
          icon={<Rss className="size-5" aria-hidden="true" />}
          title={t("subscriptions.empty.title")}
          description={t("subscriptions.empty.desc")}
          action={
            <Button onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> {t("subscriptions.create")}
            </Button>
          }
        />
      ) : (
        <>
          <Toolbar count={t("subscriptions.count", { n: items.length })}>
            <Button size="sm" onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> {t("subscriptions.create")}
            </Button>
          </Toolbar>

          <div className="overflow-hidden rounded-lg border bg-card">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("subscriptions.col.name")}</TableHead>
                  <TableHead>{t("subscriptions.col.url")}</TableHead>
                  <TableHead>{t("subscriptions.col.nodes")}</TableHead>
                  <TableHead>{t("subscriptions.col.interval")}</TableHead>
                  <TableHead>{t("subscriptions.col.status")}</TableHead>
                  <TableHead className="w-12">
                    <span className="sr-only">{t("common.actions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell>
                      <div className="font-medium">{s.name}</div>
                      {!s.enabled ? (
                        <Badge variant="outline" className="mt-1">
                          {t("subscriptions.disabled")}
                        </Badge>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      <div className="max-w-[280px] truncate font-mono text-[13px]" title={s.url}>
                        {s.url}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm tabular-nums">{s.node_count}</TableCell>
                    <TableCell className="text-sm tabular-nums">
                      {Math.round(s.interval_sec / 60)} {t("subscriptions.minutes")}
                    </TableCell>
                    <TableCell>{subscriptionStatus(s, t, lang)}</TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                          aria-label={t("subscriptions.rowMenu", { name: s.name })}
                        >
                          {busy[s.id] ? (
                            <Spinner data-icon="inline-start" />
                          ) : (
                            <Ellipsis className="size-4" aria-hidden="true" />
                          )}
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-44">
                          <DropdownMenuItem
                            onClick={() => refreshOne(s)}
                            disabled={busy[s.id]}
                          >
                            <RefreshCw className="size-4" aria-hidden="true" />
                            {t("subscriptions.refreshNow")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => openEdit(s)}>
                            <Pencil className="size-4" aria-hidden="true" /> {t("common.edit")}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(s)}>
                            <Trash2 className="size-4" aria-hidden="true" /> {t("common.delete")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editing
                ? t("subscriptions.dialog.edit", { name: editing.name })
                : t("subscriptions.dialog.create")}
            </DialogTitle>
            <DialogDescription>{t("subscriptions.dialog.desc")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="flex flex-col gap-5" noValidate>
            <Field label={t("subscriptions.field.name")} htmlFor="sub-name">
              <Input
                id="sub-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="机场订阅"
              />
            </Field>
            <Field
              label={t("subscriptions.field.url")}
              htmlFor="sub-url"
              hint={t("subscriptions.field.urlHint")}
            >
              <Input
                id="sub-url"
                className="font-mono"
                value={form.url}
                onChange={(e) => setForm({ ...form, url: e.target.value })}
                placeholder="https://example.com/api/v1/client/subscribe?token=..."
                spellCheck={false}
              />
            </Field>
            <div className="grid grid-cols-2 gap-4">
              <Field label={t("subscriptions.field.interval")} htmlFor="sub-interval">
                <Input
                  id="sub-interval"
                  type="number"
                  inputMode="numeric"
                  min={5}
                  max={1440}
                  value={form.intervalMin}
                  onChange={(e) => setForm({ ...form, intervalMin: e.target.value })}
                />
              </Field>
              <div className="flex items-end justify-between gap-4 pb-0.5">
                <div>
                  <div className="text-sm font-medium">{t("subscriptions.field.enabled")}</div>
                  <p className="text-xs text-muted-foreground">
                    {t("subscriptions.field.enabledHint")}
                  </p>
                </div>
                <Switch
                  checked={form.enabled}
                  onCheckedChange={(v) => setForm({ ...form, enabled: Boolean(v) })}
                  aria-label={t("subscriptions.field.enabled")}
                />
              </div>
            </div>
            {error ? (
              <p className="text-xs text-destructive" role="alert">
                {error}
              </p>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? <Spinner data-icon="inline-start" /> : null}
                <span className="inline-block min-w-16 text-center">
                  {saving ? t("common.saving") : t("common.save")}
                </span>
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("subscriptions.delete.title", { name: deleting?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>{t("subscriptions.delete.desc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={remove}>{t("common.delete")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
