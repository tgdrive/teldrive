import {
  Button,
  cn,
  Drawer,
  ListBox,
  Popover,
  Slider,
  Spinner,
  Tabs,
  useOverlayState,
} from "@heroui/react";
import { useCallback, useEffect, useRef, useState } from "react";

import type { FileEntry } from "@/api/types";
import {
  applyPublicationAppearance,
  closePublication,
  openPublication,
  type ReaderPreferences,
} from "@/features/files/foliate-reader";
import type { ViewState } from "@/features/files/view-state";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import MenuIcon from "~icons/gravity-ui/bars";
import LeftIcon from "~icons/gravity-ui/chevron-left";
import RightIcon from "~icons/gravity-ui/chevron-right";
import CloseIcon from "~icons/gravity-ui/xmark";

export type EpubReaderProps = {
  file: FileEntry;
  url: string;
  state?: ViewState;
  onPosition: (position: Record<string, unknown>, preferences?: Record<string, unknown>) => void;
  onClose: () => void;
};

type TocItem = {
  id: string;
  label: string;
  href: string;
  depth: number;
};

type Location = { current?: number; total?: number };

export function EpubReader({ file, url, state, onPosition, onClose }: EpubReaderProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<FoliateViewElement | undefined>(undefined);
  const activeRef = useRef(true);
  const navigationTasksRef = useRef(new Set<Promise<unknown>>());
  const preferencesRef = useRef<ReaderPreferences | undefined>(undefined);
  const onPositionRef = useRef(onPosition);
  const onCloseRef = useRef(onClose);
  const openingRef = useRef<Promise<void> | undefined>(undefined);
  const loadedDocumentsRef = useRef(new Set<Document>());
  const closingRef = useRef(false);
  const closedRef = useRef(false);
  const positionRef = useRef<Record<string, unknown>>({
    cfi: typeof state?.position.cfi === "string" ? state.position.cfi : undefined,
    fraction: numberValue(state?.position.fraction),
  });
  onPositionRef.current = onPosition;
  onCloseRef.current = onClose;
  const isDesktop = useMediaQuery("(min-width: 1024px)");
  const drawerState = useOverlayState();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string>();
  const [closing, setClosing] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [toc, setToc] = useState<TocItem[]>([]);
  const [title, setTitle] = useState(file.name);
  const [chapter, setChapter] = useState<string>();
  const [progress, setProgress] = useState(numberValue(state?.position.fraction));
  const [location, setLocation] = useState<Location>({});
  const [theme, setTheme] = useState(String(state?.preferences.theme || "paper"));
  const [flow, setFlow] = useState(String(state?.preferences.flow || "paginated"));
  const [font, setFont] = useState(String(state?.preferences.font || "publisher"));
  const [fontSize, setFontSize] = useState(numberValue(state?.preferences.fontSize) || 100);
  const [lineHeight, setLineHeight] = useState(numberValue(state?.preferences.lineHeight) || 1.55);
  const [margin, setMargin] = useState(numberValue(state?.preferences.margin) || 48);
  const [columns, setColumns] = useState(numberValue(state?.preferences.columns) || 2);

  const preferences: ReaderPreferences = {
    theme,
    flow,
    font,
    fontSize,
    lineHeight,
    margin,
    columns,
  };
  preferencesRef.current = preferences;

  const persist = useCallback(
    (
      position: Record<string, unknown>,
      nextPreferences: ReaderPreferences = preferencesRef.current!,
    ) => {
      onPositionRef.current(position, { ...nextPreferences });
    },
    [],
  );

  const trackNavigation = useCallback((task: Promise<unknown>) => {
    const tracked = Promise.resolve(task).catch(() => undefined);
    navigationTasksRef.current.add(tracked);
    void tracked.finally(() => navigationTasksRef.current.delete(tracked));
  }, []);

  const navigate = useCallback(
    (direction: "previous" | "next") => {
      const view = viewRef.current;
      if (!ready || !view?.book) return;
      trackNavigation(direction === "previous" ? view.goLeft() : view.goRight());
    },
    [ready, trackNavigation],
  );

  const goTo = useCallback(
    (href: string) => {
      const view = viewRef.current;
      if (!ready || !view?.book) return;
      trackNavigation(view.goTo(href));
      drawerState.close();
    },
    [drawerState, ready, trackNavigation],
  );

  const toggleNavigation = () => {
    if (isDesktop) setSidebarOpen((value) => !value);
    else drawerState.open();
  };

  useEffect(() => {
    activeRef.current = true;
    const host = hostRef.current;
    if (!host) return;

    let element: FoliateViewElement | undefined;
    const loadedDocuments = loadedDocumentsRef.current;

    const onReaderKeyDown = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) return;
      if (event.key === "ArrowLeft" || event.key === "PageUp") {
        event.preventDefault();
        const view = viewRef.current;
        if (view?.book) trackNavigation(view.goLeft());
      } else if (event.key === "ArrowRight" || event.key === "PageDown" || event.key === " ") {
        event.preventDefault();
        const view = viewRef.current;
        if (view?.book) trackNavigation(view.goRight());
      }
    };

    const onLoad = (event: CustomEvent<{ doc: Document }>) => {
      if (!activeRef.current) return;
      const doc = event.detail.doc;
      loadedDocuments.add(doc);
      doc.addEventListener("keydown", onReaderKeyDown);
      const updateRenderedContent = () => {
        const text = doc.body?.innerText.trim();
        const imageLabel = doc.querySelector("img")?.getAttribute("alt");
        if (text || imageLabel) element!.dataset.renderedContent = text || imageLabel || "";
        return Boolean(text || imageLabel);
      };
      if (!updateRenderedContent() && doc.body) {
        const observer = new MutationObserver(() => {
          if (!activeRef.current || updateRenderedContent()) observer.disconnect();
        });
        observer.observe(doc.body, { childList: true, subtree: true });
      }
    };

    const onRelocate = (event: CustomEvent<FoliateRelocateDetail>) => {
      if (!activeRef.current) return;
      const fraction = event.detail.fraction || 0;
      setProgress(fraction);
      setChapter(event.detail.tocItem?.label);
      setLocation(event.detail.location || {});
      positionRef.current = { cfi: event.detail.cfi, fraction };
      persist(positionRef.current, preferencesRef.current);
    };

    const open = async () => {
      setReady(false);
      setError(undefined);
      await import("foliate-js/view.js");
      if (!activeRef.current) return;

      element = document.createElement("foliate-view");
      element.className = "block h-full min-h-0 w-full";
      host.replaceChildren(element);
      viewRef.current = element;

      const response = await fetch(url);
      if (!response.ok) throw new Error(`Unable to load publication (${response.status}).`);
      const bookFile = new File([await response.blob()], file.name, { type: file.mimeType });
      if (!activeRef.current) return;

      const cfi = typeof state?.position.cfi === "string" ? state.position.cfi : undefined;
      const lastLocation = cfi || (progress > 0 ? { fraction: progress } : undefined);
      await openPublication({
        element,
        file: bookFile,
        preferences: preferencesRef.current!,
        lastLocation,
        onLoad,
        onRelocate,
      });
      if (!activeRef.current) return;

      const metadata = element.book?.metadata || {};
      const metadataTitle = typeof metadata.title === "string" ? metadata.title.trim() : "";
      if (metadataTitle) setTitle(metadataTitle);
      setToc(flattenToc(element.book?.toc || []));
      setReady(true);
    };

    openingRef.current = open().catch((reason: unknown) => {
      if (activeRef.current) {
        setError(
          reason instanceof Error ? reason.message : "This publication could not be opened.",
        );
      }
    });

    return () => {
      activeRef.current = false;
      const current = element;
      for (const doc of loadedDocuments) doc.removeEventListener("keydown", onReaderKeyDown);
      current?.removeEventListener("load", onLoad as EventListener);
      current?.removeEventListener("relocate", onRelocate as EventListener);
      if (closedRef.current) current?.remove();
    };
  }, [file.id, file.mimeType, file.name, persist, trackNavigation, url]);

  const requestClose = useCallback(() => {
    if (closingRef.current) return;
    closingRef.current = true;
    activeRef.current = false;
    setClosing(true);
    setReady(false);
    setSettingsOpen(false);
    drawerState.close();

    const finishClose = async () => {
      const pending = [...navigationTasksRef.current];
      if (openingRef.current) pending.push(openingRef.current);
      await Promise.allSettled(pending);

      const fontLoads = [...loadedDocumentsRef.current].map((doc) => doc.fonts.ready);
      await Promise.allSettled(fontLoads);
      await nextAnimationFrame();
      await nextAnimationFrame();

      const current = viewRef.current;
      if (current && !closedRef.current) {
        await closePublication(current);
        closedRef.current = true;
        viewRef.current = undefined;
      }

      await nextAnimationFrame();
      onCloseRef.current();
    };

    void finishClose();
  }, [drawerState]);

  useEffect(() => {
    const view = viewRef.current;
    if (view) applyPublicationAppearance(view, preferences);
  }, [columns, flow, font, fontSize, lineHeight, margin, theme]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) return;
      if (event.key === "Escape") {
        if (settingsOpen) setSettingsOpen(false);
        else if (drawerState.isOpen) drawerState.close();
        else requestClose();
        return;
      }
      if (event.key === "ArrowLeft" || event.key === "PageUp") navigate("previous");
      else if (event.key === "ArrowRight" || event.key === "PageDown") navigate("next");
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [drawerState, navigate, requestClose, settingsOpen]);

  const commitPreferences = (next: Partial<ReaderPreferences>) => {
    const merged = { ...preferencesRef.current!, ...next };
    persist(positionRef.current, merged);
  };

  const navigation = (
    <EpubNavigation file={file} toc={toc} activeChapter={chapter} onNavigate={goTo} />
  );

  return (
    <div
      data-epub-reader
      data-reader-theme={theme}
      className="reader-shell flex h-dvh min-h-0 flex-col overflow-hidden"
    >
      <header
        data-epub-header
        className="reader-chrome z-20 flex h-14 shrink-0 items-center gap-2 border-b px-2 sm:h-16 sm:px-3"
      >
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Open ebook navigation"
          onPress={toggleNavigation}
        >
          <MenuIcon className="size-5" />
        </Button>

        <div className="min-w-0 flex-1 px-1 sm:px-2">
          <p className="truncate text-sm font-semibold tracking-[-0.01em]">{title}</p>
          <p className="truncate text-[11px] text-(--reader-muted)">{chapter || "EPUB reader"}</p>
        </div>

        <div className="hidden min-w-28 text-center text-[11px] text-(--reader-muted) md:block">
          {locationLabel(location, progress)}
        </div>

        <EpubSettings
          isOpen={settingsOpen}
          onOpenChange={setSettingsOpen}
          theme={theme}
          flow={flow}
          font={font}
          fontSize={fontSize}
          lineHeight={lineHeight}
          margin={margin}
          columns={columns}
          onTheme={(value) => {
            setTheme(value);
            commitPreferences({ theme: value });
          }}
          onFlow={(value) => {
            setFlow(value);
            commitPreferences({ flow: value });
          }}
          onFont={(value) => {
            setFont(value);
            commitPreferences({ font: value });
          }}
          onFontSize={(value) => setFontSize(value)}
          onFontSizeCommit={(value) => commitPreferences({ fontSize: value })}
          onLineHeight={(value) => setLineHeight(value)}
          onLineHeightCommit={(value) => commitPreferences({ lineHeight: value })}
          onMargin={(value) => setMargin(value)}
          onMarginCommit={(value) => commitPreferences({ margin: value })}
          onColumns={(value) => {
            setColumns(value);
            commitPreferences({ columns: value });
          }}
        />

        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Download ebook"
          className="hidden sm:inline-flex"
          onPress={() => download(file)}
        >
          <DownloadIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Close ebook reader"
          onPress={requestClose}
        >
          <CloseIcon className="size-5" />
        </Button>
      </header>

      <div className="flex min-h-0 flex-1">
        {isDesktop && sidebarOpen ? (
          <aside
            data-epub-sidebar
            className="reader-chrome hidden w-72 shrink-0 border-r lg:block xl:w-80"
          >
            {navigation}
          </aside>
        ) : null}

        <main
          data-epub-canvas
          className="relative min-w-0 flex-1 overflow-hidden bg-(--reader-canvas)"
        >
          {!ready && !error && !closing ? (
            <div className="absolute inset-0 z-10 grid place-items-center bg-(--reader-canvas)/90">
              <div className="text-center">
                <Spinner size="lg" aria-label="Loading ebook" />
                <p className="mt-3 text-xs text-(--reader-muted)">Opening book</p>
              </div>
            </div>
          ) : null}
          {error ? (
            <div className="absolute inset-0 z-10 grid place-items-center p-6 text-center">
              <div className="max-w-lg">
                <p className="font-semibold">Unable to open this ebook</p>
                <p className="mt-2 text-sm text-(--reader-muted)">{error}</p>
              </div>
            </div>
          ) : null}
          <div className="mx-auto h-full min-h-0 w-full max-w-[1680px] px-0 sm:px-2 lg:px-4">
            <div ref={hostRef} className="reader-page h-full min-h-0 w-full overflow-hidden" />
          </div>
        </main>
      </div>

      <footer
        data-epub-footer
        className="reader-chrome z-20 grid h-12 shrink-0 grid-cols-[1fr_auto_1fr] items-center border-t px-2 sm:px-4"
      >
        <Button
          size="sm"
          variant="ghost"
          aria-label="Previous page"
          className="justify-self-start"
          isDisabled={!ready}
          onPress={() => navigate("previous")}
        >
          <LeftIcon className="size-4" />
          <span className="hidden sm:inline">Previous</span>
        </Button>
        <div className="max-w-[52vw] text-center text-[10px] tabular-nums text-(--reader-muted) sm:text-[11px]">
          <p className="truncate">
            {chapter ? `${chapter} · ` : ""}
            {locationLabel(location, progress)}
          </p>
        </div>
        <Button
          size="sm"
          variant="ghost"
          aria-label="Next page"
          className="justify-self-end"
          isDisabled={!ready}
          onPress={() => navigate("next")}
        >
          <span className="hidden sm:inline">Next</span>
          <RightIcon className="size-4" />
        </Button>
      </footer>

      {!isDesktop ? (
        <Drawer state={drawerState}>
          <Drawer.Backdrop variant="blur">
            <Drawer.Content placement="left" className="w-[min(88vw,22rem)]">
              <Drawer.Dialog data-reader-theme={theme} className="reader-chrome">
                <Drawer.Header className="border-b border-(--reader-border)">
                  <Drawer.Heading>Book navigation</Drawer.Heading>
                  <Drawer.CloseTrigger />
                </Drawer.Header>
                <Drawer.Body className="p-0">{navigation}</Drawer.Body>
              </Drawer.Dialog>
            </Drawer.Content>
          </Drawer.Backdrop>
        </Drawer>
      ) : null}
    </div>
  );
}

