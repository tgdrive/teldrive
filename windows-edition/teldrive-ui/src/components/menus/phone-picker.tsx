import { useFilter } from "@react-aria/i18n";
import {
  Divider,
  Input,
  Listbox,
  ListboxItem,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@tw-material/react";
import clsx from "clsx";
import { memo, useMemo, useState } from "react";
import type { ControllerRenderProps } from "react-hook-form";

import { type FormState, isoCodeMap, isoCodes } from "@/components/login";
import { scrollbarClasses } from "@/utils/classes";
import { flags } from "@/utils/country-flags";

interface PhoneNoPickerProps {
  field: ControllerRenderProps<FormState, "phoneCode">;
}

export const PhoneNoPicker = memo(({ field }: PhoneNoPickerProps) => {
  const { contains } = useFilter({
    sensitivity: "base",
  });

  const [isOpen, setIsOpen] = useState(false);

  const [value, setValue] = useState("");

  const codes = useMemo(
    () => isoCodes.filter((composer) => contains(composer.country, value)),
    [value],
  );
  const TriggerIcon = flags[field.value as keyof typeof flags];

  return (
    <Popover
      classNames={{
        base: "max-w-xs",
        content:
          "rounded-[24px] shadow-2xl bg-surface-container-high border border-outline-variant/30 p-2",
      }}
      placement="bottom-start"
      offset={12}
      triggerScaleOnOpen={false}
      isOpen={isOpen}
      onOpenChange={(open) => setIsOpen(open)}
    >
      <PopoverTrigger>
        <button
          type="button"
          className="outline-none flex gap-3 items-center shrink-0"
        >
          <TriggerIcon width={30} height={20} className="rounded-sm" />
          <span className="text-on-surface font-medium min-w-10">
            {isoCodeMap[field.value].value}
          </span>
          <Divider className="h-6" orientation="vertical" />
        </button>
      </PopoverTrigger>
      <PopoverContent>
        <div className="flex flex-col w-full gap-2">
          <Input
            value={value}
            placeholder="Buscar país…"
            className="w-full px-2 pt-2"
            onValueChange={setValue}
            isClearable
            variant="flat"
            classNames={{
              inputWrapper: "rounded-2xl bg-surface-container",
            }}
          />
          <Listbox
            aria-label="Código de país"
            items={codes}
            className={clsx("max-h-72 overflow-y-auto pr-1", scrollbarClasses)}
            onAction={(key) => {
              field.onChange({ target: { value: key } });
              setIsOpen(false);
            }}
            itemClasses={{
              base: "rounded-xl data-[hover=true]:bg-on-surface/10 px-4 py-2.5 transition-colors",
              title: "text-base font-medium",
            }}
            isVirtualized
          >
            {(item) => {
              const Flag = flags[item.code as keyof typeof flags];
              return (
                <ListboxItem
                  key={item.code}
                  textValue={item.country}
                  hideSelectedIcon
                >
                  <div className="flex w-full items-center gap-4">
                    <Flag
                      className="shrink-0 rounded-sm"
                      width={24}
                      height={16}
                    />
                    <span className="flex-1 truncate">{item.country}</span>
                    <span className="text-on-surface-variant font-mono text-sm">
                      {item.value}
                    </span>
                  </div>
                </ListboxItem>
              );
            }}
          </Listbox>
        </div>
      </PopoverContent>
    </Popover>
  );
});
