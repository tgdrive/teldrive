import {
  Avatar,
  Button,
  Dropdown,
  Label,
  RouterProvider as AriaRouterProvider,
  Separator,
  cn,
} from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  Link,
  Outlet,
  redirect,
  useLocation,
  useNavigate,
} from "@tanstack/react-router";
import { useTheme } from "next-themes";
import { useEffect, useState, type Ref } from "react";
import { toast } from "sonner";
import MenuIcon from "~icons/gravity-ui/bars";
import ChevronLeftIcon from "~icons/gravity-ui/chevron-left";
import ChevronRightIcon from "~icons/gravity-ui/chevron-right";
import SettingsIcon from "~icons/gravity-ui/gear";
import LogoIcon from "~icons/gravity-ui/layers";
import GridIcon from "~icons/gravity-ui/layout-header-cells";
import FolderIcon from "~icons/gravity-ui/folder";
import TasksIcon from "~icons/gravity-ui/list-ul";
import StorageIcon from "~icons/gravity-ui/database";
import SearchIcon from "~icons/gravity-ui/magnifier";
import MoonIcon from "~icons/gravity-ui/moon";
import SunIcon from "~icons/gravity-ui/sun";
import CloseIcon from "~icons/gravity-ui/xmark";
import LogoutIcon from "~icons/gravity-ui/arrow-right-from-square";
import { useCommandPalette } from "../components/command-palette-context";
import { SearchOverlay } from "../components/search-overlay";
import { UploadShelf } from "../components/upload-shelf";
import { currentUserQueryOptions } from "../auth/queries";
import { $api } from "../api/client";
import { isUnauthorized, userMessage } from "../api/errors";
import { getQueryClient } from "../lib/queryClient";

const mainNav = [
  { label: "Files", icon: GridIcon, path: "/files" },
  { label: "Shared", icon: FolderIcon, path: "/shared" },
  { label: "Shared with me", icon: FolderIcon, path: "/shared-with-me" },
  { label: "Storage", icon: StorageIcon, path: "/storage" },
  { label: "Tasks", icon: TasksIcon, path: "/tasks", capability: "system.manageJobs" },
  { label: "Trash", icon: GridIcon, path: "/trash" },
] as const;

const DESKTOP_BREAKPOINT = 1024;

function getPageTitle(pathname: string) {
  if (pathname.startsWith("/settings")) return "Settings";
  const item = mainNav.find(
    (entry) => pathname === entry.path || pathname.startsWith(`${entry.path}/`),
  );
  return item?.label ?? "Teldrive";
}

