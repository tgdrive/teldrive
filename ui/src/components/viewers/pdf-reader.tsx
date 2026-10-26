import { localEngine } from "@embedpdf/engine";
import { interactionPlugin } from "@embedpdf/react/interaction";
import { LinkLayer, linkPlugin } from "@embedpdf/react/link";
import { outlinePlugin } from "@embedpdf/react/outline";
import { RenderLayer, renderPlugin } from "@embedpdf/react/render";
import {
  createCapabilityToken,
  DocumentGate,
  type InitialDocument,
  type LockedDocumentInfo,
  useDocuments,
  Viewer,
} from "@embedpdf/react/runtime";
import { SearchLayer, searchPlugin, useSearch, useSearchState } from "@embedpdf/react/search";
import {
  copySelection,
  SelectionClipboard,
  SelectionHandles,
  SelectionLayer,
  SelectionMenu,
  selectionPlugin,
  useSelection,
} from "@embedpdf/react/selection";
import {
  Scrollbar,
  Stage,
  type StageCapability,
  stagePlugin,
  useScrollMetrics,
  useStage,
  useStageState,
} from "@embedpdf/react/stage";
import { Button, cn, Dropdown, Form, Input, Link, Popover, Spinner, Tabs } from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import type { FileEntry } from "@/api/types";
import { PdfBrightnessSlider } from "@/components/pdf-brightness-slider";
import { PreviewExpandButton } from "@/components/preview-presentation";
import { loadDocumentContent } from "@/features/files/document-cache";
import { useDocumentSettings } from "@/lib/document-settings";
import { useFormatBytes } from "@/lib/format";
import { useAppLocale } from "@/lib/locale";
import { useTheme } from "@/lib/theme";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import RotateIcon from "~icons/gravity-ui/arrow-rotate-right";
import SidebarIcon from "~icons/gravity-ui/bars";
import LeftIcon from "~icons/gravity-ui/chevron-left";
import RightIcon from "~icons/gravity-ui/chevron-right";
import CopyIcon from "~icons/gravity-ui/copy";
import SearchIcon from "~icons/gravity-ui/magnifier";
import ZoomOutIcon from "~icons/gravity-ui/magnifier-minus";
import ZoomInIcon from "~icons/gravity-ui/magnifier-plus";
import SunIcon from "~icons/gravity-ui/sun";
import CloseIcon from "~icons/gravity-ui/xmark";
import { PdfOutline } from "./pdf-outline";

type PdfReaderProps = {
  file: FileEntry;
  url: string;
  downloadUrl?: string;
  onClose: () => void;
};

const Thumbnails = createCapabilityToken<StageCapability>("reader-thumbnails");
const plugins = [
  stagePlugin({ zoom: { mode: "fit-width" }, padding: 24, gap: { px: 20 }, responsive: [] }),
  stagePlugin({
    id: "reader-thumbnails",
    token: Thumbnails,
    interaction: false,
    zoomGestures: false,
    zoom: { pageWidth: 116 },
    fitAlign: { x: "center", y: "start" },
    gap: { px: 16 },
    padding: 16,
    pageFrame: { bottom: 24 },
    responsive: [],
  }),
  renderPlugin(),
  interactionPlugin(),
  selectionPlugin(),
  searchPlugin(),
  linkPlugin(),
  outlinePlugin(),
];

// No annotation, form, page-edit or script plugins. The engine also refuses
// document mutations rather than relying only on hiding their controls.
const scope = [
  "doc.open",
  "doc.render",
  "doc.text.select",
  "doc.text.copy",
  "doc.text.search",
  "doc.annotate.read",
  "doc.forms.read",
  "doc.download",
];
const createEngine = () => localEngine();

