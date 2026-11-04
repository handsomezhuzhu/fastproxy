import { Monitor, Moon, Sun } from "lucide-react";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
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
    <ToggleGroup
      variant="outline"
      size="sm"
      spacing={0}
      aria-label={t("shell.theme.label")}
      value={[mode]}
      onValueChange={(v) => {
        const next = v[0] as ThemeMode | undefined;
        if (next) setMode(next);
      }}
    >
      {options.map(({ id, label, icon: Icon }) => (
        <ToggleGroupItem key={id} value={id} aria-label={label} title={label}>
          <Icon />
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
