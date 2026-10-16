import { Label, ListBox, NumberField, Select, Switch } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useRef, useState } from "react";
import type { UploadPreferences } from "@/api/types";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { useAppLocale } from "@/lib/locale";
import {
  MAX_PART_SIZE_MIB,
  normalizePartSizeMiB,
  usePreferences,
  useUpdatePreferences,
} from "@/lib/preferences";

export const Route = createFileRoute("/_settings/settings/uploads")({ component: UploadSettings });

function UploadSettings() {
  const uploads = usePreferences()?.uploads;
  const { mutate: updatePreferences } = useUpdatePreferences();
  const { t } = useAppLocale();
  if (!uploads) return null;

  return (
    <div className="space-y-6">
      <SettingsPageHeader title={t("uploads.title")} description={t("uploads.desc")} />
      <SettingsSection title={t("uploads.behavior")} description={t("uploads.behaviorDesc")}>
        <SettingsRow label={t("uploads.encryption")} description={t("uploads.encryptionDesc")}>
          <Switch
            aria-label={t("uploads.encryptLabel")}
            isSelected={uploads.encryption}
            onChange={(isSelected) => updatePreferences({ uploads: { encryption: isSelected } })}
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Label>{t("uploads.encryptLabel")}</Label>
            </Switch.Content>
          </Switch>
        </SettingsRow>
        <SettingsRow label={t("uploads.conflicts")} description={t("uploads.conflictsDesc")}>
          <Select
            aria-label={t("uploads.conflicts")}
            selectedKey={uploads.conflictPolicy}
            onSelectionChange={(key) =>
              updatePreferences({
                uploads: { conflictPolicy: String(key) as UploadPreferences["conflictPolicy"] },
              })
            }
          >
            <Select.Trigger>
              <Select.Value />
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                <ListBox.Item id="rename" textValue={t("uploads.optRename")}>
                  {t("uploads.optRename")}
                </ListBox.Item>
                <ListBox.Item id="replace" textValue={t("uploads.optReplace")}>
                  {t("uploads.optReplace")}
                </ListBox.Item>
                <ListBox.Item id="fail" textValue={t("uploads.optError")}>
                  {t("uploads.optError")}
                </ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
        </SettingsRow>
        <SettingsRow label={t("uploads.concurrency")} description={t("uploads.concurrencyDesc")}>
          <NumberField
            aria-label={t("uploads.concurrency")}
            value={uploads.concurrency}
            minValue={1}
            maxValue={12}
            onChange={(value) =>
              updatePreferences({
                uploads: { concurrency: Math.max(1, Math.min(12, value ?? 1)) },
              })
            }
          >
            <Label className="sr-only">{t("uploads.concurrency")}</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
          </NumberField>
        </SettingsRow>
        <SettingsRow label={t("uploads.partSize")} description={t("uploads.partSizeDesc")}>
          <PartSizeField preferredPartSize={uploads.preferredPartSize} />
        </SettingsRow>
      </SettingsSection>
    </div>
  );
}

function PartSizeField({ preferredPartSize }: { preferredPartSize: number }) {
  const { mutate: updatePreferences } = useUpdatePreferences();
  const [value, setValue] = useState(preferredPartSize / 1024 / 1024);
  const valueRef = useRef(value);
  const { t } = useAppLocale();

  const commit = () => {
    const normalized = normalizePartSizeMiB(valueRef.current);
    valueRef.current = normalized;
    setValue(normalized);
    updatePreferences({ uploads: { preferredPartSize: normalized * 1024 * 1024 } });
  };

  return (
    <NumberField
      aria-label={t("uploads.partSizeAria")}
      value={value}
      maxValue={MAX_PART_SIZE_MIB}
      onChange={(next) => {
        valueRef.current = next ?? 512;
        setValue(valueRef.current);
      }}
      onBlur={commit}
    >
      <Label className="sr-only">{t("uploads.partSizeAria")}</Label>
      <NumberField.Group>
        <NumberField.DecrementButton />
        <NumberField.Input />
        <span className="pr-2 text-xs text-muted">MiB</span>
        <NumberField.IncrementButton />
      </NumberField.Group>
    </NumberField>
  );
}