export function PdfReader({ file, url, downloadUrl = url, onClose }: PdfReaderProps) {
  const { t } = useAppLocale();
  const { resolvedTheme } = useTheme();
  const { pdfBrightness } = useDocumentSettings();
  const formatBytes = useFormatBytes();
  const [attempt, setAttempt] = useState(0);
  const initialDocuments = useMemo<InitialDocument[]>(
    () => [
      {
        name: file.name,
        source: async (signal) => {
          const content = await loadDocumentContent(file, url, signal);
          return {
            kind: "bytes",
            id: file.id,
            bytes: new Uint8Array(await content.arrayBuffer()),
          };
        },
      },
    ],
    [url, file.id, file.name],
  );
  const retry = () => setAttempt((value) => value + 1);

  return (
    <div
      data-pdf-reader
      data-reader-theme={resolvedTheme}
      data-pdf-brightness={pdfBrightness}
      style={{ "--pdf-brightness": pdfBrightness / 100 } as CSSProperties}
      className="pdf-reader flex h-full min-h-0 flex-col bg-background text-foreground"
    >
      <header
        data-preview-chrome="header"
        data-pdf-header
        className="flex h-16 shrink-0 items-center gap-3 border-b border-border bg-surface px-3 sm:px-5"
      >
        <Button isIconOnly variant="ghost" size="sm" aria-label={t("pdf.close")} onPress={onClose}>
          <CloseIcon className="size-5" />
        </Button>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{file.name}</p>
          <p className="text-xs text-muted">PDF · {formatBytes(file.size || 0)}</p>
        </div>
        <PreviewExpandButton />
        <Popover>
          <Button isIconOnly size="sm" variant="ghost" aria-label={t("pdf.brightness")}>
            <SunIcon className="size-4" />
          </Button>
          <Popover.Content placement="bottom end" className="w-64">
            <Popover.Dialog>
              <Popover.Heading className="mb-4 text-sm font-medium">
                {t("pdf.brightnessAria")}
              </Popover.Heading>
              <PdfBrightnessSlider />
            </Popover.Dialog>
          </Popover.Content>
        </Popover>
        <Link
          href={downloadUrl}
          download={file.name}
          aria-label={t("pdf.downloadOriginal")}
          className={buttonVariants({ variant: "ghost", size: "sm", isIconOnly: true })}
        >
          <DownloadIcon className="size-4" />
        </Link>
      </header>
      <Viewer
        key={attempt}
        engine={createEngine}
        plugins={plugins}
        scope={scope}
        initialDocuments={initialDocuments}
        fallback={<PdfLoading />}
        renderError={() => <PdfFailure onRetry={retry} />}
      >
        <DocumentGate
          fallback={<PdfLoading />}
          locked={(document) => <PdfPassword document={document} />}
          error={(document) => <PdfFailure detail={document.error.message} onRetry={retry} />}
        >
          <PdfWorkspace onClose={onClose} />
        </DocumentGate>
      </Viewer>
    </div>
  );
}

