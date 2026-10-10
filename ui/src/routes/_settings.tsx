import { Button, Modal, Separator, Typography } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { useState } from "react";
import KeyIcon from "~icons/gravity-ui/key";
import MenuIcon from "~icons/gravity-ui/bars";
import PaletteIcon from "~icons/gravity-ui/palette";
import PersonIcon from "~icons/gravity-ui/person";
import RobotIcon from "~icons/gravity-ui/layers";
import SessionsIcon from "~icons/gravity-ui/list-ul";
import StorageIcon from "~icons/gravity-ui/database";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import CloseIcon from "~icons/gravity-ui/xmark";
import ClockIcon from "~icons/gravity-ui/clock";
import { currentUserQueryOptions } from "@/auth/queries";

const SETTINGS_GROUPS = [
  {
    label: "Account",
    items: [
      { label: "Overview", path: "/settings", icon: PersonIcon },
      {
        label: "Users & roles",
        path: "/settings/users",
        icon: PersonIcon,
        capability: "system.manageUsers",
      },
    ],
  },
  {
    label: "Telegram storage",
    items: [
      { label: "Channels", path: "/settings/channels", icon: StorageIcon },
      { label: "Bots", path: "/settings/bots", icon: RobotIcon },
    ],
  },
  {
    label: "Security",
    items: [
      { label: "Sessions", path: "/settings/sessions", icon: SessionsIcon },
      { label: "API keys", path: "/settings/api-keys", icon: KeyIcon },
    ],
  },
  {
    label: "Preferences",
    items: [
      { label: "Uploads", path: "/settings/uploads", icon: UploadIcon },
      { label: "Appearance", path: "/settings/appearance", icon: PaletteIcon },
    ],
  },
  {
    label: "System",
    items: [
      {
        label: "Periodic Jobs",
        path: "/settings/periodic-jobs",
        icon: ClockIcon,
        capability: "system.maintenance",
      },
    ],
  },
] as const;

export const Route = createFileRoute("/_settings")({ component: SettingsLayout });

function SettingsLayout() {
  const location = useLocation();
  const { data: user } = useQuery(currentUserQueryOptions());
  const [mobileOpen, setMobileOpen] = useState(false);
  const visibleGroups = settingsGroupsFor(user?.capabilities ?? []);
  const activeLabel = visibleGroups.reduce<string | undefined>(
    (found, group) => found ?? group.items.find((item) => item.path === location.pathname)?.label,
    undefined,
  );

  return (
    <div className="mx-auto grid w-full max-w-7xl gap-6 lg:grid-cols-[15rem_minmax(0,1fr)]">
      <aside className="hidden lg:block">
        <div className="sticky top-0 flex max-h-[calc(100dvh-7rem)] flex-col gap-5 overflow-y-auto border-r border-border pr-5">
          <div>
            <Typography type="h2" className="text-lg font-semibold">
              Settings
            </Typography>
            <Typography.Paragraph className="mt-1 text-xs text-muted">
              Configure Teldrive and this browser.
            </Typography.Paragraph>
          </div>
          <SettingsNavigation currentPath={location.pathname} groups={visibleGroups} />
        </div>
      </aside>
      <main className="min-w-0">
        <div className="mb-5 flex items-center justify-between border-b border-border pb-4 lg:hidden">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              {activeLabel ?? "Settings"}
            </Typography>
            <Typography.Paragraph className="text-xs text-muted">
              Teldrive settings
            </Typography.Paragraph>
          </div>
          <Button
            isIconOnly
            size="sm"
            variant="tertiary"
            aria-label="Open settings navigation"
            onPress={() => setMobileOpen(true)}
          >
            <MenuIcon className="size-4" />
          </Button>
        </div>
        <Outlet />
      </main>
      <Modal.Backdrop isOpen={mobileOpen} onOpenChange={setMobileOpen} isDismissable>
        <Modal.Container size="sm" className="mr-auto h-dvh max-h-dvh rounded-none">
          <Modal.Dialog className="h-full rounded-none">
            <Modal.Header className="flex-row items-center justify-between border-b border-border">
              <div>
                <Modal.Heading>Settings</Modal.Heading>
                <Typography.Paragraph className="text-xs text-muted">
                  Choose a settings area.
                </Typography.Paragraph>
              </div>
              <Button
                isIconOnly
                size="sm"
                variant="tertiary"
                aria-label="Close settings navigation"
                onPress={() => setMobileOpen(false)}
              >
                <CloseIcon className="size-4" />
              </Button>
            </Modal.Header>
            <Modal.Body className="py-5">
              <SettingsNavigation
                currentPath={location.pathname}
                groups={visibleGroups}
                onNavigate={() => setMobileOpen(false)}
              />
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </div>
  );
}

function SettingsNavigation({
  currentPath,
  groups,
  onNavigate,
}: {
  currentPath: string;
  groups: SettingsGroup[];
  onNavigate?: () => void;
}) {
  return (
    <nav aria-label="Settings navigation" className="flex flex-col gap-5">
      {groups.map((group, groupIndex) => (
        <div key={group.label} className="flex flex-col gap-1.5">
          {groupIndex > 0 ? <Separator className="mb-3" /> : null}
          <p className="px-2 text-[0.68rem] font-semibold uppercase tracking-[0.14em] text-muted">
            {group.label}
          </p>
          {group.items.map((item) => {
            const active = currentPath === item.path;
            return (
              <Link
                key={item.path}
                to={item.path}
                onClick={onNavigate}
                className={`flex min-h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium transition-colors ${active ? "bg-accent/10 text-accent" : "text-muted hover:bg-default/30 hover:text-foreground"}`}
              >
                <item.icon className="size-4 shrink-0" />
                <span>{item.label}</span>
              </Link>
            );
          })}
        </div>
      ))}
    </nav>
  );
}

type SettingsItem = (typeof SETTINGS_GROUPS)[number]["items"][number];
type SettingsGroup = { label: string; items: SettingsItem[] };

function settingsGroupsFor(capabilities: string[]): SettingsGroup[] {
  const allowed = new Set(capabilities);
  return SETTINGS_GROUPS.map((group) => ({
    label: group.label,
    items: group.items.filter((item) => !("capability" in item) || allowed.has(item.capability)),
  })).filter((group) => group.items.length > 0) as SettingsGroup[];
}
