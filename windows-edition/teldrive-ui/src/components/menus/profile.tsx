import { useNavigate } from "@tanstack/react-router";
import {
  Avatar,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
} from "@tw-material/react";
import IconBaselineLogout from "~icons/ic/baseline-logout";
import IconOutlineSettings from "~icons/ic/outline-settings";

import { $api } from "@/utils/api";
import { useSession } from "@/utils/query-options";
import { useCallback } from "react";

export function ProfileDropDown() {
  const [session] = useSession();

  const signOut = $api.useMutation("post", "/auth/logout");

  const navigate = useNavigate();

  const onSignOut = useCallback(() => {
    signOut.mutateAsync({}).then(() => {
      window.location.replace(new URL("/login", window.location.origin));
    });
  }, [signOut]);

  return (
    <Dropdown
      classNames={{
        content:
          "min-w-40 bg-surface-container-high border border-outline-variant/30 rounded-2xl shadow-2xl",
      }}
    >
      <DropdownTrigger>
        <Avatar
          as="button"
          size="sm"
          showFallback
          className="outline-none shrink-0 hover:ring-4 ring-on-surface/5 transition-all"
          src={"/api/users/profile/profile.jpeg"}
        />
      </DropdownTrigger>
      <DropdownMenu
        aria-label="Menú de cuenta"
        onAction={(key) => {
          if (String(key) === "settings") {
            navigate({ to: "/settings/$tabId", params: { tabId: "general" } });
          }
        }}
        classNames={{
          base: "bg-transparent",
        }}
        itemClasses={{
          base: "rounded-xl data-[hover=true]:bg-on-surface/10 px-4 py-2.5 transition-colors",
          title: "text-base font-medium",
          startContent: "text-on-surface-variant",
          endContent: "text-on-surface-variant",
        }}
      >
        <DropdownItem
          key="profile"
          className="pointer-events-none mb-1 data-[hover=true]:bg-transparent"
        >
          <div className="flex flex-col">
            <p className="text-xs text-on-surface-variant">Sesión iniciada como</p>
            <p className="font-bold text-on-surface">{session?.userName}</p>
          </div>
        </DropdownItem>

        <DropdownItem
          key="settings"
          endContent={<IconOutlineSettings className="size-5" />}
        >
          Configuración
        </DropdownItem>
        <DropdownItem
          key="logout"
          className="text-error data-[hover=true]:bg-error/10 data-[hover=true]:text-error"
          endContent={<IconBaselineLogout className="size-5" />}
          onPress={onSignOut}
        >
          Cerrar sesión
        </DropdownItem>
      </DropdownMenu>
    </Dropdown>
  );
}