function PdfWorkspace({ onClose }: { onClose: () => void }) {
  const { t } = useAppLocale();
  const stage = useStage();
  const { currentPageIndex, pageCount, zoomLevel } = useStageState();
  const [sidebar, setSidebar] = useState(false);
  const [sidebarTab, setSidebarTab] = useState("pages");
  const [find, setFind] = useState(false);
  const [page, setPage] = useState("");
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => setPage(String(currentPageIndex + 1)), [currentPageIndex]);

  useEffect(() => {
    function keydown(event: KeyboardEvent) {
      if (event.defaultPrevented) return;
      const target = event.target;
      if (target instanceof Element && target.closest('[data-slot="popover-dialog"]')) return;
      const editing =
        target instanceof HTMLElement &&
        (target.matches("input,textarea,select") || target.isContentEditable);
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
        event.preventDefault();
        setFind(true);
      } else if (event.key === "Escape") {
        event.preventDefault();
        if (find) setFind(false);
        else if (sidebar) setSidebar(false);
        else onClose();
      } else if (!editing && !event.ctrlKey && !event.metaKey && !event.altKey) {
        if (event.key === "ArrowRight" || event.key === "PageDown") {
          event.preventDefault();
          stage.nextPage();
        } else if (event.key === "ArrowLeft" || event.key === "PageUp") {
          event.preventDefault();
          stage.previousPage();
        } else if (event.key === "+" || event.key === "=") {
          event.preventDefault();
          stage.zoomIn();
        } else if (event.key === "-") {
          event.preventDefault();
          stage.zoomOut();
        }
      }
    }
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  }, [stage, find, sidebar, onClose]);

  function goToPage() {
    const value = Number(page);
    if (Number.isInteger(value) && value >= 1 && value <= pageCount) stage.goToPage(value - 1);
    else setPage(String(currentPageIndex + 1));
  }

  return (
    <div ref={root} className="flex min-h-0 flex-1 flex-col">
      <SelectionClipboard target={root} />
      <div
        data-pdf-toolbar
        data-preview-chrome="pdf-toolbar"
        className="flex h-12 shrink-0 items-center justify-between gap-1 border-b border-border bg-surface px-2 sm:px-4"
      >
        <div className="flex items-center gap-1">
          <Button
            isIconOnly
            size="sm"
            variant={sidebar ? "secondary" : "ghost"}
            aria-label={t("pdf.toggleSidebar")}
            aria-pressed={sidebar}
            onPress={() => setSidebar(!sidebar)}
          >
            <SidebarIcon />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant={find ? "secondary" : "ghost"}
            aria-label={t("pdf.search")}
            aria-expanded={find}
            onPress={() => setFind(!find)}
          >
            <SearchIcon />
          </Button>
        </div>
        <div className="flex items-center gap-1 text-xs tabular-nums">
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.prevPage")}
            isDisabled={currentPageIndex === 0}
            onPress={() => stage.previousPage()}
          >
            <LeftIcon />
          </Button>
          <Input
            aria-label={t("pdf.pageNumber")}
            inputMode="numeric"
            value={page}
            onChange={(event) => setPage(event.target.value)}
            onBlur={goToPage}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                goToPage();
                event.currentTarget.blur();
              }
            }}
            className="h-7 w-10 rounded-md border border-field-border bg-field-background text-center text-foreground outline-none focus:border-focus"
          />
          <span className="text-muted">/ {pageCount}</span>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.nextPage")}
            isDisabled={currentPageIndex >= pageCount - 1}
            onPress={() => stage.nextPage()}
          >
            <RightIcon />
          </Button>
        </div>
        <div className="flex items-center gap-1">
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.zoomOut")}
            isDisabled={zoomLevel <= 0.1}
            onPress={() => stage.zoomOut()}
          >
            <ZoomOutIcon />
          </Button>
          <Dropdown>
            <Button
              size="sm"
              variant="ghost"
              aria-label={t("pdf.zoomOptions")}
              className="tabular-nums"
            >
              {Math.round(zoomLevel * 100)}%
            </Button>
            <Dropdown.Popover>
              <Dropdown.Menu
                aria-label={t("pdf.zoomOptions")}
                onAction={(key) => {
                  const value = String(key);
                  if (value === "width") stage.fitWidth();
                  else if (value === "page") stage.fitPage();
                  else if (value === "rotate") stage.rotateViewBy(90);
                  else stage.zoomTo(Number(value));
                }}
              >
                <Dropdown.Item id="width">{t("pdf.fitWidth")}</Dropdown.Item>
                <Dropdown.Item id="page">{t("pdf.fitPage")}</Dropdown.Item>
                {[0.5, 1, 1.5, 2, 3].map((value) => (
                  <Dropdown.Item key={value} id={String(value)} textValue={`${value * 100}%`}>
                    {value * 100}%
                  </Dropdown.Item>
                ))}
                <Dropdown.Item id="rotate">{t("pdf.rotate")}</Dropdown.Item>
              </Dropdown.Menu>
            </Dropdown.Popover>
          </Dropdown>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.zoomIn")}
            isDisabled={zoomLevel >= 10}
            onPress={() => stage.zoomIn()}
          >
            <ZoomInIcon />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            className="hidden lg:flex"
            aria-label={t("pdf.rotate")}
            onPress={() => stage.rotateViewBy(90)}
          >
            <RotateIcon />
          </Button>
        </div>
      </div>
      {find ? <PdfSearch onClose={() => setFind(false)} /> : null}
      <div className="relative flex min-h-0 flex-1">
        {sidebar ? (
          <aside
            aria-label={t("pdf.docNav")}
            className={cn(
              "absolute inset-y-0 left-0 z-20 flex flex-col border-r border-border bg-surface shadow-lg sm:relative sm:shrink-0 sm:shadow-none",
              sidebarTab === "outline" ? "w-64" : "w-40",
            )}
          >
            <div className="flex h-11 items-center justify-between border-b border-border px-3 text-xs font-medium">
              {t("pdf.docNav")}
              <Button
                isIconOnly
                size="sm"
                variant="ghost"
                aria-label={t("pdf.toggleSidebar")}
                onPress={() => setSidebar(false)}
              >
                <CloseIcon className="size-3" />
              </Button>
            </div>
            <Tabs
              selectedKey={sidebarTab}
              onSelectionChange={(key) => setSidebarTab(String(key))}
              className="flex min-h-0 flex-1 flex-col"
            >
              <Tabs.ListContainer>
                <Tabs.List aria-label={t("pdf.sidebarAria")} className="w-full">
                  <Tabs.Tab id="pages" className="flex-1 text-xs">
                    {t("pdf.tabPages")}
                  </Tabs.Tab>
                  <Tabs.Tab id="outline" className="flex-1 text-xs">
                    {t("pdf.tabOutline")}
                  </Tabs.Tab>
                </Tabs.List>
              </Tabs.ListContainer>
              <Tabs.Panel id="pages" className="min-h-0 flex-1">
                <ThumbnailSidebar
                  onNavigate={() => {
                    if (window.innerWidth < 640) setSidebar(false);
                  }}
                />
              </Tabs.Panel>
              <Tabs.Panel id="outline" className="min-h-0 flex-1 overflow-y-auto">
                {sidebarTab === "outline" ? (
                  <PdfOutline
                    onNavigate={() => {
                      if (window.innerWidth < 640) setSidebar(false);
                    }}
                  />
                ) : null}
              </Tabs.Panel>
            </Tabs>
          </aside>
        ) : null}
        <Stage
          className="pdf-stage min-h-0 min-w-0 flex-1"
          overlay={
            <>
              <Scrollbar axis="y" />
              <Scrollbar axis="x" />
              <SelectionHandles />
              <PdfSelectionMenu />
            </>
          }
        >
          {(page) => (
            <div data-pdf-page={page.pageIndex + 1} className="absolute inset-0">
              <div data-pdf-page-content className="absolute inset-0">
                <RenderLayer />
              </div>
              <LinkLayer />
              <SearchLayer />
              <SelectionLayer />
            </div>
          )}
        </Stage>
      </div>
    </div>
  );
}

