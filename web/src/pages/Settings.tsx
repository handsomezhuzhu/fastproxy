import * as React from "react";
import { Spinner } from "@/components/ui/spinner";
import { toast } from "sonner";
import { api, type Data, type Settings } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Field, FormSelect, PageHeader } from "@/components/page";
import { useI18n } from "@/i18n";

/** v2：Settings 不再有 allow_direct，发送前剔除该字段，避免旧后端/新后端字段冲突。 */
function toPayload(form: Settings): Settings {
  const { access_log, log_level, log_buffer, log_retention_days, dial_timeout_sec, handshake_timeout_sec, probe_enabled, probe_interval_sec, probe_timeout_sec, probe_concurrency, probe_target } = form;
  return {
    access_log,
    log_level,
    log_buffer: Number(log_buffer),
    log_retention_days: Number(log_retention_days),
    dial_timeout_sec: Number(dial_timeout_sec),
    handshake_timeout_sec: Number(handshake_timeout_sec),
    probe_enabled,
    probe_interval_sec: Number(probe_interval_sec),
    probe_timeout_sec: Number(probe_timeout_sec),
    // 并发数限制 1-32，与检测请求的并发上限一致。
    probe_concurrency: Math.min(32, Math.max(1, Math.floor(Number(probe_concurrency) || 1))),
    probe_target: probe_target.trim(),
  };
}

