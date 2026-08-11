import { Button, RouterProvider as AriaRouterProvider, Separator } from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import type { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  Link,
  Outlet,
  redirect,
  useLocation,
  useNavigate,
} from "@tanstack/react-router";
import { clsx } from "clsx";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import MenuIcon from "~icons/gravity-ui/bars";
import ChevronLeftIcon from "~icons/gravity-ui/chevron-left";
import ChevronRightIcon from "~icons/gravity-ui/chevron-right";
import SettingsIcon from "~icons/gravity-ui/gear";
import LogoIcon from "~icons/gravity-ui/layers";
import GridIcon from "~icons/gravity-ui/layout-header-cells";
import TasksIcon from "~icons/gravity-ui/list-ul";
import StorageIcon from "~icons/gravity-ui/database";
import SearchIcon from "~icons/gravity-ui/magnifier";
import MoonIcon from "~icons/gravity-ui/moon";
import SunIcon from "~icons/gravity-ui/sun";
import CloseIcon from "~icons/gravity-ui/xmark";
import { useCommandPalette } from "../components/command-palette-context";
import { SearchOverlay } from "../components/search-overlay";
import { UploadShelf } from "../components/upload-shelf";
import { currentUserQueryOptions } from "../auth/queries";
import { isUnauthorized } from "../api/errors";

const mainNav = [
  { label: "Files", icon: GridIcon, path: "/files" },
  { label: "Storage", icon: StorageIcon, path: "/storage" },
  { label: "Tasks", icon: TasksIcon, path: "/tasks" },
  { label: "Trash", icon: GridIcon, path: "/trash" },
] as const;

const bottomNav = [{ label: "Settings", icon: SettingsIcon, path: "/settings" }] as const;
const DESKTOP_BREAKPOINT = 1024;

function getPageTitle(pathname: string) {
  const item = [...mainNav, ...bottomNav].find(
    (entry) => pathname === entry.path || pathname.startsWith(entry.path),
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
  const renderItem = (item: (typeof mainNav)[number] | (typeof bottomNav)[number]) => {
    const link = (
      <Link
        key={item.label}
        to={item.path}
        preload="intent"
        activeOptions={{ exact: false }}
        className="h-10 w-full justify-start rounded-full px-3 text-sm font-medium"
        activeProps={{
          className: clsx(buttonVariants({ variant: "secondary" }), "bg-accent/10 text-accent"),
        }}
        inactiveProps={{
          className: clsx(
            buttonVariants({ variant: "ghost" }),
            "text-muted hover:bg-default/30 hover:text-foreground",
          ),
        }}
        onClick={() => onNavigate?.()}
      >
        <item.icon className="size-4 shrink-0" />
        <span
          className={clsx(
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
      className={clsx(
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
          className={clsx(
            "min-w-0 overflow-hidden whitespace-nowrap transition-[width,opacity] duration-200",
            collapsed && !mobile ? "max-w-0 opacity-0" : "max-w-40 opacity-100",
          )}
        >
          <p className="text-base font-semibold tracking-tight">Teldrive</p>
          <p className="text-[10px] uppercase tracking-[0.16em] text-muted">Cloud drive</p>
        </div>
      </div>

      <Separator className="mx-3 w-auto" />
      <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-4">{mainNav.map(renderItem)}</nav>
      <div className="space-y-1 border-t border-border px-3 py-3">{bottomNav.map(renderItem)}</div>
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

  if (pathname === "/login") return <Outlet />;
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
          <main className="min-w-0 flex-1 select-none overflow-y-auto px-3 py-4 sm:px-5 sm:py-5 lg:px-7 lg:py-6">
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
    if (location.pathname === "/login") return;
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
