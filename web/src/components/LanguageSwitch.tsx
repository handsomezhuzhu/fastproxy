import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useI18n } from "@/i18n";

/** Language toggle, shared by the login page and the app header. */
export function AuthLanguageToggle() {
  const { lang, setLang, t } = useI18n();
  return (
    <ToggleGroup
      variant="outline"
      size="sm"
      spacing={0}
      aria-label={t("shell.language")}
      value={[lang]}
      onValueChange={(v) => {
        const next = v[0] as "zh" | "en" | undefined;
        if (next) setLang(next);
      }}
    >
      {(["zh", "en"] as const).map((l) => (
        <ToggleGroupItem key={l} value={l} lang={l === "zh" ? "zh-CN" : "en"} className="px-2.5">
          {t(l === "zh" ? "shell.lang.zh" : "shell.lang.en")}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