function ThumbnailSidebar({ onNavigate }: { onNavigate: () => void }) {
  const { t } = useAppLocale();
  const stage = useStage();
  const thumbs = useStage(Thumbnails);
  const { currentPageIndex } = useStageState();
  const { clientWidth, clientHeight } = useScrollMetrics(Thumbnails);
  useEffect(() => {
    // Tab panels mount before their stage has a measured viewport. Reveal again
    // once it is laid out, rather than letting the first resize reset to page 1.
    if (!clientWidth || !clientHeight) return;
    thumbs.reveal(currentPageIndex);
  }, [thumbs, currentPageIndex, clientWidth, clientHeight]);
  return (
    <Stage
      token={Thumbnails}
      className="h-full"
      pageChrome={(page) => (
        <span className="absolute inset-x-0 bottom-0 text-center text-[11px] leading-6 text-muted">
          {page.pageIndex + 1}
        </span>
      )}
    >
      {(page) => (
        <Button
          type="button"
          variant="ghost"
          aria-label={t("pdf.goToPage", { page: page.pageIndex + 1 })}
          aria-current={page.pageIndex === currentPageIndex ? "page" : undefined}
          onPress={() => {
            stage.goToPage(page.ref);
            onNavigate();
          }}
          className={cn(
            "absolute inset-0 h-full w-full min-w-0 rounded-none p-0 outline-none",
            page.pageIndex === currentPageIndex
              ? "ring-2 ring-accent"
              : "ring-1 ring-border hover:ring-accent",
          )}
        >
          <div data-pdf-page-content className="absolute inset-0">
            <RenderLayer tiles={false} />
          </div>
        </Button>
      )}
    </Stage>
  );
}

