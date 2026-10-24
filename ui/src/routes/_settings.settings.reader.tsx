import { createFileRoute } from "@tanstack/react-router";
import { DocumentReaderSettings } from "@/components/document-reader-settings";
import { SettingsPageHeader } from "@/components/settings-layout";
import { useAppLocale } from "@/lib/locale";

export const Route = createFileRoute("/_settings/settings/reader")({
  component: ReaderSettings,
});

function ReaderSettings() {
  const { t } = useAppLocale();
  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.item.reader")}
        description={t("readerSettings.description")}
      />
      <DocumentReaderSettings />
    </div>
  );
}
