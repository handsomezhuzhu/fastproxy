import * as React from "react";
import { Loader2 } from "lucide-react";
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

export function SettingsPage({
  data,
  reload,
  onLoggedOut,
}: {
  data: Data;
  reload: () => Promise<void>;
  onLoggedOut: () => void;
}) {
  const [form, setForm] = React.useState<Settings>(data.settings);
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => setForm(data.settings), [data.settings]);

  async function saveSettings(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await api.saveSettings({
        ...form,
        log_buffer: Number(form.log_buffer),
        log_retention_days: Number(form.log_retention_days),
        dial_timeout_sec: Number(form.dial_timeout_sec),
        handshake_timeout_sec: Number(form.handshake_timeout_sec),
        probe_interval_sec: Number(form.probe_interval_sec),
        probe_timeout_sec: Number(form.probe_timeout_sec),
        probe_concurrency: Number(form.probe_concurrency),
      });
      toast.success("系统设置已保存");
      await reload();
    } catch (err: any) {
      toast.error(err?.message || "保存失败");
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
      toast.error("两次输入的新密码不一致");
      return;
    }
    setPwdBusy(true);
    try {
      await api.changePassword(oldPassword, newPassword);
      toast.success("密码已修改，请重新登录");
      onLoggedOut();
    } catch (err: any) {
      toast.error(err?.message || "修改失败");
    } finally {
      setPwdBusy(false);
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="系统设置" description="全局配置：日志、超时、直连策略、探测与管理密码。" />

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>转发与日志</CardTitle>
            <CardDescription>保存后新连接立即生效，无需重启服务。</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={saveSettings} className="space-y-5">
              <div className="flex items-center justify-between gap-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium">记录连接日志</p>
                  <p className="text-xs text-muted-foreground">关闭后仅记录失败连接。</p>
                </div>
                <Switch
                  checked={form.access_log}
                  onCheckedChange={(v: boolean) => setForm({ ...form, access_log: v })}
                  aria-label="记录连接日志"
                />
              </div>

              <div className="flex items-center justify-between gap-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium">允许直连端口</p>
                  <p className="text-xs text-muted-foreground">端口未绑定上游时从本机直接出口。</p>
                </div>
                <Switch
                  checked={form.allow_direct}
                  onCheckedChange={(v: boolean) => setForm({ ...form, allow_direct: v })}
                  aria-label="允许直连端口"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <Field label="日志级别" htmlFor="set-level">
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
                <Field label="日志保留（天）" htmlFor="set-retention">
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
                <Field label="连接超时（秒）" htmlFor="set-dial">
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
                <Field label="握手超时（秒）" htmlFor="set-handshake">
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

              <div className="space-y-4 border-t pt-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">自动探测</p>
                    <p className="text-xs text-muted-foreground">
                      定时检测上游健康状态、延迟与出口 IP，并更新本机出口 IP。
                    </p>
                  </div>
                  <Switch
                    checked={form.probe_enabled}
                    onCheckedChange={(v: boolean) => setForm({ ...form, probe_enabled: v })}
                    aria-label="自动探测上游"
                  />
                </div>
                <div className="grid grid-cols-3 gap-3">
                  <Field label="探测间隔（秒）" htmlFor="set-probe-interval">
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
                  <Field label="探测超时（秒）" htmlFor="set-probe-timeout">
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
                  <Field label="检测并发数" htmlFor="set-probe-concurrency">
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
                      label="探测目标（域名）"
                      htmlFor="set-probe-target"
                      hint="通过上游请求该域名获取出口 IP，依次尝试 HTTP 80 / HTTPS 443。"
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
                {saving ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                <span className="inline-block min-w-24 text-center">{saving ? "保存中…" : "保存系统设置"}</span>
              </Button>
            </form>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>修改管理密码</CardTitle>
              <CardDescription>修改后所有会话失效，需要重新登录。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={changePassword} className="space-y-3">
                <Field label="原密码" htmlFor="pwd-old">
                  <Input
                    id="pwd-old"
                    type="password"
                    autoComplete="current-password"
                    value={oldPassword}
                    onChange={(e) => setOldPassword(e.target.value)}
                  />
                </Field>
                <Field label="新密码（至少 6 位）" htmlFor="pwd-new">
                  <Input
                    id="pwd-new"
                    type="password"
                    autoComplete="new-password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                  />
                </Field>
                <Field label="确认新密码" htmlFor="pwd-confirm">
                  <Input
                    id="pwd-confirm"
                    type="password"
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                  />
                </Field>
                <Button type="submit" variant="secondary" className="w-full" disabled={pwdBusy || !newPassword}>
                  {pwdBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                  <span className="inline-block min-w-16 text-center">{pwdBusy ? "处理中…" : "修改密码"}</span>
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
