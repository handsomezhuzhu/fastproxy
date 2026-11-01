import * as React from "react";
import { Loader2, Zap } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

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
      setError(err?.message || "登录失败，请重试");
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-6">
      <Card className="w-full max-w-sm rounded-xl border shadow-sm">
        <CardContent className="p-6">
          <div className="mb-6 flex flex-col items-center gap-3 text-center">
            <div className="flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
              <Zap className="size-6" aria-hidden="true" />
            </div>
            <div>
              <h1 className="text-lg font-semibold tracking-tight">FastProxy 控制台</h1>
              <p className="mt-1 text-xs text-muted-foreground">
                多端口 · 多账户 · 上游绑定 转发中转
              </p>
            </div>
          </div>
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="admin-password">管理密码</Label>
              <Input
                id="admin-password"
                name="password"
                type="password"
                autoComplete="current-password"
                autoFocus
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入管理密码…"
                spellCheck={false}
                aria-invalid={!!error}
                aria-describedby={error ? "login-error" : undefined}
              />
            </div>
            {error ? (
              <p id="login-error" role="alert" className="text-xs text-destructive">
                {error}
              </p>
            ) : null}
            <Button className="w-full" type="submit" disabled={busy || !password}>
              {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              {busy ? "登录中…" : "登录"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
