import * as React from "react";
import { Zap } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { AuthLanguageToggle } from "@/components/LanguageSwitch";
import { LoginTopology } from "@/components/LoginTopology";
import { ThemeSwitch } from "@/components/ThemeSwitch";
import { useI18n } from "@/i18n";

/**
 * 登录页（shadcn login-02 双栏布局：左侧表单，右侧路由示意图，窄屏只留表单）。
 * 登录失败后端返回 429 `{error, retry_after_sec}`，提示尝试过多，按 X 秒后重试；
 * 按钮置灰并显示倒计时，到期自动恢复（不刷新页面）。
 * 如果响应未带 retry_after_sec，则按 30 秒兜底，避免按钮永久禁用。
 */
export function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useI18n();
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [capsLock, setCapsLock] = React.useState(false);
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

  function trackCapsLock(e: React.KeyboardEvent<HTMLInputElement>) {
    setCapsLock(e.getModifierState?.("CapsLock") ?? false);
  }

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
    <div className="grid min-h-svh lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
      <div className="flex flex-col gap-4 p-6 md:p-10">
        <header className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 font-medium" aria-hidden="true">
            <div className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
              <Zap className="size-4" />
            </div>
            FastProxy
          </div>
          <div className="flex items-center gap-2">
            <ThemeSwitch />
            <AuthLanguageToggle />
          </div>
        </header>

        <main className="flex flex-1 items-center justify-center py-10">
          <div className="flex w-full max-w-sm flex-col gap-8">
            <div className="flex flex-col gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">{t("login.title")}</h1>
              <p className="text-sm text-muted-foreground">{t("login.description")}</p>
            </div>

            <form onSubmit={submit} noValidate>
              <FieldGroup>
                <Field data-invalid={error ? true : undefined}>
                  <FieldLabel htmlFor="admin-password" className="text-sm">
                    {t("login.password")}
                  </FieldLabel>
                  <Input
                    id="admin-password"
                    name="password"
                    type="password"
                    autoComplete="current-password"
                    autoFocus
                    required
                    className="h-10"
                    placeholder={t("login.placeholder")}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    onKeyDown={trackCapsLock}
                    onKeyUp={trackCapsLock}
                    onBlur={() => setCapsLock(false)}
                    spellCheck={false}
                    aria-invalid={!!error}
                    aria-describedby={error ? "login-error" : capsLock ? "login-caps" : undefined}
                  />
                  {error ? (
                    <FieldError id="login-error" role="alert">
                      {error}
                    </FieldError>
                  ) : capsLock ? (
                    <FieldDescription id="login-caps">{t("login.capsLock")}</FieldDescription>
                  ) : null}
                </Field>
                <Field>
                  <Button
                    type="submit"
                    size="lg"
                    className="h-10 w-full"
                    disabled={busy || locked || !password}
                  >
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
          </div>
        </main>
      </div>

      <div className="relative hidden overflow-hidden border-l bg-muted lg:flex lg:flex-col lg:justify-between lg:p-10">
        <div
          className="pointer-events-none absolute inset-0 opacity-70 [background-image:radial-gradient(var(--border)_1px,transparent_1px)] [background-size:18px_18px] [mask-image:radial-gradient(ellipse_at_center,black_40%,transparent_80%)]"
          aria-hidden="true"
        />
        <div className="relative flex flex-1 items-center justify-center">
          <LoginTopology className="w-full max-w-2xl" />
        </div>
        <div className="relative flex max-w-md flex-col gap-2">
          <p className="text-xl font-semibold tracking-tight text-balance">{t("login.heroTitle")}</p>
          <p className="text-sm leading-relaxed text-balance text-muted-foreground">{t("login.heroBody")}</p>
        </div>
      </div>
    </div>
  );
}
