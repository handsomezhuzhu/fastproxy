import * as React from "react";
import { api, type Data, type Settings } from "../api";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  Switch,
} from "../components/ui";
import { toast } from "../components/Toast";

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

  async function saveSettings() {
    setSaving(true);
    try {
      await api.saveSettings({
        ...form,
        log_buffer: Number(form.log_buffer),
        dial_timeout_sec: Number(form.dial_timeout_sec),
        handshake_timeout_sec: Number(form.handshake_timeout_sec),
      });
      toast("系统设置已保存");
      await reload();
    } catch (e: any) {
      toast(e?.message || "保存失败", "error");
    } finally {
      setSaving(false);
    }
  }

  const [oldPwd, setOldPwd] = React.useState("");
  const [newPwd, setNewPwd] = React.useState("");
  const [newPwd2, setNewPwd2] = React.useState("");
  const [pwdBusy, setPwdBusy] = React.useState(false);

  async function changePassword(e: React.FormEvent) {
    e.preventDefault();
    if (newPwd !== newPwd2) {
      toast("两次输入的新密码不一致", "error");
      return;
    }
    setPwdBusy(true);
    try {
      await api.changePassword(oldPwd, newPwd);
      toast("密码已修改，请重新登录");
      onLoggedOut();
    } catch (err: any) {
      toast(err?.message || "修改失败", "error");
    } finally {
      setPwdBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold">系统设置</h1>
        <p className="text-xs text-muted-foreground">全局配置入口：日志、超时、直连策略与管理密码</p>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>转发与日志</CardTitle>
            <CardDescription>修改后新连接立即生效，无需重启</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <Label className="text-foreground">记录连接日志</Label>
                <p className="text-[11px] text-muted-foreground">关闭后仅记录失败连接</p>
              </div>
              <Switch checked={form.access_log} onCheckedChange={(v) => setForm({ ...form, access_log: v })} />
            </div>

            <div className="flex items-center justify-between">
              <div>
                <Label className="text-foreground">允许直连端口</Label>
                <p className="text-[11px] text-muted-foreground">端口未绑定上游时直接从本机出口</p>
              </div>
              <Switch
                checked={form.allow_direct}
                onCheckedChange={(v) => setForm({ ...form, allow_direct: v })}
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>日志级别</Label>
                <Select value={form.log_level} onChange={(e) => setForm({ ...form, log_level: e.target.value })}>
                  <option value="debug">debug</option>
                  <option value="info">info</option>
                  <option value="warn">warn</option>
                  <option value="error">error</option>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label>内存日志条数</Label>
                <Input
                  type="number"
                  min={100}
                  max={10000}
                  value={form.log_buffer}
                  onChange={(e) => setForm({ ...form, log_buffer: Number(e.target.value) })}
                />
              </div>
              <div className="space-y-1.5">
                <Label>连接超时（秒）</Label>
                <Input
                  type="number"
                  min={3}
                  max={120}
                  value={form.dial_timeout_sec}
                  onChange={(e) => setForm({ ...form, dial_timeout_sec: Number(e.target.value) })}
                />
              </div>
              <div className="space-y-1.5">
                <Label>握手超时（秒）</Label>
                <Input
                  type="number"
                  min={10}
                  max={300}
                  value={form.handshake_timeout_sec}
                  onChange={(e) => setForm({ ...form, handshake_timeout_sec: Number(e.target.value) })}
                />
              </div>
            </div>

            <Button onClick={saveSettings} disabled={saving} className="w-full">
              {saving ? "保存中…" : "保存系统设置"}
            </Button>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>修改管理密码</CardTitle>
              <CardDescription>修改后所有会话失效，需要重新登录</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={changePassword} className="space-y-3">
                <div className="space-y-1.5">
                  <Label>原密码</Label>
                  <Input type="password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label>新密码（至少 6 位）</Label>
                  <Input type="password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label>确认新密码</Label>
                  <Input type="password" value={newPwd2} onChange={(e) => setNewPwd2(e.target.value)} />
                </div>
                <Button type="submit" variant="secondary" className="w-full" disabled={pwdBusy || !newPwd}>
                  {pwdBusy ? "处理中…" : "修改密码"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>关于</CardTitle>
              <CardDescription>FastProxy · 多端口多账户转发中转</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 text-xs text-muted-foreground">
              <div className="flex items-center justify-between">
                <span>版本</span>
                <Badge variant="outline">v{data.version}</Badge>
              </div>
              <div className="flex items-center justify-between">
                <span>面板地址</span>
                <span className="font-mono">{window.location.host}</span>
              </div>
              <div className="flex items-center justify-between">
                <span>入站协议</span>
                <span>HTTP / SOCKS5（同端口自动识别）</span>
              </div>
              <div className="flex items-center justify-between">
                <span>配置文件</span>
                <span className="font-mono">/data/fastproxy.json</span>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