export function SettingsPage({
  data,
  reload,
  onLoggedOut,
}: {
  data: Data;
  reload: () => Promise<void>;
  onLoggedOut: () => void;
}) {
  const { t } = useI18n();
  const [form, setForm] = React.useState<Settings>(data.settings);
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => setForm(data.settings), [data.settings]);

  async function saveSettings(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await api.saveSettings(toPayload(form));
      toast.success(t("settings.saved"));
      await reload();
    } catch (err: any) {
      toast.error(err?.message || t("common.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  const [oldPassword, setOldPassword] = React.useState("");
  const [newPassword, setNewPassword] = React.useState("");
  const [confirmPassword, setConfirmPassword] = React.useState("");
  const [pwdBusy, setPwdBusy] = React.useState(false);

  async function changePassword(e: React.FormEvent) {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      toast.error(t("settings.pwd.mismatch"));
      return;
    }
    setPwdBusy(true);
    try {
      await api.changePassword(oldPassword, newPassword);
      toast.success(t("settings.pwd.done"));
      onLoggedOut();
    } catch (err: any) {
      toast.error(err?.message || t("settings.pwd.failed"));
    } finally {
      setPwdBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader title={t("settings.title")} description={t("settings.description")} />

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t("settings.fwd.title")}</CardTitle>
            <CardDescription>{t("settings.fwd.desc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={saveSettings} className="flex flex-col gap-5">
              <div className="flex items-center justify-between gap-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{t("settings.accessLog")}</p>
                  <p className="text-xs text-muted-foreground">{t("settings.accessLogHint")}</p>
                </div>
                <Switch
                  checked={form.access_log}
                  onCheckedChange={(v: boolean) => setForm({ ...form, access_log: v })}
                  aria-label={t("settings.accessLog")}
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <Field label={t("settings.logLevel")} htmlFor="set-level">
                  <FormSelect
                    id="set-level"
                    value={form.log_level}
                    onValueChange={(v) => setForm({ ...form, log_level: v })}
                    items={[
                      { value: "debug", label: "debug" },
                      { value: "info", label: "info" },
                      { value: "warn", label: "warn" },
                      { value: "error", label: "error" },
                    ]}
                  />
                </Field>
                <Field label={t("settings.logRetention")} htmlFor="set-retention">
                  <Input
                    id="set-retention"
                    type="number"
                    inputMode="numeric"
                    min={1}
                    max={90}
                    value={form.log_retention_days}
                    onChange={(e) => setForm({ ...form, log_retention_days: Number(e.target.value) })}
                  />
                </Field>
                <Field label={t("settings.dialTimeout")} htmlFor="set-dial">
                  <Input
                    id="set-dial"
                    type="number"
                    inputMode="numeric"
                    min={3}
                    max={120}
                    value={form.dial_timeout_sec}
                    onChange={(e) => setForm({ ...form, dial_timeout_sec: Number(e.target.value) })}
                  />
                </Field>
                <Field label={t("settings.handshakeTimeout")} htmlFor="set-handshake">
                  <Input
                    id="set-handshake"
                    type="number"
                    inputMode="numeric"
                    min={10}
                    max={300}
                    value={form.handshake_timeout_sec}
                    onChange={(e) => setForm({ ...form, handshake_timeout_sec: Number(e.target.value) })}
                  />
                </Field>
              </div>

              <div className="flex flex-col gap-4 border-t pt-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{t("settings.probe")}</p>
                    <p className="text-xs text-muted-foreground">{t("settings.probeHint")}</p>
                  </div>
                  <Switch
                    checked={form.probe_enabled}
                    onCheckedChange={(v: boolean) => setForm({ ...form, probe_enabled: v })}
                    aria-label={t("settings.probe")}
                  />
                </div>
                <div className="grid grid-cols-3 gap-4">
                  <Field label={t("settings.probeInterval")} htmlFor="set-probe-interval">
                    <Input
                      id="set-probe-interval"
                      type="number"
                      inputMode="numeric"
                      min={30}
                      max={3600}
                      value={form.probe_interval_sec}
                      onChange={(e) => setForm({ ...form, probe_interval_sec: Number(e.target.value) })}
                    />
                  </Field>
                  <Field label={t("settings.probeTimeout")} htmlFor="set-probe-timeout">
                    <Input
                      id="set-probe-timeout"
                      type="number"
                      inputMode="numeric"
                      min={3}
                      max={30}
                      value={form.probe_timeout_sec}
                      onChange={(e) => setForm({ ...form, probe_timeout_sec: Number(e.target.value) })}
                    />
                  </Field>
                  <Field label={t("settings.probeConcurrency")} htmlFor="set-probe-concurrency">
                    <Input
                      id="set-probe-concurrency"
                      type="number"
                      inputMode="numeric"
                      min={1}
                      max={32}
                      value={form.probe_concurrency}
                      onChange={(e) => setForm({ ...form, probe_concurrency: Number(e.target.value) })}
                    />
                  </Field>
                  <div className="col-span-3">
                    <Field
                      label={t("settings.probeTarget")}
                      htmlFor="set-probe-target"
                      hint={t("settings.probeTargetHint")}
                    >
                      <Input
                        id="set-probe-target"
                        className="font-mono"
                        value={form.probe_target}
                        onChange={(e) => setForm({ ...form, probe_target: e.target.value })}
                        placeholder="api.ipify.org"
                        spellCheck={false}
                      />
                    </Field>
                  </div>
                </div>
              </div>

              <Button type="submit" className="w-full" disabled={saving}>
                {saving ? <Spinner data-icon="inline-start" /> : null}
                <span className="inline-block min-w-24 text-center">
                  {saving ? t("common.saving") : t("settings.save")}
                </span>
              </Button>
            </form>
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.pwd.title")}</CardTitle>
              <CardDescription>{t("settings.pwd.desc")}</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={changePassword} className="flex flex-col gap-4">
                <Field label={t("settings.pwd.old")} htmlFor="pwd-old">
                  <Input
                    id="pwd-old"
                    type="password"
                    autoComplete="current-password"
                    value={oldPassword}
                    onChange={(e) => setOldPassword(e.target.value)}
                  />
                </Field>
                <Field label={t("settings.pwd.new")} htmlFor="pwd-new">
                  <Input
                    id="pwd-new"
                    type="password"
                    autoComplete="new-password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                  />
                </Field>
                <Field label={t("settings.pwd.confirm")} htmlFor="pwd-confirm">
                  <Input
                    id="pwd-confirm"
                    type="password"
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                  />
                </Field>
                <Button type="submit" variant="secondary" className="w-full" disabled={pwdBusy || !newPassword}>
                  {pwdBusy ? <Spinner data-icon="inline-start" /> : null}
                  <span className="inline-block min-w-16 text-center">
                    {pwdBusy ? t("settings.pwd.busy") : t("settings.pwd.submit")}
                  </span>
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
