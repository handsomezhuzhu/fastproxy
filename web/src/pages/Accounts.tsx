import * as React from "react";
import { Copy, Ellipsis, Eye, EyeOff, Pencil, Plus, Trash2, Users } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { Spinner } from "@/components/ui/spinner";
import { toast } from "sonner";
import { api, type Account, type Data } from "@/api";
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
import {
  EmptyState,
  Field,
  FormSelect,
  PageHeader,
  PaginationControls,
  SelectAllCheckbox,
  Toolbar,
  usePagination,
  useSelection,
} from "@/components/page";
import { useI18n } from "@/i18n";
import { bytes } from "@/lib/utils";
import type { CopyPreset } from "@/components/CopyWizard";

type BulkAction = "enable" | "disable" | "delete";

export function AccountsPage({
  data,
  reload,
  onCopy,
}: {
  data: Data;
  reload: () => Promise<void>;
  onCopy: (p: CopyPreset) => void;
}) {
  const { t } = useI18n();
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Account | null>(null);
  const [form, setForm] = React.useState({
    username: "",
    password: "",
    port_id: "",
    upstream_id: "",
    remark: "",
  });
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Account | null>(null);
  const [revealed, setRevealed] = React.useState<Record<string, boolean>>({});

  // Bulk state
  const sel = useSelection();
  const [bulkAction, setBulkAction] = React.useState<BulkAction | null>(null);
  const [bulkBusy, setBulkBusy] = React.useState(false);
  const [bulkConfirmDelete, setBulkConfirmDelete] = React.useState(false);

  const pag = usePagination(data.accounts);

  const status = Object.fromEntries(data.status.accounts.map((s) => [s.id, s]));
  const upstreamName = (id: string) => data.upstreams.find((u) => u.id === id)?.name ?? "–";
  const portLabel = (id: string) => {
    const p = data.ports.find((x) => x.id === id);
    return p ? t("copy.portOption", { port: p.port }) : "–";
  };

  function openCreate() {
    setEditing(null);
    setForm({ username: "", password: "", port_id: data.ports[0]?.id ?? "", upstream_id: "", remark: "" });
    setError("");
    setOpen(true);
  }
  function openEdit(a: Account) {
    setEditing(a);
    setForm({
      username: a.username,
      password: a.password,
      port_id: a.port_id || "",
      upstream_id: a.upstream_id || "",
      remark: a.remark || "",
    });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!form.port_id) {
      setError(t("accounts.error.portRequired"));
      return;
    }
    setBusy(true);
    setError("");
    const body = {
      username: form.username.trim(),
      password: form.password,
      port_id: form.port_id,
      upstream_id: form.upstream_id,
      remark: form.remark,
    };
    try {
      if (editing) await api.updateAccount(editing.id, body);
      else await api.createAccount(body);
      toast.success(t("accounts.saved"));
      setOpen(false);
      await reload();
    } catch (err: any) {
      setError(err?.message || t("common.saveFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function toggle(a: Account, enabled: boolean) {
    try {
      await api.toggleAccount(a.id, enabled);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || t("common.operationFailed"));
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deleteAccount(deleting.id);
      toast.success(t("accounts.deleted", { name: deleting.username }));
      setDeleting(null);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || t("common.deleteFailed"));
    }
  }

  const actionLabels: Record<BulkAction, string> = {
    enable: t("common.enable"),
    disable: t("common.disable"),
    delete: t("common.delete"),
  };

  async function runBulk(action: BulkAction) {
    const ids = [...sel.selected];
    if (ids.length === 0) return;
    setBulkBusy(true);
    try {
      const r = await api.bulkAccounts(ids, action);
      const failed = r.errors ?? [];
      const done = r.updated ?? Math.max(0, ids.length - failed.length);
      if (failed.length > 0) {
        toast.error(t("accounts.bulkFailed", { n: failed.length, action: actionLabels[action], err: failed[0].error }));
        if (done > 0) toast.success(t("accounts.bulkEach", { n: done, action: actionLabels[action] }));
      } else {
        toast.success(t("accounts.bulkEach", { n: done, action: actionLabels[action] }));
      }
      sel.clear();
      await reload();
    } catch (err: any) {
      toast.error(err?.message || t("common.bulkFailed"));
    } finally {
      setBulkBusy(false);
      setBulkConfirmDelete(false);
    }
  }

  function askBulk(action: BulkAction) {
    setBulkAction(action);
    if (action === "delete") setBulkConfirmDelete(true);
    else runBulk(action);
  }

  // 默认账号随端口删除，不参与批量选择
  const selectable = (a: Account) => !a.is_default;
  const pageIDs = pag.pageItems.filter(selectable).map((a) => a.id);
  const pageAllSelected = pageIDs.length > 0 && pageIDs.every((id) => sel.selected.has(id));
  const pageSomeSelected = pageIDs.some((id) => sel.selected.has(id));

  const portItems = data.ports.map((p) => ({ value: p.id, label: t("copy.portOption", { port: p.port }) }));
  const upstreamItems = [
    { value: "", label: t("common.redirect") },
    ...data.upstreams.map((u) => ({ value: u.id, label: `${u.name}（${u.type} · ${u.host}）` })),
  ];

  const enabledCount = data.accounts.filter((a) => a.enabled).length;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader title={t("accounts.title")} description={t("accounts.description")} />

      {data.accounts.length === 0 ? (
        <EmptyState
          icon={<Users className="size-5" aria-hidden="true" />}
          title={t("accounts.empty.title")}
          description={t("accounts.empty.desc")}
          action={
            <Button onClick={openCreate} disabled={data.ports.length === 0}>
              <Plus className="size-4" aria-hidden="true" /> {t("accounts.create")}
            </Button>
          }
        />
      ) : (
        <>
          <Toolbar
            count={t("accounts.count", { n: data.accounts.length })}
            detail={t("accounts.enabledCount", { n: enabledCount })}
          >
            <Button size="sm" onClick={openCreate} disabled={data.ports.length === 0}>
              <Plus className="size-4" aria-hidden="true" /> {t("accounts.create")}
            </Button>
          </Toolbar>

          {sel.selected.size > 0 ? (
            <div
              className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2"
              role="status"
            >
              <span className="text-sm tabular-nums">{t("common.selected", { n: sel.selected.size })} ·</span>
              <Button size="sm" variant="outline" onClick={() => askBulk("enable")} disabled={bulkBusy}>
                {t("common.enable")}
              </Button>
              <Button size="sm" variant="outline" onClick={() => askBulk("disable")} disabled={bulkBusy}>
                {t("common.disable")}
              </Button>
              <Button size="sm" variant="destructive" onClick={() => askBulk("delete")} disabled={bulkBusy}>
                {bulkBusy ? <Spinner data-icon="inline-start" /> : null}
                {t("common.delete")}
              </Button>
              <Button size="sm" variant="ghost" onClick={sel.clear} aria-label={t("common.clearSelection")}>
                {t("common.cancel")}
              </Button>
            </div>
          ) : null}

          <div className="overflow-hidden rounded-lg border">
            <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <SelectAllCheckbox
                    checked={pageAllSelected}
                    indeterminate={!pageAllSelected && pageSomeSelected}
                    onCheckedChange={(v) => sel.toggleMany(pageIDs, v)}
                    label={t("common.selectAllPage")}
                  />
                </TableHead>
                <TableHead className="w-16">{t("common.enabled")}</TableHead>
                <TableHead>{t("accounts.col.username")}</TableHead>
                <TableHead>{t("accounts.col.password")}</TableHead>
                <TableHead>{t("common.port")}</TableHead>
                <TableHead>{t("common.upstream")}</TableHead>
                <TableHead className="text-right">{t("accounts.col.conns")}</TableHead>
                <TableHead className="text-right">{t("accounts.col.traffic")}</TableHead>
                <TableHead>{t("common.remark")}</TableHead>
                <TableHead className="w-12">
                  <span className="sr-only">{t("common.actions")}</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pag.pageItems.map((a) => {
                const s = status[a.id];
                const shown = revealed[a.id];
                const checked = sel.selected.has(a.id);
                return (
                  <TableRow key={a.id} data-state={checked ? "selected" : undefined}>
                    <TableCell>
                      <Checkbox
                        checked={checked}
                        disabled={!selectable(a)}
                        title={selectable(a) ? undefined : t("accounts.defaultTip")}
                        onCheckedChange={(checked: boolean) => sel.toggle(a.id, checked)}
                        aria-label={`${t("common.account")} ${a.username}`}
                      />
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={a.enabled}
                        onCheckedChange={(v: boolean) => toggle(a, v)}
                        aria-label={`${t("common.account")} ${a.username} ${t("common.enabled")}`}
                      />
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5">
                        <span className="font-medium">{a.username}</span>
                        {a.is_default ? <Badge variant="outline">{t("accounts.default")}</Badge> : null}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5">
                        <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                          {shown ? a.password : "••••••"}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => setRevealed((r) => ({ ...r, [a.id]: !r[a.id] }))}
                          aria-label={shown ? t("accounts.hidePassword") : t("accounts.showPassword")}
                        >
                          {shown ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                        </Button>
                      </span>
                    </TableCell>
                    <TableCell className="text-sm">
                      {a.is_default ? (
                        <span className="text-muted-foreground">{portLabel(a.port_id)}</span>
                      ) : (
                        portLabel(a.port_id)
                      )}
                    </TableCell>
                    <TableCell className="text-sm">
                      {a.upstream_id ? (
                        upstreamName(a.upstream_id)
                      ) : (
                        <span className="text-muted-foreground">{t("common.redirect")}</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-sm tabular-nums">
                      {s?.conns_cur ?? 0} / {s?.conns_total ?? 0}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-sm tabular-nums">
                      ↑{bytes(s?.bytes_out ?? 0)} ↓{bytes(s?.bytes_in ?? 0)}
                    </TableCell>
                    <TableCell className="max-w-[140px] truncate text-xs text-muted-foreground">
                      {a.remark || "–"}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                          aria-label={`${t("common.account")} ${a.username} ${t("common.actions")}`}
                        >
                          <Ellipsis className="size-4" aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-40">
                          <DropdownMenuItem onClick={() => openEdit(a)}>
                            <Pencil className="size-4" aria-hidden="true" /> {t("common.edit")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onCopy({ dim: "account", id: a.id })}>
                            <Copy className="size-4" aria-hidden="true" /> {t("shell.copyAccess")}
                          </DropdownMenuItem>
                          {!a.is_default ? (
                            <>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem variant="destructive" onClick={() => setDeleting(a)}>
                                <Trash2 className="size-4" aria-hidden="true" /> {t("common.delete")}
                              </DropdownMenuItem>
                            </>
                          ) : null}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
            </Table>
          </div>

          <PaginationControls
            page={pag.page}
            pageSize={pag.pageSize}
            total={pag.total}
            onChange={pag.onChange}
          />
        </>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing ? t("accounts.dialog.edit", { name: editing.username }) : t("accounts.dialog.create")}
            </DialogTitle>
            <DialogDescription>{t("accounts.dialog.desc")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="flex flex-col gap-5">
            <Field label={t("accounts.field.username")} htmlFor="acc-username">
              <Input
                id="acc-username"
                name="username"
                autoComplete="off"
                spellCheck={false}
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                placeholder="alice"
              />
            </Field>
            <Field label={t("common.password")} htmlFor="acc-password" hint={t("accounts.field.passwordHint")}>
              <Input
                id="acc-password"
                name="password"
                autoComplete="new-password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                placeholder={t("accounts.field.passwordPlaceholder")}
              />
            </Field>
            <Field
              label={t("accounts.field.port")}
              htmlFor="acc-port"
              hint={
                editing?.is_default
                  ? t("accounts.field.portDefault")
                  : data.ports.length === 0
                    ? t("accounts.field.portNone")
                    : undefined
              }
            >
              <FormSelect
                id="acc-port"
                value={form.port_id}
                onValueChange={(v) => setForm({ ...form, port_id: v })}
                items={portItems}
                disabled={!!editing?.is_default}
              />
            </Field>
            <Field label={t("accounts.field.upstream")} htmlFor="acc-upstream">
              <FormSelect
                id="acc-upstream"
                value={form.upstream_id}
                onValueChange={(v) => setForm({ ...form, upstream_id: v })}
                items={upstreamItems}
              />
            </Field>
            <Field label={t("common.remark")} htmlFor="acc-remark">
              <Input
                id="acc-remark"
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
                placeholder={t("accounts.field.remarkPlaceholder")}
              />
            </Field>
            {error ? (
              <p className="text-xs text-destructive" role="alert">
                {error}
              </p>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={busy || data.ports.length === 0}>
                {busy ? <Spinner data-icon="inline-start" /> : null}
                <span className="inline-block min-w-16 text-center">
                  {busy ? t("common.saving") : t("common.save")}
                </span>
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("accounts.delete.title", { name: deleting?.username ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("accounts.delete.desc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={remove}>{t("common.delete")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={bulkConfirmDelete} onOpenChange={(v) => !v && setBulkConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.bulkDelete", { n: sel.selected.size })}</AlertDialogTitle>
            <AlertDialogDescription>{t("accounts.bulkDelete.desc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => runBulk(bulkAction ?? "delete")} disabled={bulkBusy}>
              {bulkBusy ? <Spinner data-icon="inline-start" /> : null}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
