import * as React from "react";
import { Check, Copy, Ellipsis, Loader2, Network, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, DEFAULT_PORT_RANGE, type Account, type Data, type Port, type PortRange } from "@/api";
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
  DotBadge,
  EmptyState,
  Field,
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

/** Backend answers 400 for out-of-range ports; recognise both the structured and free-text forms. */
function looksOutOfRange(msg: string): boolean {
  return /port_range|port range|端口.*范围|范围.*端口|out of range|超出/i.test(msg || "");
}

export function PortsPage({
  data,
  reload,
  onCopy,
}: {
  data: Data;
  reload: () => Promise<void>;
  onCopy: (p: CopyPreset) => void;
}) {
  const { t } = useI18n();
  const range: PortRange = data.port_range ?? DEFAULT_PORT_RANGE;
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Port | null>(null);
  const [form, setForm] = React.useState({ port: "", remark: "" });
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [deleting, setDeleting] = React.useState<Port | null>(null);
  const [toggling, setToggling] = React.useState(false);
  const [created, setCreated] = React.useState<{ port: number; account: Account } | null>(null);
  const [copied, setCopied] = React.useState(false);

  // Bulk state
  const sel = useSelection();
  const [bulkAction, setBulkAction] = React.useState<BulkAction | null>(null);
  const [bulkBusy, setBulkBusy] = React.useState(false);
  const [bulkConfirmDelete, setBulkConfirmDelete] = React.useState(false);

  const pag = usePagination(data.ports);

  const status = Object.fromEntries(data.status.ports.map((s) => [s.id, s]));
  const listening = data.ports.filter((p) => status[p.id]?.listening).length;

  function openCreate() {
    setEditing(null);
    setForm({ port: "", remark: "" });
    setError("");
    setOpen(true);
  }
  function openEdit(p: Port) {
    setEditing(p);
    setForm({ port: String(p.port), remark: p.remark || "" });
    setError("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const num = Number(form.port);
    if (!Number.isInteger(num) || num < 1 || num > 65535) {
      setError(t("ports.error.invalid"));
      return;
    }
    if (num < range.min || num > range.max) {
      setError(t("ports.error.outOfRange", { min: range.min, max: range.max }));
      return;
    }
    setBusy(true);
    try {
      if (editing) {
        const res = await api.updatePort(editing.id, { port: num, remark: form.remark });
        if (res.warning) toast.warning(t("ports.savedWarn", { warn: res.warning }));
        else toast.success(t("ports.saved"));
      } else {
        const res = await api.createPort({ port: num, remark: form.remark });
        // saved.port 可能是完整 Port 对象（React error #31 修复：不得直接渲染对象）
        const savedPort = res.saved?.port;
        const portNum =
          savedPort && typeof savedPort === "object"
            ? savedPort.port
            : Number(savedPort ?? form.port);
        const acc = res.saved?.default_account;
        if (acc) {
          // 不改变页面状态：保存成功后弹出默认账号提示。
          setCreated({ port: portNum, account: acc });
          setCopied(false);
        } else if (res.warning) {
          toast.warning(t("ports.savedWarn", { warn: res.warning }));
        } else {
          toast.success(t("ports.created"));
        }
      }
      setOpen(false);
      await reload();
    } catch (err: any) {
      const msg = err?.message || t("common.saveFailed");
      // 后端越界 400：优先显示本地化的范围说明，原始信息附在括号里。
      setError(
        looksOutOfRange(msg)
          ? t("ports.error.backendRange", { min: range.min, max: range.max })
          : msg
      );
    } finally {
      setBusy(false);
    }
  }

  async function toggle(p: Port, enabled: boolean) {
    setToggling(true);
    try {
      await api.togglePort(p.id, enabled);
      toast.success(enabled ? t("ports.toggledOn", { port: p.port }) : t("ports.toggledOff", { port: p.port }));
      await reload();
    } catch (err: any) {
      toast.error(err?.message || t("common.operationFailed"));
    } finally {
      setToggling(false);
    }
  }

  async function remove() {
    if (!deleting) return;
    try {
      await api.deletePort(deleting.id);
      toast.success(t("ports.deleted", { port: deleting.port }));
      setDeleting(null);
      await reload();
    } catch (err: any) {
      toast.error(err?.message || t("common.deleteFailed"));
    }
  }

  async function copyAccount() {
    if (!created) return;
    const text = t("ports.copy.desc", {
      host: window.location.hostname,
      port: created.port,
      user: created.account.username,
      pass: created.account.password,
    });
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success(t("ports.copy.done"));
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      toast.error(t("common.copyFailed"));
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
      const r = await api.bulkPorts(ids, action);
      const failed = r.errors ?? [];
      const done = r.updated ?? Math.max(0, ids.length - failed.length);
      if (failed.length > 0) {
        toast.error(t("ports.bulkFailed", { n: failed.length, action: actionLabels[action], err: failed[0].error }));
        if (done > 0) toast.success(t("ports.bulkEach", { n: done, action: actionLabels[action] }));
      } else {
        toast.success(t("ports.bulkEach", { n: done, action: actionLabels[action] }));
      }
      sel.clear();
      await reload();
    } catch (err: any) {
      // 后端批量接口未上线（404）或其他错误：友好提示，不清空选择
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

  const pageIDs = pag.pageItems.map((p) => p.id);
  const pageAllSelected = pageIDs.length > 0 && pageIDs.every((id) => sel.selected.has(id));
  const pageSomeSelected = pageIDs.some((id) => sel.selected.has(id));

  return (
    <div className="space-y-5">
      <PageHeader title={t("ports.title")} description={t("ports.description")} />

      {data.ports.length === 0 ? (
        <EmptyState
          icon={<Network className="size-5" aria-hidden="true" />}
          title={t("ports.empty.title")}
          description={t("ports.empty.desc", { min: range.min, max: range.max })}
          action={
            <Button onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> {t("ports.create")}
            </Button>
          }
        />
      ) : (
        <>
          <Toolbar
            count={t("ports.count", { n: data.ports.length })}
            detail={t("ports.listening", { n: listening })}
          >
            <Button size="sm" onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> {t("ports.create")}
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
                {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
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
                <TableHead className="w-16">{t("ports.col.enabled")}</TableHead>
                <TableHead>{t("common.port")}</TableHead>
                <TableHead>{t("ports.col.status")}</TableHead>
                <TableHead className="text-right">{t("ports.col.accounts")}</TableHead>
                <TableHead className="text-right">{t("ports.col.conns")}</TableHead>
                <TableHead className="text-right">{t("ports.col.traffic")}</TableHead>
                <TableHead>{t("ports.col.remark")}</TableHead>
                <TableHead className="w-12">
                  <span className="sr-only">{t("common.actions")}</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pag.pageItems.map((p) => {
                const s = status[p.id];
                const bound = data.accounts.filter((a) => a.port_id === p.id).length;
                const checked = sel.selected.has(p.id);
                return (
                  <TableRow key={p.id} data-state={checked ? "selected" : undefined}>
                    <TableCell>
                      <input
                        type="checkbox"
                        className="size-3.5 accent-primary focus-visible:outline-2 focus-visible:outline-ring"
                        checked={checked}
                        onChange={(e) => sel.toggle(p.id, e.target.checked)}
                        aria-label={`${t("common.port")} ${p.port}`}
                      />
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={p.enabled}
                        disabled={toggling}
                        onCheckedChange={(v: boolean) => toggle(p, v)}
                        aria-label={t("ports.col.enabled")}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="font-medium tabular-nums">{p.port}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {window.location.hostname}:{p.port}
                      </div>
                    </TableCell>
                    <TableCell>
                      {!p.enabled ? (
                        <DotBadge tone="muted">{t("ports.status.disabled")}</DotBadge>
                      ) : s?.listening ? (
                        <DotBadge tone="ok">{t("ports.status.listening")}</DotBadge>
                      ) : (
                        <DotBadge tone="fail" title={s?.error}>{t("ports.status.notListening")}</DotBadge>
                      )}
                      {s?.error ? (
                        <p className="mt-1 max-w-[220px] truncate text-xs text-destructive" title={s.error}>
                          {s.error}
                        </p>
                      ) : null}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{bound}</TableCell>
                    <TableCell className="whitespace-nowrap text-right text-sm tabular-nums">
                      {s?.conns_cur ?? 0} / {s?.conns_total ?? 0}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-sm tabular-nums">
                      ↑{bytes(s?.bytes_out ?? 0)} ↓{bytes(s?.bytes_in ?? 0)}
                    </TableCell>
                    <TableCell className="max-w-[160px] truncate text-xs text-muted-foreground">
                      {p.remark || "–"}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                          aria-label={`${t("common.port")} ${p.port} ${t("common.actions")}`}
                        >
                          <Ellipsis className="size-4" aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-40">
                          <DropdownMenuItem onClick={() => openEdit(p)}>
                            <Pencil className="size-4" aria-hidden="true" /> {t("common.edit")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onCopy({ dim: "port", id: p.id })}>
                            <Copy className="size-4" aria-hidden="true" /> {t("shell.copyAccess")}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(p)}>
                            <Trash2 className="size-4" aria-hidden="true" /> {t("common.delete")}
                          </DropdownMenuItem>
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
              {editing ? t("ports.dialog.edit", { port: editing.port }) : t("ports.dialog.create")}
            </DialogTitle>
            <DialogDescription>{t("ports.dialog.desc")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="space-y-5" noValidate>
            <Field
              label={t("ports.field.port")}
              htmlFor="port-input"
              hint={t("ports.field.rangeHint", { min: range.min, max: range.max })}
            >
              <Input
                id="port-input"
                name="port"
                type="number"
                inputMode="numeric"
                min={range.min}
                max={range.max}
                step={1}
                required
                value={form.port}
                onChange={(e) => setForm({ ...form, port: e.target.value })}
                placeholder={String(range.min + 20)}
                aria-invalid={!!error}
                aria-describedby={error ? "port-error" : undefined}
              />
            </Field>
            <Field label={t("common.remark")} htmlFor="port-remark">
              <Input
                id="port-remark"
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
                placeholder={t("ports.field.remarkPlaceholder")}
              />
            </Field>
            {error ? (
              <p id="port-error" className="text-xs text-destructive" role="alert">
                {error}
              </p>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-16 text-center">
                  {busy ? t("common.saving") : t("common.save")}
                </span>
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!created} onOpenChange={(v) => !v && setCreated(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("ports.created.title")}</DialogTitle>
            <DialogDescription>{t("ports.created.desc", { port: created?.port ?? "" })}</DialogDescription>
          </DialogHeader>
          {created ? (
            <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
              <div className="flex items-center justify-between gap-4 text-sm">
                <span className="text-muted-foreground">{t("common.username")}</span>
                <code className="font-mono">{created.account.username}</code>
              </div>
              <div className="flex items-center justify-between gap-4 text-sm">
                <span className="text-muted-foreground">{t("common.password")}</span>
                <code className="font-mono">{created.account.password}</code>
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreated(null)}>
              {t("common.gotIt")}
            </Button>
            <Button onClick={copyAccount}>
              {copied ? <Check className="size-4" aria-hidden="true" /> : <Copy className="size-4" aria-hidden="true" />}
              {copied ? t("common.copied") : t("ports.created.copyAll")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleting} onOpenChange={(v) => !v && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("ports.delete.title", { port: deleting?.port ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("ports.delete.desc")}</AlertDialogDescription>
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
            <AlertDialogDescription>{t("ports.bulkDelete.desc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => runBulk(bulkAction ?? "delete")} disabled={bulkBusy}>
              {bulkBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