function Sidebar({
  collapsed,
  mobile,
  onNavigate,
}: {
  collapsed: boolean;
  mobile?: boolean;
  onNavigate?: () => void;
}) {
  const navigate = useNavigate();
  const { data: user } = useQuery(currentUserQueryOptions());
  const logout = $api.useMutation("post", "/v1/auth/cookie/logout");
  const visibleMainNav = mainNav.filter(
    (item) => !("capability" in item) || Boolean(user?.capabilities.includes(item.capability)),
  );
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const displayName =
    user?.displayName?.trim() ||
    user?.username?.trim() ||
    (user ? `User ${user.userId}` : "Account");
  const secondaryLabel = user?.username
    ? `@${user.username}`
    : user?.premium
      ? "Telegram Premium"
      : "Telegram";
  const initials =
    displayName
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "U";
  const signOut = async () => {
    try {
      await logout.mutateAsync({});
      getQueryClient().clear();
      onNavigate?.();
      await navigate({ to: "/login", search: { redirect: "/files" }, replace: true });
    } catch (error) {
      toast.error("Unable to log out", { description: userMessage(error) });
    }
  };
  const renderItem = (item: (typeof mainNav)[number]) => {
    const link = (
      <Link
        key={item.label}
        to={item.path}
        preload="intent"
        className="h-10 w-full justify-start rounded-full px-3 text-sm font-medium"
        activeProps={{
          className: cn(buttonVariants({ variant: "secondary" }), "bg-accent/10 text-accent"),
        }}
        inactiveProps={{
          className: cn(
            buttonVariants({ variant: "ghost" }),
            "text-muted hover:bg-default/30 hover:text-foreground",
          ),
        }}
        onClick={() => onNavigate?.()}
      >
        <item.icon className="size-4 shrink-0" />
        <span
          className={cn(
            "overflow-hidden whitespace-nowrap transition-[width,opacity,margin] duration-200",
            collapsed && !mobile ? "ml-0 max-w-0 opacity-0" : "ml-3 max-w-52 opacity-100",
          )}
        >
          {item.label}
        </span>
      </Link>
    );

    return link;
  };

  return (
    <aside
      className={cn(
        "flex h-full flex-col overflow-hidden border-r border-border bg-sidebar/95 backdrop-blur-xl",
        !mobile && "transition-[width] duration-200",
        mobile ? "w-[min(19rem,86vw)] shadow-2xl" : collapsed ? "w-16" : "w-60",
      )}
    >
      <div className="flex h-16 items-center gap-3 px-4">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-accent text-accent-foreground">
          <LogoIcon className="size-4" />
        </div>
        <div
          className={cn(
            "min-w-0 overflow-hidden whitespace-nowrap transition-[width,opacity] duration-200",
            collapsed && !mobile ? "max-w-0 opacity-0" : "max-w-40 opacity-100",
          )}
        >
          <p className="text-base font-semibold tracking-tight">Teldrive</p>
          <p className="text-[10px] uppercase tracking-[0.16em] text-muted">Cloud drive</p>
        </div>
      </div>

      <Separator className="mx-3 w-auto" />
      <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-4">
        {visibleMainNav.map(renderItem)}
      </nav>
      <div className="border-t border-border px-3 py-3">
        <Dropdown isOpen={accountMenuOpen} onOpenChange={setAccountMenuOpen}>
          {collapsed && !mobile ? (
            <Dropdown.Trigger className="flex size-10 items-center justify-center rounded-full hover:bg-default/30">
              <Avatar className="size-8 cursor-pointer">
                <Avatar.Image alt={displayName} src="/api/v1/me/photo" />
                <Avatar.Fallback>{initials}</Avatar.Fallback>
              </Avatar>
            </Dropdown.Trigger>
          ) : (
            <Button
              variant="ghost"
              aria-label={`Open account menu for ${displayName}`}
              className="flex h-14 w-full items-center justify-start gap-3 rounded-xl px-2 text-muted hover:bg-default/30 hover:text-foreground"
            >
              <Avatar className="size-9 shrink-0">
                <Avatar.Image alt={displayName} src="/api/v1/me/photo" />
                <Avatar.Fallback>{initials}</Avatar.Fallback>
              </Avatar>
              <div className="min-w-0 overflow-hidden whitespace-nowrap text-left">
                <p className="truncate text-sm font-medium text-foreground">{displayName}</p>
                <p className="truncate text-xs text-muted">{secondaryLabel}</p>
              </div>
            </Button>
          )}
          <Dropdown.Popover placement="top start" className="min-w-52">
            <Dropdown.Menu
              aria-label="Account"
              onAction={(key) => {
                if (key === "logout") void signOut();
              }}
            >
              <Dropdown.Item
                id="settings"
                textValue="Settings"
                render={({ ref, ...itemProps }) => {
                  return (
                    // @ts-expect-error HeroUI types render props for a menu item div; this render target is an anchor.
                    <Link
                      {...itemProps}
                      ref={ref as Ref<HTMLAnchorElement>}
                      to="/settings"
                      preload="intent"
                      onClick={() => {
                        setAccountMenuOpen(false);
                        onNavigate?.();
                      }}
                    />
                  );
                }}
              >
                <SettingsIcon className="size-4" />
                <Label>Settings</Label>
              </Dropdown.Item>
              <Dropdown.Item id="logout" textValue="Log out" isDisabled={logout.isPending}>
                <LogoutIcon className="size-4" />
                <Label>{logout.isPending ? "Logging out…" : "Log out"}</Label>
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
      </div>
    </aside>
  );
}

