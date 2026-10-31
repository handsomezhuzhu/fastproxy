import * as React from "react";
import { Zap } from "lucide-react";
import { api } from "../api";
import { Button, Card, CardContent, Input, Label } from "../components/ui";

export function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.login(password);
      onSuccess();
    } catch (err: any) {
      setError(err?.message || "登录失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardContent className="pt-8">
          <div className="mb-6 flex flex-col items-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary/15">
              <Zap className="h-6 w-6 text-primary" />
            </div>
            <div className="text-center">
              <h1 className="text-lg font-semibold">FastProxy 控制台</h1>
              <p className="mt-1 text-xs text-muted-foreground">
                多端口 · 多账户 · 上游绑定 转发中转
              </p>
            </div>
          </div>
          <form onSubmit={submit} className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="password">管理密码</Label>
              <Input
                id="password"
                type="password"
                autoFocus
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入管理密码"
              />
            </div>
            {error ? <p className="text-xs text-destructive">{error}</p> : null}
            <Button className="w-full" type="submit" disabled={busy || !password}>
              {busy ? "登录中…" : "登 录"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