function EpubNavigation({
  file,
  toc,
  activeChapter,
  onNavigate,
}: {
  file: FileEntry;
  toc: TocItem[];
  activeChapter?: string;
  onNavigate: (href: string) => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="border-b border-(--reader-border) px-5 py-4">
        <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-(--reader-muted)">
          Library
        </p>
        <p className="mt-1 truncate text-sm font-semibold">{file.name}</p>
      </div>
      <Tabs defaultSelectedKey="contents" className="flex min-h-0 flex-1 flex-col px-3 pt-3">
        <Tabs.ListContainer>
          <Tabs.List aria-label="Ebook navigation" className="w-full">
            <Tabs.Tab id="contents" className="flex-1">
              Contents
            </Tabs.Tab>
            <Tabs.Tab id="details" className="flex-1">
              Details
            </Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>
        <Tabs.Panel id="contents" className="min-h-0 flex-1 overflow-y-auto py-3">
          {toc.length ? (
            <ListBox
              aria-label="Table of contents"
              selectionMode="none"
              className="w-full gap-1 p-0"
              onAction={(key) => {
                const item = toc.find((candidate) => candidate.id === String(key));
                if (item) onNavigate(item.href);
              }}
            >
              {toc.map((item) => (
                <ListBox.Item
                  key={item.id}
                  id={item.id}
                  textValue={item.label}
                  className={cn(
                    "text-sm",
                    activeChapter === item.label && "bg-default/60 font-medium",
                  )}
                  style={{ paddingInlineStart: `${12 + item.depth * 18}px` }}
                >
                  {item.label}
                </ListBox.Item>
              ))}
            </ListBox>
          ) : (
            <p className="px-2 py-4 text-xs text-(--reader-muted)">
              This book has no table of contents.
            </p>
          )}
        </Tabs.Panel>
        <Tabs.Panel id="details" className="space-y-4 overflow-y-auto px-2 py-5 text-sm">
          <Detail label="Title" value={file.name} />
          <Detail label="Format" value="EPUB" />
          <Detail label="File size" value={formatBytes(file.size || 0)} />
        </Tabs.Panel>
      </Tabs>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-[10px] uppercase tracking-wider text-(--reader-muted)">{label}</p>
      <p className="mt-1 wrap-break-word font-medium">{value}</p>
    </div>
  );
}

