import { Label, ListBox, NumberField, Select, Switch } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { MAX_PART_SIZE_MIB, normalizePartSizeMiB, useUploadStore } from "@/features/uploads/store";

export const Route = createFileRoute("/_settings/settings/uploads")({ component: UploadSettings });

function UploadSettings() {
  const settings = useUploadStore((state) => state.settings);
  const setSettings = useUploadStore((state) => state.setSettings);

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Uploads"
        description="Browser upload concurrency, encryption, conflict handling, and multipart sizing."
      />
      <SettingsSection
        title="Upload behavior"
        description="These preferences are stored in this browser and apply to new uploads."
      >
        <SettingsRow
          label="Encryption"
          description="Encrypt file parts with Teldrive's server-managed key before storage."
        >
          <Switch
            aria-label="Encrypt uploaded files"
            isSelected={settings.encryption}
            onChange={(isSelected) => setSettings({ encryption: isSelected })}
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Label>Encrypt uploaded files</Label>
            </Switch.Content>
          </Switch>
        </SettingsRow>
        <SettingsRow
          label="Name conflicts"
          description="Choose what happens when the destination already contains the same name."
        >
          <Select
            aria-label="Name conflicts"
            selectedKey={settings.conflictPolicy}
            onSelectionChange={(key) =>
              setSettings({ conflictPolicy: String(key) as typeof settings.conflictPolicy })
            }
          >
            <Select.Trigger>
              <Select.Value />
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                <ListBox.Item id="rename" textValue="Rename new file">
                  Rename new file
                </ListBox.Item>
                <ListBox.Item id="replace" textValue="Replace existing">
                  Replace existing
                </ListBox.Item>
                <ListBox.Item id="error" textValue="Stop with error">
                  Stop with error
                </ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
        </SettingsRow>
        <SettingsRow
          label="Concurrent uploads"
          description="Number of browser uploads processed at the same time."
        >
          <NumberField
            aria-label="Concurrent uploads"
            value={settings.concurrency}
            minValue={1}
            maxValue={12}
            onChange={(value) =>
              setSettings({ concurrency: Math.max(1, Math.min(12, value ?? 1)) })
            }
          >
            <Label className="sr-only">Concurrent uploads</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
          </NumberField>
        </SettingsRow>
        <SettingsRow
          label="Preferred part size"
          description="Defaults to 512 MiB. Values are rounded to the nearest 16 MiB for encrypted uploads; the server may choose a different size."
        >
          <PartSizeField />
        </SettingsRow>
      </SettingsSection>
    </div>
  );
}

function PartSizeField() {
  const preferredPartSize = useUploadStore((state) => state.settings.preferredPartSize);
  const setSettings = useUploadStore((state) => state.setSettings);
  const [value, setValue] = useState(preferredPartSize / 1024 / 1024);
  const valueRef = useRef(value);

  const commit = () => {
    const normalized = normalizePartSizeMiB(valueRef.current);
    valueRef.current = normalized;
    setValue(normalized);
    setSettings({ preferredPartSize: normalized * 1024 * 1024 });
  };

  return (
    <NumberField
      aria-label="Preferred part size in MiB"
      value={value}
      maxValue={MAX_PART_SIZE_MIB}
      onChange={(next) => {
        valueRef.current = next ?? 512;
        setValue(valueRef.current);
      }}
      onBlur={commit}
    >
      <Label className="sr-only">Preferred part size in MiB</Label>
      <NumberField.Group>
        <NumberField.DecrementButton />
        <NumberField.Input />
        <span className="pr-2 text-xs text-muted">MiB</span>
        <NumberField.IncrementButton />
      </NumberField.Group>
    </NumberField>
  );
}
