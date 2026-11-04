import * as React from "react";
import { Zap } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { AuthLanguageToggle } from "@/components/LanguageSwitch";
import { ThemeSwitch } from "@/components/ThemeSwitch";
import { useI18n } from "@/i18n";

/**
 * 登录页（shadcn login-01 布局）。
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
        setNow(Date.now());
        setLockedUntil(Date.now() + sec * 1000);
        setError(t("login.locked", { sec }));
      } else {
        setError(err?.message || t("login.failed"));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-svh flex-col bg-muted">
      <header className="flex items-center justify-end gap-2 p-4">
        <ThemeSwitch />
        <AuthLanguageToggle />
      </header>

      <main className="flex flex-1 flex-col items-center justify-center gap-6 px-6 pb-16">
        <div className="flex items-center gap-2 font-medium" aria-hidden="true">
          <div className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Zap className="size-4" />
          </div>
          FastProxy
        </div>

        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <CardTitle className="text-xl">{t("login.title")}</CardTitle>
            <CardDescription>{t("login.placeholder")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit}>
              <FieldGroup>
                <Field data-invalid={error ? true : undefined}>
                  <FieldLabel htmlFor="admin-password">{t("login.password")}</FieldLabel>
                  <Input
                    id="admin-password"
                    name="password"
                    type="password"
                    autoComplete="current-password"
                    autoFocus
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    spellCheck={false}
                    aria-invalid={!!error}
                    aria-describedby={error ? "login-error" : undefined}
                  />
                  {error ? (
                    <FieldError id="login-error" role="alert">
                      {error}
                    </FieldError>
                  ) : null}
                </Field>
                <Field>
                  <Button type="submit" className="w-full" disabled={busy || locked || !password}>
                    {busy ? <Spinner data-icon="inline-start" /> : null}
                    {locked
                      ? t("login.lockedShort", { sec: remaining })
                      : busy
                        ? t("login.submitting")
                        : t("login.submit")}
                  </Button>
                </Field>
              </FieldGroup>
            </form>
          </CardContent>
        </Card>
      </main>
    </div>
  );
}
