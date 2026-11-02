import * as React from "react";
import { useI18n } from "@/i18n";

/** 登录页也提供语言切换（未登录时没有侧栏）。 */
export function AuthLanguageToggle() {
  const { lang, setLang, t } = useI18n();
  return (
    <div
      role="group"
      aria-label={t("shell.language")}
      className="inline-flex items-center rounded-md border p-0.5 text-xs"
    >
      {(["zh", "en"] as const).map((l) => (
        <button
          key={l}
          type="button"
          lang={l === "zh" ? "zh-CN" : "en"}
          aria-pressed={lang === l}
          onClick={() => setLang(l)}
          className={
            "rounded px-2 py-0.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 " +
            (lang === l
              ? "bg-foreground font-medium text-background"
              : "text-muted-foreground hover:text-foreground")
          }
        >
          {t(l === "zh" ? "shell.lang.zh" : "shell.lang.en")}
        </button>
      ))}
    </div>
  );
}
