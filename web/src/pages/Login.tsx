import * as React from "react";
import { Loader2, Zap } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthLanguageToggle } from "@/components/LanguageSwitch";
import { useI18n } from "@/i18n";

/**
 * 登录页。
 * 连续失败后后端返回 429 `{error, retry_after_sec}`：显示“尝试过多，请 X 秒后重试”，
 * 按钮禁用并本地倒计时，结束自动恢复（不刷新页面）。
 * 后端未返回 retry_after_sec 时按 30 秒兜底，避免按钮永远禁用。
 */
export function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useI18n();
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [lockedUntil, setLockedUntil] = React.useState(0);
  const [now, setNow] = React.useState(() => Date.now());

  React.useEffect(() => {
    if (!lockedUntil) return;
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [lockedUntil]);

  const remaining = lockedUntil ? Math.max(0, Math.ceil((lockedUntil - now) / 1000)) : 0;
  const locked = remaining > 0;

  React.useEffect(() => {
    if (!lockedUntil || remaining > 0) return;
    // 倒计时结束：解除锁定并清掉锁定提示。
    setLockedUntil(0);
    setError("");
  }, [lockedUntil, remaining]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (locked || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.login(password);
      onSuccess();
    } catch (err: any) {
      if (err?.status === 429) {
        const sec = Math.min(Math.max(1, Number(err?.retryAfterSec) || 30), 24 * 3600);
        const until = Date.now() + sec * 1000;
        setNow(Date.now());
        setLockedUntil(until);
        setError(t("login.locked", { sec }));
      } else {
        setError(err?.message || t("login.failed"));
        setBusy(false);
        // 密码错误后也刷新一次 session，确保按钮状态始终可重试。
      }
    } finally {
      // 成功时组件很快被卸载，这里兜底避免异常路径一直转圈。
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-6">
      <div className="w-full max-w-sm space-y-3">
        <div className="flex justify-end">
          <AuthLanguageToggle />
        </div>
        <Card className="rounded-xl border shadow-sm">
          <CardContent className="p-6">
            <div className="mb-6 flex flex-col items-center gap-3 text-center">
              <div className="flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
                <Zap className="size-6" aria-hidden="true" />
              </div>
              <div>
                <h1 className="text-lg font-semibold tracking-tight">{t("login.title")}</h1>
                <p className="mt-1 text-xs text-muted-foreground">{t("login.subtitle")}</p>
              </div>
            </div>
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="admin-password">{t("login.password")}</Label>
                <Input
                  id="admin-password"
                  name="password"
                  type="password"
                  autoComplete="current-password"
                  autoFocus
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t("login.placeholder")}
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
              <Button className="w-full" type="submit" disabled={busy || locked || !password}>
                {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
                {locked
                  ? t("login.lockedShort", { sec: remaining })
                  : busy
                    ? t("login.submitting")
                    : t("login.submit")}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