function TopBar({
  collapsed,
  desktop,
  onToggleSidebar,
  onOpenMobile,
}: {
  collapsed: boolean;
  desktop: boolean;
  onToggleSidebar: () => void;
  onOpenMobile: () => void;
}) {
  const { resolvedTheme, setTheme } = useTheme();
  const commandPalette = useCommandPalette();
  const pathname = useLocation({ select: (location) => location.pathname });
  const title = getPageTitle(pathname);

  return (
    <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border bg-background/80 px-3 backdrop-blur-xl sm:px-5">
      <Button
        isIconOnly
        variant="ghost"
        size="sm"
        className="size-9 rounded-xl"
        onPress={desktop ? onToggleSidebar : onOpenMobile}
        aria-label={
          desktop ? (collapsed ? "Expand sidebar" : "Collapse sidebar") : "Open navigation"
        }
      >
        {desktop ? (
          collapsed ? (
            <ChevronRightIcon className="size-4" />
          ) : (
            <ChevronLeftIcon className="size-4" />
          )
        ) : (
          <MenuIcon className="size-4" />
        )}
      </Button>

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold sm:text-base">{title}</p>
      </div>

      <Button
        variant="ghost"
        className="hidden h-9 w-64 justify-between rounded-xl border border-border bg-surface/70 px-3 text-sm font-normal text-muted shadow-sm md:flex"
        onPress={commandPalette.open}
      >
        <span className="flex items-center gap-2">
          <SearchIcon className="size-3.5" />
          Search files
        </span>
        <kbd className="rounded-md border border-border bg-default/30 px-1.5 py-0.5 text-[10px]">
          Ctrl K
        </kbd>
      </Button>
      <Button
        isIconOnly
        variant="ghost"
        className="size-9 rounded-xl md:hidden"
        onPress={commandPalette.open}
        aria-label="Search files"
      >
        <SearchIcon className="size-4" />
      </Button>
      <Button
        isIconOnly
        variant="ghost"
        className="size-9 rounded-xl"
        onPress={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
        aria-label="Toggle color theme"
      >
        {resolvedTheme === "dark" ? (
          <SunIcon className="size-4" />
        ) : (
          <MoonIcon className="size-4" />
        )}
      </Button>
    </header>
  );
}

function Layout() {
  const navigate = useNavigate();
  const pathname = useLocation({ select: (location) => location.pathname });
  const [collapsed, setCollapsed] = useState(false);
  const [desktop, setDesktop] = useState(() =>
    typeof window === "undefined" ? true : window.innerWidth >= DESKTOP_BREAKPOINT,
  );
  const [mobileOpen, setMobileOpen] = useState(false);

  useEffect(() => {
    const media = window.matchMedia(`(min-width: ${DESKTOP_BREAKPOINT}px)`);
    const update = () => {
      setDesktop(media.matches);
      if (media.matches) setMobileOpen(false);
    };
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMobileOpen(false);
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  if (pathname === "/login" || pathname.startsWith("/share/")) return <Outlet />;
  return (
    <AriaRouterProvider
      navigate={(path) => navigate({ to: String(path) })}
      useHref={(path) => String(path)}
    >
      <div className="flex h-dvh w-full overflow-hidden bg-background text-foreground">
        {desktop && <Sidebar collapsed={collapsed} />}
        {!desktop && mobileOpen && (
          <div
            className="fixed inset-0 z-50 flex"
            role="dialog"
            aria-modal="true"
            aria-label="Navigation"
          >
            <Button
              type="button"
              variant="ghost"
              aria-label="Close navigation"
              className="absolute inset-0 h-full w-full rounded-none bg-black/55 backdrop-blur-sm"
              onPress={() => setMobileOpen(false)}
            />
            <div className="relative z-10 h-full animate-in slide-in-from-left duration-200">
              <Sidebar collapsed={false} mobile onNavigate={() => setMobileOpen(false)} />
              <Button
                isIconOnly
                variant="ghost"
                className="absolute right-3 top-3 size-9 rounded-xl"
                onPress={() => setMobileOpen(false)}
                aria-label="Close navigation"
              >
                <CloseIcon className="size-4" />
              </Button>
            </div>
          </div>
        )}

        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <TopBar
            collapsed={collapsed}
            desktop={desktop}
            onToggleSidebar={() => setCollapsed((value) => !value)}
            onOpenMobile={() => setMobileOpen(true)}
          />
          <main
            className={cn(
              "min-w-0 flex-1 select-none overflow-y-auto px-3 pb-4 sm:px-5 sm:pb-5 lg:px-7 lg:pb-6",
              pathname === "/files" ? "pt-[2px]" : "pt-4 sm:pt-5 lg:pt-6",
            )}
          >
            <Outlet />
          </main>
        </div>
        <SearchOverlay />
        <UploadShelf />
      </div>
    </AriaRouterProvider>
  );
}

export type RouterContext = {
  queryClient: QueryClient;
};

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context, location }) => {
    if (location.pathname === "/login" || location.pathname.startsWith("/share/")) return;
    try {
      await context.queryClient.ensureQueryData(currentUserQueryOptions());
    } catch (error) {
      if (!isUnauthorized(error)) throw error;
      throw redirect({
        to: "/login",
        search: { redirect: location.href },
        replace: true,
      });
    }
  },
  component: Layout,
});
