import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
} from "@tw-material/react";
import IconIcOutlineSettingsBrightness from "~icons/ic/outline-settings-brightness";
import IconPhSun from "~icons/ph/sun";
import IconRiMoonClearLine from "~icons/ri/moon-clear-line";

import { useTheme } from "@/components/theme-provider";

export function ThemeToggle() {
  const { setTheme } = useTheme();

  return (
    <Dropdown
      classNames={{
        content:
          "min-w-40 bg-surface-container-high border border-outline-variant/30 rounded-2xl shadow-2xl",
      }}
    >
      <DropdownTrigger>
        <Button
          className="text-inherit hover:bg-on-surface/5"
          variant="text"
          isIconOnly
        >
          <IconPhSun className="pointer-events-none size-6 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
          <IconRiMoonClearLine
            className="pointer-events-none absolute size-6 rotate-90 scale-0 transition-all
            dark:rotate-0 dark:scale-100"
          />
          <span className="sr-only">Cambiar apariencia</span>
        </Button>
      </DropdownTrigger>
      <DropdownMenu
        aria-label="Menú de apariencia"
        classNames={{
          base: "bg-transparent",
        }}
        itemClasses={{
          base: "rounded-xl data-[hover=true]:bg-on-surface/10 px-4 py-2.5 transition-colors",
          title: "text-base font-medium",
          startContent: "text-on-surface-variant",
        }}
      >
        <DropdownItem
          key="light"
          onPress={() => setTheme("light")}
          startContent={<IconPhSun className="size-5" />}
        >
          Claro
        </DropdownItem>
        <DropdownItem
          key="dark"
          onPress={() => setTheme("dark")}
          startContent={<IconRiMoonClearLine className="size-5" />}
        >
          Oscuro
        </DropdownItem>
        <DropdownItem
          key="system"
          onPress={() => setTheme("system")}
          startContent={<IconIcOutlineSettingsBrightness className="size-5" />}
        >
          Automático
        </DropdownItem>
      </DropdownMenu>
    </Dropdown>
  );
}
