import { t } from "@/lib/localization";
import { Label, ListBox, NumberField, Select, Switch } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useRef, useState } from "react";
import type { UploadPreferences } from "@/api/types";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
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

  if (!uploads) return null;

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("Subidas")}
        description={t(
          "Configura subidas simultáneas, cifrado, conflictos de nombre y tamaño de fragmentos.",
        )}
      />
      <SettingsSection
        title={t("Comportamiento de subidas")}
        description={t("These preferences are saved to your account and apply to new uploads.")}
      >
        <SettingsRow
          label={t("Cifrado")}
          description={t("Cifra los fragmentos con la clave del servidor antes de almacenarlos.")}
        >
          <Switch
            aria-label={t("Cifrar archivos subidos")}
            isSelected={uploads.encryption}
            onChange={(isSelected) => updatePreferences({ uploads: { encryption: isSelected } })}
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Label>{t("Cifrar archivos subidos")}</Label>
            </Switch.Content>
          </Switch>
        </SettingsRow>
        <SettingsRow
          label={t("Conflictos de nombre")}
          description={t(
            "Elige qué hacer si el destino ya contiene un elemento con el mismo nombre.",
          )}
        >
          <Select
            aria-label={t("Conflictos de nombre")}
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
                <ListBox.Item id="rename" textValue={t("Renombrar el nuevo archivo")}>
                  {t("Renombrar el nuevo archivo")}
                </ListBox.Item>
                <ListBox.Item id="replace" textValue={t("Reemplazar existente")}>
                  {t("Reemplazar existente")}
                </ListBox.Item>
                <ListBox.Item id="fail" textValue={t("Detener con error")}>
                  {t("Detener con error")}
                </ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
        </SettingsRow>
        <SettingsRow
          label={t("Subidas simultáneas")}
          description={t("Cantidad de archivos que este navegador sube simultáneamente.")}
        >
          <NumberField
            aria-label={t("Concurrent uploads")}
            value={uploads.concurrency}
            minValue={1}
            maxValue={12}
            onChange={(value) =>
              updatePreferences({
                uploads: { concurrency: Math.max(1, Math.min(12, value ?? 1)) },
              })
            }
          >
            <Label className="sr-only">{t("Subidas simultáneas")}</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
          </NumberField>
        </SettingsRow>
        <SettingsRow
          label={t("Tamaño preferido de fragmento")}
          description={t(
            "El valor inicial es 512 MiB. En subidas cifradas se ajusta a múltiplos de 16 MiB; el servidor puede elegir otro tamaño.",
          )}
        >
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

  const commit = () => {
    const normalized = normalizePartSizeMiB(valueRef.current);
    valueRef.current = normalized;
    setValue(normalized);
    updatePreferences({ uploads: { preferredPartSize: normalized * 1024 * 1024 } });
  };

  return (
    <NumberField
      aria-label={t("Tamaño preferido de fragmento en MiB")}
      value={value}
      maxValue={MAX_PART_SIZE_MIB}
      onChange={(next) => {
        valueRef.current = next ?? 512;
        setValue(valueRef.current);
      }}
      onBlur={commit}
    >
      <Label className="sr-only">{t("Tamaño preferido de fragmento en MiB")}</Label>
      <NumberField.Group>
        <NumberField.DecrementButton />
        <NumberField.Input />
        <span className="pr-2 text-xs text-muted">MiB</span>
        <NumberField.IncrementButton />
      </NumberField.Group>
    </NumberField>
  );
}