function EpubSettings({
  isOpen,
  onOpenChange,
  theme,
  flow,
  font,
  fontSize,
  lineHeight,
  margin,
  columns,
  onTheme,
  onFlow,
  onFont,
  onFontSize,
  onFontSizeCommit,
  onLineHeight,
  onLineHeightCommit,
  onMargin,
  onMarginCommit,
  onColumns,
}: ReaderPreferences & {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  onTheme: (value: string) => void;
  onFlow: (value: string) => void;
  onFont: (value: string) => void;
  onFontSize: (value: number) => void;
  onFontSizeCommit: (value: number) => void;
  onLineHeight: (value: number) => void;
  onLineHeightCommit: (value: number) => void;
  onMargin: (value: number) => void;
  onMarginCommit: (value: number) => void;
  onColumns: (value: number) => void;
}) {
  return (
    <Popover isOpen={isOpen} onOpenChange={onOpenChange}>
      <Button
        size="sm"
        variant="ghost"
        aria-label="Reading settings"
        className="min-w-9 px-2 font-serif text-base"
      >
        Aa
      </Button>
      <Popover.Content placement="bottom end" offset={8} className="w-[min(92vw,22rem)]">
        <Popover.Dialog className="p-0">
          <div className="flex items-start justify-between gap-3 border-b border-border px-4 py-3">
            <div>
              <Popover.Heading className="text-sm font-semibold">
                Reading appearance
              </Popover.Heading>
              <p className="mt-0.5 text-xs text-muted">Typography and page layout</p>
            </div>
            <Button size="sm" variant="ghost" onPress={() => onOpenChange(false)}>
              Done
            </Button>
          </div>
          <div className="max-h-[min(72vh,36rem)] space-y-5 overflow-y-auto p-4">
            <SettingButtons
              label="Theme"
              value={theme}
              options={[
                ["White", "white"],
                ["Paper", "paper"],
                ["Gray", "gray"],
                ["Night", "night"],
              ]}
              onChange={onTheme}
            />
            <SettingButtons
              label="Font"
              value={font}
              options={[
                ["Original", "publisher"],
                ["Serif", "serif"],
                ["Sans", "sans"],
              ]}
              onChange={onFont}
            />
            <SettingSlider
              label="Text size"
              value={fontSize}
              min={80}
              max={180}
              step={5}
              output={`${fontSize}%`}
              onChange={onFontSize}
              onCommit={onFontSizeCommit}
            />
            <SettingSlider
              label="Line spacing"
              value={lineHeight}
              min={1.2}
              max={2}
              step={0.05}
              output={lineHeight.toFixed(2)}
              onChange={onLineHeight}
              onCommit={onLineHeightCommit}
            />
            <SettingSlider
              label="Page margins"
              value={margin}
              min={16}
              max={96}
              step={4}
              output={`${margin}px`}
              onChange={onMargin}
              onCommit={onMarginCommit}
            />
            <SettingButtons
              label="Layout"
              value={flow}
              options={[
                ["Pages", "paginated"],
                ["Scroll", "scrolled"],
              ]}
              onChange={onFlow}
            />
            <SettingButtons
              label="Columns"
              value={String(columns)}
              options={[
                ["Single", "1"],
                ["Double", "2"],
              ]}
              onChange={(value) => onColumns(Number(value))}
            />
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}

function SettingButtons({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<[string, string]>;
  onChange: (value: string) => void;
}) {
  return (
    <div>
      <p className="mb-2 text-xs font-medium">{label}</p>
      <div className="grid grid-cols-2 gap-1.5">
        {options.map(([name, option]) => (
          <Button
            key={option}
            size="sm"
            variant={value === option ? "primary" : "secondary"}
            onPress={() => onChange(option)}
          >
            {name}
          </Button>
        ))}
      </div>
    </div>
  );
}

function SettingSlider({
  label,
  value,
  min,
  max,
  step,
  output,
  onChange,
  onCommit,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  output: string;
  onChange: (value: number) => void;
  onCommit: (value: number) => void;
}) {
  return (
    <Slider
      aria-label={label}
      value={value}
      minValue={min}
      maxValue={max}
      step={step}
      onChange={(next) => onChange(Number(next))}
      onChangeEnd={(next) => onCommit(Number(next))}
    >
      <div className="mb-2 flex justify-between text-xs">
        <span>{label}</span>
        <span className="tabular-nums text-muted">{output}</span>
      </div>
      <Slider.Track>
        <Slider.Fill />
        <Slider.Thumb />
      </Slider.Track>
    </Slider>
  );
}

function flattenToc(
  items: Array<{ label: string; href: string; subitems?: unknown[] }>,
  depth = 0,
  parent = "root",
): TocItem[] {
  return items.flatMap((item) => {
    const id = `${parent}:${item.href}:${item.label}`;
    return [
      { id, label: item.label, href: item.href, depth },
      ...flattenToc(
        (item.subitems || []) as Array<{ label: string; href: string; subitems?: unknown[] }>,
        depth + 1,
        id,
      ),
    ];
  });
}

function locationLabel(location: Location, progress: number) {
  if (location.current !== undefined && location.total)
    return `Page ${location.current + 1} of ${location.total}`;
  return `${Math.round(progress * 100)}%`;
}

function numberValue(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function download(file: FileEntry) {
  const anchor = document.createElement("a");
  anchor.href = `/api/v1/files/${encodeURIComponent(file.id)}/content`;
  anchor.download = file.name;
  anchor.click();
}

function formatBytes(value: number) {
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function isEditableTarget(target: EventTarget | null) {
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    (target instanceof HTMLElement && target.isContentEditable)
  );
}

function nextAnimationFrame() {
  return new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
}

function useMediaQuery(query: string) {
  const [matches, setMatches] = useState(() =>
    typeof window === "undefined" ? false : window.matchMedia(query).matches,
  );
  useEffect(() => {
    const media = window.matchMedia(query);
    const update = () => setMatches(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [query]);
  return matches;
}
