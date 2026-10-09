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
        title="Subidas"
        description="Configura subidas simultáneas, cifrado, conflictos de nombre y tamaño de fragmentos."
      />
      <SettingsSection
        title="Comportamiento de subidas"
        description="Estas preferencias se guardan en este navegador y se aplican a las nuevas subidas."
      >
        <SettingsRow
          label="Cifrado"
          description="Cifra los fragmentos con la clave del servidor antes de almacenarlos."
        >
          <Switch
            aria-label="Cifrar archivos subidos"
            isSelected={settings.encryption}
            onChange={(isSelected) => setSettings({ encryption: isSelected })}
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Label>Cifrar archivos subidos</Label>
            </Switch.Content>
          </Switch>
        </SettingsRow>
        <SettingsRow
          label="Name conflicts"
          description="Elige qué hacer si el destino ya contiene un elemento con el mismo nombre."
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
                <ListBox.Item id="rename" textValue="Renombrar el nuevo archivo">
                  Renombrar el nuevo archivo
                </ListBox.Item>
                <ListBox.Item id="replace" textValue="Reemplazar existente">
                  Reemplazar existente
                </ListBox.Item>
                <ListBox.Item id="error" textValue="Detener con error">
                  Detener con error
                </ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
        </SettingsRow>
        <SettingsRow
          label="Subidas simultáneas"
          description="Cantidad de archivos que este navegador sube simultáneamente."
        >
          <NumberField
            aria-label="Subidas simultáneas"
            value={settings.concurrency}
            minValue={1}
            maxValue={12}
            onChange={(value) =>
              setSettings({ concurrency: Math.max(1, Math.min(12, value ?? 1)) })
            }
          >
            <Label className="sr-only">Subidas simultáneas</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
          </NumberField>
        </SettingsRow>
        <SettingsRow
          label="Tamaño preferido de fragmento"
          description="El valor inicial es 512 MiB. En subidas cifradas se ajusta a múltiplos de 16 MiB; el servidor puede elegir otro tamaño."
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
      aria-label="Tamaño preferido de fragmento en MiB"
      value={value}
      maxValue={MAX_PART_SIZE_MIB}
      onChange={(next) => {
        valueRef.current = next ?? 512;
        setValue(valueRef.current);
      }}
      onBlur={commit}
    >
      <Label className="sr-only">Tamaño preferido de fragmento en MiB</Label>
      <NumberField.Group>
        <NumberField.DecrementButton />
        <NumberField.Input />
        <span className="pr-2 text-xs text-muted">MiB</span>
        <NumberField.IncrementButton />
      </NumberField.Group>
    </NumberField>
  );
}
