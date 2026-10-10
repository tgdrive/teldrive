import type { SettingFieldConfig } from "@/config/settings";
import { debounce } from "@/utils/debounce";
import { Input, Select, SelectItem, Switch } from "@tw-material/react";
import clsx from "clsx";
import { memo, useCallback, useEffect, useState } from "react";

interface SettingsFieldProps<T> {
  config: SettingFieldConfig<T>;
  value: T;
  onChange: (value: T) => void;
  disabled?: boolean;
}

function validateUrl(value: string): boolean {
  if (!value) return true;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

export const SettingsField = memo(
  <T,>({ config, value, onChange, disabled }: SettingsFieldProps<T>) => {
    const [error, setError] = useState("");
    const [localValue, setLocalValue] = useState<T>(value);

    useEffect(() => {
      setLocalValue(value);
    }, [value]);

    const debouncedValidate = useCallback(
      debounce((newValue: T) => {
        validateAndSave(newValue);
      }, 1000),
      [config],
    );

    const validateAndSave = (newValue: T) => {
      let errorMessage = "";

      if (config.type === "url" && typeof newValue === "string") {
        if (newValue && !validateUrl(newValue)) {
          errorMessage = "La URL no es válida";
        }
      } else if (config.validation?.pattern && typeof newValue === "string") {
        if (newValue && !config.validation.pattern.test(newValue)) {
          errorMessage = "El formato no es válido";
        }
      } else if (config.validation?.custom && newValue) {
        const result = config.validation.custom(newValue as any);
        if (result !== true) {
          errorMessage = result;
        }
      }

      setError(errorMessage);

      if (!errorMessage) {
        onChange(newValue);
      }
    };

    const handleFieldChange = (newValue: T) => {
      setLocalValue(newValue);
      debouncedValidate(newValue);
    };

    const renderField = () => {
      switch (config.type) {
        case "text":
        case "email":
        case "url":
          return (
            <Input
              size="lg"
              variant="bordered"
              isInvalid={!!error}
              errorMessage={error}
              placeholder={config.placeholder}
              value={localValue as string}
              onValueChange={(v) => handleFieldChange(v as T)}
              isDisabled={disabled}
              classNames={{
                inputWrapper:
                  "bg-surface-container hover:bg-surface-container-high group-data-[focus=true]:bg-surface-container-high border-none transition-colors",
              }}
            />
          );

        case "number":
          return (
            <Input
              size="lg"
              variant="bordered"
              isInvalid={!!error}
              errorMessage={error}
              type="number"
              value={
                localValue !== undefined && localValue !== null
                  ? String(localValue)
                  : ""
              }
              onValueChange={(v) => handleFieldChange(Number(v) as T)}
              isDisabled={disabled}
              classNames={{
                inputWrapper:
                  "bg-surface-container hover:bg-surface-container-high group-data-[focus=true]:bg-surface-container-high border-none transition-colors",
              }}
            />
          );

        case "select":
          return (
            <Select
              aria-label={config.label}
              size="lg"
              variant="bordered"
              isInvalid={!!error}
              errorMessage={error}
              defaultSelectedKeys={[String(value)]}
              scrollShadowProps={{
                isEnabled: false,
              }}
              classNames={{
                popoverContent:
                  "bg-surface-container-high border border-outline-variant/30 rounded-[24px] shadow-2xl",
                trigger:
                  "bg-surface-container hover:bg-surface-container-high border-none transition-colors",
                listbox: "rounded-xl bg-transparent",
              }}
              listboxProps={{
                itemClasses: {
                  base: "rounded-xl hover:bg-on-surface/10 px-4 py-2.5 transition-colors",
                  title: "text-base font-medium",
                },
              }}
              items={config.options || []}
              onSelectionChange={(keys) => {
                const selected = Array.from(keys)[0];
                if (selected !== undefined) {
                  const option = config.options?.find(
                    (opt) => String(opt.value) === selected,
                  );
                  if (option) {
                    handleFieldChange(option.value as T);
                  }
                }
              }}
              isDisabled={disabled}
            >
              {(item: any) => (
                <SelectItem key={String(item.value)} value={String(item.value)}>
                  {item.label}
                </SelectItem>
              )}
            </Select>
          );

        case "switch":
          return (
            <Switch
              size="lg"
              onChange={(e) => handleFieldChange(e.target.checked as T)}
              isSelected={localValue as boolean}
              name={config.key}
              isDisabled={disabled}
            />
          );

        default:
          return null;
      }
    };

    return (
      <div className="flex flex-col md:flex-row gap-4 md:items-center justify-between py-2 border-b border-outline-variant/30 last:border-0 last:pb-0 first:pt-0">
        <div className="flex-1 min-w-0">
          <p className="text-base font-semibold text-on-surface">
            {config.label}
          </p>
          <p className="text-sm font-normal text-on-surface-variant max-w-xl">
            {config.description}
          </p>
        </div>
        <div
          className={clsx(
            "flex justify-start min-w-[200px] md:justify-end",
            disabled && "opacity-50",
          )}
        >
          {renderField()}
        </div>
      </div>
    );
  },
);
