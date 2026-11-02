import { Monitor, Moon, Sun } from "lucide-react";
import { useI18n } from "@/i18n";
import { useTheme } from "@/lib/theme-context";
import type { ThemeMode } from "@/lib/theme";

/** Three-mode theme control: follow system / light / dark. */
export function ThemeSwitch() {
  const { t } = useI18n();
  const { mode, setMode } = useTheme();
  const options: { id: ThemeMode; label: string; icon: typeof Monitor }[] = [
    { id: "system", label: t("shell.theme.systemShort"), icon: Monitor },
    { id: "light", label: t("shell.theme.lightShort"), icon: Sun },
    { id: "dark", label: t("shell.theme.darkShort"), icon: Moon },
  ];
  return (
    <div
      role="group"
      aria-label={t("shell.theme.label")}
      className="inline-flex items-center rounded-md border p-0.5 text-xs"
    >
      {options.map(({ id, label, icon: Icon }) => (
        <button
          key={id}
          type="button"
          title={label}
          aria-label={label}
          aria-pressed={mode === id}
          onClick={() => setMode(id)}
          className={
            "rounded p-1 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 " +
            (mode === id
              ? "bg-foreground text-background"
              : "text-muted-foreground hover:text-foreground")
          }
        >
          <Icon className="size-3.5" aria-hidden="true" />
        </button>
      ))}
    </div>
  );
}
