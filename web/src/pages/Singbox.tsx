import { PageHeader } from "@/components/page";
import { SingboxCard } from "@/components/SingboxCard";
import { useI18n } from "@/i18n";

export function SingboxPage() {
  const { t } = useI18n();
  return (
    <div className="space-y-5">
      <PageHeader title={t("dashboard.singbox.title")} description={t("dashboard.singbox.pageDesc")} />
      <SingboxCard titleKey="dashboard.singbox.statusTitle" />
    </div>
  );
}