function PdfSearch({ onClose }: { onClose: () => void }) {
  const { t } = useAppLocale();
  const search = useSearch();
  const { hitCount, activeHitIndex, status, error } = useSearchState();
  const [text, setText] = useState("");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    input.current?.focus();
  }, []);
  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void search.search({ text }).catch(() => {});
    }, 180);
    return () => window.clearTimeout(timeout);
  }, [search, text]);
  useEffect(() => () => search.clear(), [search]);
  return (
    <div
      data-pdf-findbar
      data-preview-chrome="pdf-findbar"
      className="flex min-h-12 shrink-0 items-center gap-2 border-b border-border bg-surface px-3 py-2 sm:px-5"
    >
      <SearchIcon className="size-4 shrink-0 text-muted" />
      <Input
        ref={input}
        aria-label={t("pdf.find")}
        placeholder={t("pdf.findPlaceholder")}
        value={text}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            if (event.shiftKey) search.previousHit();
            else search.nextHit();
          }
        }}
        className="h-8 min-w-0 flex-1 bg-transparent text-sm outline-none"
      />
      <output aria-live="polite" className="shrink-0 text-xs tabular-nums text-muted">
        {status === "searching" ? (
          <Spinner size="sm" />
        ) : hitCount ? (
          `${activeHitIndex + 1} / ${hitCount}`
        ) : text ? (
          t("pdf.noMatches")
        ) : (
          ""
        )}
      </output>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        isDisabled={!hitCount}
        aria-label={t("pdf.prevResult")}
        onPress={() => search.previousHit()}
      >
        <LeftIcon />
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        isDisabled={!hitCount}
        aria-label={t("pdf.nextResult")}
        onPress={() => search.nextHit()}
      >
        <RightIcon />
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={t("pdf.closeSearch")}
        onPress={onClose}
      >
        <CloseIcon />
      </Button>
      {error ? (
        <span role="alert" className="text-xs text-danger">
          {error.message}
        </span>
      ) : null}
    </div>
  );
}

function PdfSelectionMenu() {
  const { t } = useAppLocale();
  const selection = useSelection();
  const [error, setError] = useState("");
  return (
    <SelectionMenu>
      <div className="rounded-lg border border-border bg-overlay p-1 shadow-lg">
        {/* EmbedPDF isolates pointerdown on this overlay with a native listener.
            A click handler still bubbles to React; HeroUI's onPress cannot start
             its pointer gesture here. Use onClick on the shared button. */}
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            void copySelection(selection).catch((cause) =>
              setError(cause instanceof Error ? cause.message : t("manager.copyFailed")),
            );
          }}
        >
          <CopyIcon className="size-4" />
          {t("share.copy")}
        </Button>
        {error ? (
          <p role="alert" className="px-2 text-xs text-danger">
            {error}
          </p>
        ) : null}
      </div>
    </SelectionMenu>
  );
}

function PdfLoading() {
  const { t } = useAppLocale();
  return (
    <div
      role="status"
      className="flex min-h-0 flex-1 items-center justify-center gap-3 p-6 text-sm text-muted"
    >
      <Spinner size="sm" />
      {t("preview.loadingPdf")}
    </div>
  );
}

function PdfFailure({ detail, onRetry }: { detail?: string; onRetry: () => void }) {
  const { t } = useAppLocale();
  return (
    <div
      role="alert"
      className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center text-sm"
    >
      <p>{t("pdf.openFailed")}</p>
      {detail ? <p className="max-w-md break-words text-xs text-muted">{detail}</p> : null}
      <Button size="sm" variant="secondary" onPress={onRetry}>
        {t("pdf.retry")}
      </Button>
    </div>
  );
}

function PdfPassword({ document }: { document: LockedDocumentInfo }) {
  const { t } = useAppLocale();
  const documents = useDocuments();
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Form
      className="m-auto flex w-full max-w-sm flex-col gap-4 p-6"
      onSubmit={(event) => {
        event.preventDefault();
        if (pending) return;
        setPending(true);
        void documents
          .unlock(document.id, { password })
          .catch(() => setFailed(true))
          .finally(() => setPending(false));
      }}
    >
      <h2 className="text-base font-semibold">{t("pdf.protected")}</h2>
      <p className="text-sm text-muted">
        {t(document.passwordProvided || failed ? "pdf.badPassword" : "pdf.needPassword")}
      </p>
      <Input
        type="password"
        autoComplete="off"
        aria-label={t("pdf.passwordAria")}
        value={password}
        onChange={(event) => setPassword(event.target.value)}
        className="h-10 rounded-lg border border-field-border bg-field-background px-3 text-sm outline-none focus:border-focus"
      />
      <Button type="submit" variant="primary" isDisabled={pending}>
        {pending ? <Spinner size="sm" /> : t("pdf.unlock")}
      </Button>
    </Form>
  );
}
