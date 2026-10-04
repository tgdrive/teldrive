import {
  RouterProvider as AriaRouterProvider,
  Avatar,
  Button,
  cn,
  Dropdown,
  InputGroup,
  Kbd,
  Label,
  Separator,
} from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import { type QueryClient, useQuery } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  Link,
  Outlet,
  redirect,
  useLocation,
  useNavigate,
} from "@tanstack/react-router";
import { useTheme } from "next-themes";
import { type Ref, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import LogoutIcon from "~icons/gravity-ui/arrow-right-from-square";
import MenuIcon from "~icons/gravity-ui/bars";
import ChevronLeftIcon from "~icons/gravity-ui/chevron-left";
import ChevronRightIcon from "~icons/gravity-ui/chevron-right";
import StorageIcon from "~icons/gravity-ui/database";
import FolderIcon from "~icons/gravity-ui/folder";
import SettingsIcon from "~icons/gravity-ui/gear";
import LogoIcon from "~icons/gravity-ui/layers";
import GridIcon from "~icons/gravity-ui/layout-header-cells";
import TasksIcon from "~icons/gravity-ui/list-ul";
import SearchIcon from "~icons/gravity-ui/magnifier";
import MoonIcon from "~icons/gravity-ui/moon";
import SunIcon from "~icons/gravity-ui/sun";
import CloseIcon from "~icons/gravity-ui/xmark";
import { $api } from "../api/client";
import { isUnauthorized, userMessage } from "../api/errors";
import { currentUserQueryOptions } from "../auth/queries";
import { UploadShelf } from "../components/upload-shelf";
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
  if (pathname === "/search") return "Search";
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
        activeOptions={{ exact: true }}
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
  const navigate = useNavigate();
  const searchRef = useRef<HTMLInputElement>(null);
  const searchTimer = useRef<number | undefined>(undefined);
  const composingSearch = useRef(false);
  const [searchText, setSearchText] = useState("");
  const pathname = useLocation({ select: (location) => location.pathname });
  const routeSearch = useLocation({
    select: (location) => location.search as Record<string, unknown>,
  });
  const locationKey = useLocation({ select: (location) => location.href });
  const title = getPageTitle(pathname);
  const scheduleSearch = (value: string) => {
    if (searchTimer.current) window.clearTimeout(searchTimer.current);
    searchTimer.current = undefined;
    const q = value.trim() || undefined;
    if (pathname !== "/search" || composingSearch.current || q === routeSearch.q) return;
    searchTimer.current = window.setTimeout(() => {
      searchTimer.current = undefined;
      void navigate({ to: "/search", search: { ...routeSearch, q }, replace: true });
    }, 300);
  };

  useEffect(() => {
    if (pathname === "/search")
      setSearchText(typeof routeSearch.q === "string" ? routeSearch.q : "");
  }, [pathname, routeSearch]);
  useEffect(
    () => () => {
      if (searchTimer.current) window.clearTimeout(searchTimer.current);
    },
    [locationKey],
  );

  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (
        event.isComposing ||
        (event.target instanceof HTMLElement && event.target.closest('[role="dialog"]'))
      )
        return;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  }, []);

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

      <div className="hidden min-w-0 flex-1 md:block">
        <p className="truncate text-sm font-semibold sm:text-base">{title}</p>
      </div>

      <search aria-label="Search drive" className="flex min-w-0 flex-1 items-center md:max-w-md">
        <form
          className="w-full"
          onSubmit={(event) => {
            event.preventDefault();
            if (composingSearch.current) return;
            if (searchTimer.current) window.clearTimeout(searchTimer.current);
            searchTimer.current = undefined;
            void navigate({
              to: "/search",
              search: {
                ...(pathname === "/search" ? routeSearch : {}),
                ...(pathname === "/files"
                  ? {
                      parentId:
                        typeof routeSearch.parentId === "string" ? routeSearch.parentId : undefined,
                      folderPath:
                        typeof routeSearch.path === "string" && routeSearch.path !== "/"
                          ? routeSearch.path
                          : undefined,
                    }
                  : {}),
                q: searchText.trim() || undefined,
              },
            });
          }}
        >
          <InputGroup className="w-full" variant="secondary">
            <InputGroup.Prefix>
              <SearchIcon className="size-4 text-muted" />
            </InputGroup.Prefix>
            <InputGroup.Input
              ref={searchRef}
              aria-label="Search files"
              value={searchText}
              maxLength={512}
              enterKeyHint="search"
              onCompositionStart={() => {
                composingSearch.current = true;
                if (searchTimer.current) window.clearTimeout(searchTimer.current);
              }}
              onCompositionEnd={(event) => {
                composingSearch.current = false;
                setSearchText(event.currentTarget.value);
                scheduleSearch(event.currentTarget.value);
              }}
              onKeyDown={(event) => {
                if (
                  event.key === "Enter" &&
                  (event.nativeEvent.isComposing || composingSearch.current)
                )
                  event.preventDefault();
              }}
              onChange={(event) => {
                const value = event.target.value;
                setSearchText(value);
                scheduleSearch(value);
              }}
              placeholder="Search files"
              className="min-w-0 text-sm"
            />
            <InputGroup.Suffix className="hidden md:flex">
              <Kbd>Ctrl K</Kbd>
            </InputGroup.Suffix>
          </InputGroup>
        </form>
      </search>
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
          <main className="min-w-0 flex-1 select-none overflow-y-auto px-3 py-4 sm:px-5 sm:py-5 lg:px-7 lg:py-6">
            <Outlet />
          </main>
        </div>
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
