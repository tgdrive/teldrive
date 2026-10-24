import { Button, Label, Spinner, Switch } from "@heroui/react";
import { useEffect, useState } from "react";
import {
  clearDocumentCache,
  configureDocumentCache,
  DOCUMENT_CACHE_CHANGED,
  documentCacheUsage,
} from "@/features/files/document-cache";
import { useDocumentSettings } from "@/lib/document-settings";
import { useFormatBytes } from "@/lib/format";
import { useAppLocale } from "@/lib/locale";
import { PdfBrightnessSlider } from "./pdf-brightness-slider";
import { SettingsRow, SettingsSection } from "./settings-layout";

export function DocumentReaderSettings() {
  const { t } = useAppLocale();
  const formatBytes = useFormatBytes();
  const settings = useDocumentSettings();
  const [usage, setUsage] = useState<{ files: number; bytes: number }>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const refresh = () => {
      void documentCacheUsage()
        .then((value) => {
          if (active) setUsage(value);
        })
        .catch(() => {
          if (active) setError(t("documents.storageUnavailable"));
        });
    };
    refresh();
    window.addEventListener(DOCUMENT_CACHE_CHANGED, refresh);
    window.addEventListener("focus", refresh);
    return () => {
      active = false;
      window.removeEventListener(DOCUMENT_CACHE_CHANGED, refresh);
      window.removeEventListener("focus", refresh);
    };
  }, [t]);

  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch {
      setError(t("documents.storageUnavailable"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <SettingsSection title={t("documents.title")} description={t("documents.description")}>
      <SettingsRow label={t("pdf.brightnessAria")} description={t("documents.brightnessHint")}>
        <PdfBrightnessSlider />
      </SettingsRow>
      <SettingsRow label={t("documents.cacheLabel")} description={t("documents.cacheHint")}>
        <Switch
          aria-label={t("documents.cacheSwitch")}
          isSelected={settings.cacheEnabled}
          isDisabled={busy}
          onChange={(cacheEnabled) => void perform(() => configureDocumentCache({ cacheEnabled }))}
        >
          <Switch.Content>
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
            <Label>{t("documents.cacheSwitch")}</Label>
          </Switch.Content>
        </Switch>
      </SettingsRow>
      <SettingsRow label={t("documents.cacheLimit")} description={t("documents.cacheLimitHint")}>
        <select
          aria-label={t("documents.cacheLimit")}
          value={settings.cacheLimitMiB}
          disabled={busy || !settings.cacheEnabled}
          onChange={(event) =>
            void perform(() =>
              configureDocumentCache({ cacheLimitMiB: Number(event.target.value) }),
            )
          }
          className="h-10 w-full rounded-lg border border-field-border bg-field-background px-3 text-sm text-foreground disabled:opacity-50"
        >
          {[32, 64, 128, 256, 512, 1024].map((size) => (
            <option key={size} value={size}>
              {formatBytes(size * 1024 * 1024)}
            </option>
          ))}
        </select>
      </SettingsRow>
      <SettingsRow label={t("documents.cacheUsage")} description={t("documents.cachePrivacy")}>
        <div className="flex items-center justify-between gap-3">
          <output aria-label={t("documents.cacheUsage")} className="text-xs text-muted">
            {usage
              ? t("documents.cacheUsageValue", {
                  count: usage.files,
                  size: formatBytes(usage.bytes),
                })
              : "—"}
          </output>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={busy}
            onPress={() => void perform(clearDocumentCache)}
          >
            {busy ? <Spinner size="sm" /> : t("documents.clearCache")}
          </Button>
        </div>
        {error ? (
          <p role="alert" className="mt-2 text-xs text-danger">
            {error}
          </p>
        ) : null}
      </SettingsRow>
    </SettingsSection>
  );
}
