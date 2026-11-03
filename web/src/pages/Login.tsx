import * as React from "react";
import { Loader2 } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthLanguageToggle } from "@/components/LanguageSwitch";
import { ThemeSwitch } from "@/components/ThemeSwitch";
import { useI18n } from "@/i18n";

/**
 * 登录页。
 * 登录失败后端返回 429 `{error, retry_after_sec}`，提示尝试过多，按 X 秒后重试；
 * 按钮置灰并显示倒计时，到期自动恢复（不刷新页面）。
 * 如果响应未带 retry_after_sec，则按 30 秒兜底，避免按钮永久禁用。
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
    // 倒计时结束，清除锁定提示。
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
      }
    } finally {
      // 成功时会立刻卸载，这里兜底避免异常路径一直转圈。
      setBusy(false);
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4 py-10">
      <div className="absolute right-4 top-4 flex items-center gap-2">
        <ThemeSwitch />
        <AuthLanguageToggle />
      </div>

      <div className="w-full max-w-[400px]">
        <div className="rounded-xl border bg-card px-6 py-8 shadow-sm sm:px-8">
          <h1 className="text-center text-[28px] font-semibold leading-9 tracking-tight">
            {t("login.title")}
          </h1>

          <form onSubmit={submit} className="mt-8 space-y-4">
            <div className="space-y-2">
              <Label
                htmlFor="admin-password"
                className="text-[13px] font-medium uppercase tracking-[0.05em] text-muted-foreground"
              >
                {t("login.password")}
              </Label>
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
                className="h-11 rounded-lg bg-background px-3"
              />
            </div>
            {error ? (
              <p id="login-error" role="alert" className="text-sm text-destructive">
                {error}
              </p>
            ) : null}
            <Button className="h-11 w-full" type="submit" disabled={busy || locked || !password}>
              {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              {locked
                ? t("login.lockedShort", { sec: remaining })
                : busy
                  ? t("login.submitting")
                  : t("login.submit")}
            </Button>
          </form>
        </div>
      </div>
    </div>
  );
}
