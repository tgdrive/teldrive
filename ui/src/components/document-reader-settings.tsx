import { Button, Label, ListBox, Select, Spinner, Switch } from "@heroui/react";
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
        <Select
          aria-label={t("documents.cacheLimit")}
          selectedKey={String(settings.cacheLimitMiB)}
          isDisabled={busy || !settings.cacheEnabled}
          onSelectionChange={(key) => {
            if (key !== null)
              void perform(() => configureDocumentCache({ cacheLimitMiB: Number(key) }));
          }}
        >
          <Select.Trigger aria-label={t("documents.cacheLimit")}>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              {[32, 64, 128, 256, 512, 1024].map((size) => (
                <ListBox.Item
                  key={size}
                  id={String(size)}
                  textValue={formatBytes(size * 1024 * 1024)}
                >
                  {formatBytes(size * 1024 * 1024)}
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
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
