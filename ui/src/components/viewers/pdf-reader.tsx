import {
  AnnotationEditorParamsType,
  AnnotationEditorType,
  AnnotationMode,
  GlobalWorkerOptions,
  getDocument,
  PasswordResponses,
  type PDFDocumentProxy,
} from "pdfjs-dist";
import workerSrc from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import {
  EventBus,
  FindState,
  LinkTarget,
  PDFFindController,
  PDFLinkService,
  PDFViewer,
} from "pdfjs-dist/web/pdf_viewer.mjs";
import "pdfjs-dist/web/pdf_viewer.css";

import {
  Button,
  cn,
  Drawer,
  InputGroup,
  Popover,
  Slider,
  Spinner,
  Tabs,
  useOverlayState,
} from "@heroui/react";
import {
  memo,
  type KeyboardEvent as ReactKeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { GridList, GridListItem, ListLayout, Virtualizer } from "react-aria-components";
import type { FileEntry } from "@/api/types";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { copyText } from "@/features/files/download";
import { useFormatBytes } from "@/lib/format";
import { useAppLocale } from "@/lib/locale";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import UndoIcon from "~icons/gravity-ui/arrow-rotate-left";
import RotateIcon from "~icons/gravity-ui/arrow-rotate-right";
import MenuIcon from "~icons/gravity-ui/bars";
import BookmarkIcon from "~icons/gravity-ui/bookmark";
import BrushIcon from "~icons/gravity-ui/brush";
import DownIcon from "~icons/gravity-ui/chevron-down";
import LeftIcon from "~icons/gravity-ui/chevron-left";
import RightIcon from "~icons/gravity-ui/chevron-right";
import EllipsisIcon from "~icons/gravity-ui/ellipsis";
import SaveIcon from "~icons/gravity-ui/floppy-disk";
import HandIcon from "~icons/gravity-ui/hand";
import SearchIcon from "~icons/gravity-ui/magnifier";
import ZoomOutIcon from "~icons/gravity-ui/magnifier-minus";
import ZoomInIcon from "~icons/gravity-ui/magnifier-plus";
import PencilIcon from "~icons/gravity-ui/pencil";
import TextIcon from "~icons/gravity-ui/text";
import CloseIcon from "~icons/gravity-ui/xmark";

GlobalWorkerOptions.workerSrc = workerSrc;

type PdfReaderProps = {
  file: FileEntry;
  url: string;
  onClose: () => void;
};

type SidebarTab = "thumbnails" | "outline";
type AnnotationTool = "select" | "pan" | "highlight" | "text" | "ink";
type EditorTool = "highlight" | "text" | "ink";
type ToolSettings = { color: string; size: number; thickness: number; opacity: number };
type EditorSettings = Record<EditorTool, ToolSettings>;
// PDF.js supports this callback, but its generated types declare it as null only.
type EditableAnnotationStorage = Omit<PDFDocumentProxy["annotationStorage"], "onSetModified"> & {
  onSetModified: (() => void) | null;
};
type OutlineItem = {
  title: string;
  dest: string | unknown[] | null;
  url?: string | null;
  items?: OutlineItem[];
};

type PdfRuntime = {
  eventBus: EventBus;
  linkService: PDFLinkService;
  findController: PDFFindController;
  viewer: PDFViewer;
  document: PDFDocumentProxy;
};

type FindCount = { current: number; total: number };
type PasswordChallenge = {
  incorrect: boolean;
  submit: (password: string) => void;
};

const HIGHLIGHT_COLORS = ["#facc15", "#4ade80", "#60a5fa", "#f472b6"] as const;
const ANNOTATION_COLORS = ["#000000", "#e11d48", ...HIGHLIGHT_COLORS] as const;
const PDFJS_HIGHLIGHT_COLORS = "yellow=#facc15,green=#4ade80,blue=#60a5fa,pink=#f472b6";

// Bound retained canvases without sacrificing visible-area HiDPI detail.
const MOBILE_CANVAS_PIXELS = 6_291_456;
const DESKTOP_CANVAS_PIXELS = 12_582_912;
// A bounded full-page canvas keeps memory controlled. Its limit must still
// accommodate at least the visible viewport at native device resolution;
// PDF.js draws the remainder of zoomed/oversized pages in a detail canvas.
function pdfCanvasPixelBudget(container: HTMLElement) {
  const pixelRatio = Math.max(1, window.devicePixelRatio || 1);
  const visiblePixels = Math.ceil(container.clientWidth * container.clientHeight * pixelRatio ** 2);
  const minimum = window.matchMedia("(pointer: coarse)").matches
    ? MOBILE_CANVAS_PIXELS
    : DESKTOP_CANVAS_PIXELS;
  return Math.max(minimum, visiblePixels);
}
// PDF.js tracks visible pages on scroll. Avoid starting new canvas paints
// while moving; its trailing 100 ms scroll update resumes the render queue.
class ScrollAwarePDFViewer extends PDFViewer {
  isScrolling: () => boolean = () => false;

  override forceRendering(visiblePages?: Parameters<PDFViewer["forceRendering"]>[0]) {
    if (this.isScrolling()) return false;
    return super.forceRendering(visiblePages);
  }
}
function defaultEditorSettings(): EditorSettings {
  return {
    highlight: { color: HIGHLIGHT_COLORS[0], size: 12, thickness: 12, opacity: 1 },
    text: { color: "#000000", size: 12, thickness: 2, opacity: 1 },
    ink: { color: "#000000", size: 12, thickness: 2, opacity: 1 },
  };
}

export function PdfReader({ file, url, onClose }: PdfReaderProps) {
  const readerRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const viewerRef = useRef<HTMLDivElement>(null);
  const runtimeRef = useRef<PdfRuntime | null>(null);
  const lastDocumentScrollRef = useRef(0);
  const closeRef = useRef(onClose);
  const drawerState = useOverlayState();
  const [toolsOpen, setToolsOpen] = useState(false);

  const initialPage = 1;
  const initialScaleValue = "page-width";
  const initialRotation = 0;
  const initialSidebarOpen = true;
  const initialSidebarTab: SidebarTab = "thumbnails";

  const [document, setDocument] = useState<PDFDocumentProxy>();
  const [outline, setOutline] = useState<OutlineItem[]>([]);
  const [ready, setReady] = useState(false);
  const readyRef = useRef(false);
  const [error, setError] = useState<string>();
  const [loadingProgress, setLoadingProgress] = useState<number>();
  const [pageNumber, setPageNumber] = useState(initialPage);
  const [pageDraft, setPageDraft] = useState(String(initialPage));
  const pageDraftRef = useRef(String(initialPage));
  const [numPages, setNumPages] = useState(0);
  const [scale, setScale] = useState(1);
  const [scaleValue, setScaleValue] = useState(initialScaleValue);
  const [brightness, setBrightness] = useState(100);
  const [_rotation, setRotation] = useState(initialRotation);
  const [sidebarOpen, setSidebarOpen] = useState(initialSidebarOpen);
  const [sidebarTab, setSidebarTab] = useState<SidebarTab>(initialSidebarTab);
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selectionActionsVisibleRef = useRef(false);
  const [selectionAction, setSelectionAction] = useState<{
    text: string;
    x: number;
    y: number;
  } | null>(null);
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [wholeWord, setWholeWord] = useState(false);
  const [findCount, setFindCount] = useState<FindCount>({ current: 0, total: 0 });
  const [findState, setFindState] = useState<number>(FindState.FOUND);
  const [annotationTool, setAnnotationTool] = useState<AnnotationTool>("select");
  const annotationToolRef = useRef<AnnotationTool>("select");
  const [editorSettings, setEditorSettings] = useState(defaultEditorSettings);
  const editorSettingsRef = useRef(editorSettings);
  const pendingToolRef = useRef<{ tool: EditorTool; settings: ToolSettings } | null>(null);
  const [toolSwitching, setToolSwitching] = useState(false);
  const [canAnnotate, setCanAnnotate] = useState(false);
  const [selectedEditorTool, setSelectedEditorTool] = useState<EditorTool>();
  const selectedEditorRef = useRef(false);
  const lastParamsToolRef = useRef<EditorTool | undefined>(undefined);
  const [canUndo, setCanUndo] = useState(false);
  const [canRedo, setCanRedo] = useState(false);
  const [dirty, setDirty] = useState(false);
  const dirtyRef = useRef(false);
  const editRevisionRef = useRef(0);
  const [closePrompt, setClosePrompt] = useState(false);
  const [saveError, setSaveError] = useState<string>();
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [passwordChallenge, setPasswordChallenge] = useState<PasswordChallenge>();
  const [passwordDraft, setPasswordDraft] = useState("");
  const { t } = useAppLocale();
  const formatBytes = useFormatBytes();

  closeRef.current = onClose;
  const activeEditorTool =
    selectedEditorTool || (isEditorTool(annotationTool) ? annotationTool : undefined);

  useEffect(() => {
    const viewport = containerRef.current;
    if (!viewport) return;
    let frame = 0;
    const showActions = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (annotationToolRef.current !== "select") return;
        const selection = window.getSelection();
        const node = selection?.anchorNode;
        const element = node instanceof Element ? node : node?.parentElement;
        if (
          !selection ||
          selection.isCollapsed ||
          !selection.rangeCount ||
          !element?.closest(".textLayer") ||
          !viewport.contains(element)
        ) {
          selectionActionsVisibleRef.current = false;
          setSelectionAction(null);
          return;
        }
        const text = selection.toString().trim();
        const rect = selection.getRangeAt(0).getBoundingClientRect();
        if (!text || !rect.width || !rect.height) return;
        selectionActionsVisibleRef.current = true;
        setSelectionAction({
          text,
          x: Math.max(48, Math.min(window.innerWidth - 48, rect.left + rect.width / 2)),
          y: Math.max(48, rect.top),
        });
      });
    };
    const hideActions = () => {
      lastDocumentScrollRef.current = performance.now();
      if (selectionActionsVisibleRef.current) {
        selectionActionsVisibleRef.current = false;
        setSelectionAction(null);
      }
    };
    viewport.addEventListener("pointerup", showActions);
    viewport.addEventListener("keyup", showActions);
    viewport.addEventListener("scroll", hideActions, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      viewport.removeEventListener("pointerup", showActions);
      viewport.removeEventListener("keyup", showActions);
      viewport.removeEventListener("scroll", hideActions);
    };
  }, []);
  const markDirty = useCallback(() => {
    editRevisionRef.current += 1;
    dirtyRef.current = true;
    setDirty(true);
  }, []);

  const requestClose = () => {
    if (savingRef.current) return;
    // Commit a currently edited form field or text annotation before checking.
    if (window.document.activeElement instanceof HTMLElement) window.document.activeElement.blur();
    if (dirtyRef.current) setClosePrompt(true);
    else closeRef.current();
  };

  useEffect(() => {
    if (!dirty) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [dirty]);

  useEffect(() => {
    const container = containerRef.current;
    const viewerElement = viewerRef.current;
    if (!container || !viewerElement) return;

    let active = true;
    const viewerAbort = new AbortController();
    let loadingTask: ReturnType<typeof getDocument> | undefined;
    const eventBus = new EventBus();
    const linkService = new PDFLinkService({
      eventBus,
      ignoreDestinationZoom: true,
      externalLinkTarget: LinkTarget.BLANK,
      externalLinkRel: "noopener noreferrer nofollow",
    });
    const findController = new PDFFindController({ eventBus, linkService });
    const viewerOptions = {
      container,
      viewer: viewerElement,
      eventBus,
      linkService,
      findController,
      textLayerMode: 1,
      // Keep the usual system-blue browser text selection instead of
      // PDF.js's transparent selection-rendering overlay.
      enableSelectionRendering: false,
      annotationMode: AnnotationMode.ENABLE_FORMS,
      annotationEditorMode: AnnotationEditorType.NONE,
      annotationEditorHighlightColors: PDFJS_HIGHLIGHT_COLORS,
      removePageBorders: true,
      // Render at full device resolution when it fits. Oversized pages use
      // a bounded base canvas and a sharp detail canvas for the visible region.
      maxCanvasPixels: pdfCanvasPixelBudget(container),
      enableDetailCanvas: true,
      // Use PDF.js's default canvas-update throttle (500 ms) while pages render.
      // Showing every intermediate canvas frame increases main-thread paint work.
      abortSignal: viewerAbort.signal,
    };
    const viewer = new ScrollAwarePDFViewer(viewerOptions);
    viewer.isScrolling = () => performance.now() - lastDocumentScrollRef.current < 80;
    const renderingQueue = viewer.renderingQueue;
    if (renderingQueue) {
      const isHighestPriority = renderingQueue.isHighestPriority.bind(renderingQueue);
      // PDF.js pauses canvas tasks via isHighestPriority at rendering checkpoints.
      // Preserve the partial canvas, then resume using its normal render queue
      // after the trailing scroll update (rather than canceling and redrawing).
      renderingQueue.isHighestPriority = (pageView) =>
        !viewer.isScrolling() && isHighestPriority(pageView);
    }
    linkService.setViewer(viewer);

    let pageUpdateTimeout = 0;
    const commitPageNumber = (next: number) => {
      if (!active) return;
      setPageNumber(next);
      pageDraftRef.current = String(next);
      setPageDraft(String(next));
    };
    const onPageChanging = (event: { pageNumber?: number }) => {
      const next = positiveInt(event.pageNumber, viewer.currentPageNumber || 1);
      window.clearTimeout(pageUpdateTimeout);
      if (performance.now() - lastDocumentScrollRef.current < 120) {
        // Only publish the last visible page while a continuous scroll is in progress.
        pageUpdateTimeout = window.setTimeout(() => commitPageNumber(next), 120);
      } else commitPageNumber(next);
    };
    const onScaleChanging = (event: { scale?: number; presetValue?: string }) => {
      setScale(positiveNumber(event.scale, viewer.currentScale || 1));
      setScaleValue(event.presetValue || viewer.currentScaleValue || "custom");
    };
    const onRotationChanging = (event: { pagesRotation?: number }) => {
      setRotation(normalizedRotation(event.pagesRotation));
    };
    const onFindCount = (event: { matchesCount?: FindCount }) =>
      setFindCount(event.matchesCount || { current: 0, total: 0 });
    const onFindState = (event: { state?: number; matchesCount?: FindCount }) => {
      if (typeof event.state === "number") setFindState(event.state);
      if (event.matchesCount) setFindCount(event.matchesCount);
    };
    const onEditingState = (event: {
      details?: {
        hasSomethingToUndo?: boolean;
        hasSomethingToRedo?: boolean;
        hasSelectedEditor?: boolean;
      };
    }) => {
      if (!active) return;
      if (event.details?.hasSomethingToUndo !== undefined)
        setCanUndo(event.details.hasSomethingToUndo);
      if (event.details?.hasSomethingToRedo !== undefined)
        setCanRedo(event.details.hasSomethingToRedo);
      if (event.details?.hasSelectedEditor !== undefined) {
        selectedEditorRef.current = event.details.hasSelectedEditor;
        setSelectedEditorTool(
          event.details.hasSelectedEditor ? lastParamsToolRef.current : undefined,
        );
      }
    };
    const onEditorParams = (event: { details?: [number, unknown][] }) => {
      if (!active || pendingToolRef.current) return;
      const next = readEditorParams(editorSettingsRef.current, event.details || []);
      const tool = editorToolFromParams(event.details || []);
      if (tool) {
        lastParamsToolRef.current = tool;
        if (selectedEditorRef.current) setSelectedEditorTool(tool);
      }
      editorSettingsRef.current = next;
      setEditorSettings(next);
    };
    const onEditorMode = (event: { mode?: number }) => {
      const tool = editorToolFromMode(event.mode);
      if (!active || !tool || tool !== annotationToolRef.current) return;
      // Changing modes is asynchronous and clears the previous selection.
      // Apply this tool's defaults only after PDF.js has finished that switch.
      const settings = pendingToolRef.current?.settings || editorSettingsRef.current[tool];
      pendingToolRef.current = null;
      const next = { ...editorSettingsRef.current, [tool]: settings };
      editorSettingsRef.current = next;
      setEditorSettings(next);
      applyEditorSettings(eventBus, tool, settings);
      setToolSwitching(false);
    };

    eventBus.on("pagechanging", onPageChanging);
    eventBus.on("scalechanging", onScaleChanging);
    eventBus.on("rotationchanging", onRotationChanging);
    eventBus.on("updatefindmatchescount", onFindCount);
    eventBus.on("updatefindcontrolstate", onFindState);
    eventBus.on("editingstateschanged", onEditingState);
    eventBus.on("annotationeditorparamschanged", onEditorParams);
    eventBus.on("annotationeditormodechanged", onEditorMode);

    const updateCanvasBudget = () => {
      const pixels = pdfCanvasPixelBudget(container);
      if (viewer.maxCanvasPixels === pixels) return false;
      viewer.maxCanvasPixels = pixels;
      for (let i = 0; i < viewer.pagesCount; i++) {
        viewer.getPageView(i).maxCanvasPixels = pixels;
      }
      return true;
    };
    // Moving the window between normal and Retina displays can change DPR
    // without changing the viewer's CSS width. Invalidate the scaled canvases
    // and re-render the visible pages at their new device pixel density.
    let displayedPixelRatio = window.devicePixelRatio || 1;
    let resolutionMedia = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`);
    const onResolutionChange = () => {
      const nextPixelRatio = window.devicePixelRatio || 1;
      if (nextPixelRatio === displayedPixelRatio) return;
      displayedPixelRatio = nextPixelRatio;
      resolutionMedia.removeEventListener("change", onResolutionChange);
      resolutionMedia = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`);
      resolutionMedia.addEventListener("change", onResolutionChange);
      if (!active || !viewer.pdfDocument) return;
      updateCanvasBudget();
      viewer.refresh();
    };
    resolutionMedia.addEventListener("change", onResolutionChange);
    // The browser normally reports display changes through resolution or
    // resize events. Zoom itself is rerendered by PDF.js; no polling needed.
    window.addEventListener("resize", onResolutionChange);
    let resizeFrame = 0;
    let previousWidth = container.clientWidth;
    const resizeObserver = new ResizeObserver(() => {
      const budgetChanged = updateCanvasBudget();
      // Height-only changes (keyboard, toolbars) must not reflow every PDF page.
      if (container.clientWidth === previousWidth) {
        if (budgetChanged && viewer.pdfDocument) viewer.refresh();
        return;
      }
      previousWidth = container.clientWidth;
      cancelAnimationFrame(resizeFrame);
      resizeFrame = requestAnimationFrame(() => {
        if (!active || !viewer.pdfDocument) return;
        const preset = viewer.currentScaleValue;
        if (preset === "page-width" || preset === "page-fit" || preset === "auto") {
          viewer.currentScaleValue = preset;
        } else viewer.update();
      });
    });
    resizeObserver.observe(container);
    const open = async () => {
      readyRef.current = false;
      setReady(false);
      setError(undefined);
      setLoadingProgress(undefined);
      setPasswordChallenge(undefined);
      setPasswordDraft("");
      setOutline([]);
      setNumPages(0);
      setCanUndo(false);
      setCanRedo(false);
      setCanAnnotate(false);
      selectedEditorRef.current = false;
      setSelectedEditorTool(undefined);
      setSaveError(undefined);
      dirtyRef.current = false;
      setDirty(false);
      loadingTask = getDocument({
        url,
        withCredentials: true,
        enableHWA: true,
        cMapUrl: "/pdfjs/cmaps/",
        standardFontDataUrl: "/pdfjs/standard_fonts/",
        wasmUrl: "/pdfjs/wasm/",
        iccUrl: "/pdfjs/iccs/",
      });
      loadingTask.onPassword = (updatePassword: (password: string) => void, reason: number) => {
        if (!active) return;
        setPasswordDraft("");
        setPasswordChallenge({
          incorrect: reason === PasswordResponses.INCORRECT_PASSWORD,
          submit: updatePassword,
        });
      };
      loadingTask.onProgress = (progress: { loaded: number; total?: number }) => {
        if (!active || !progress.total) return;
        setLoadingProgress(Math.min(100, Math.round((progress.loaded / progress.total) * 100)));
      };
      const pdf = await loadingTask.promise;
      if (!active) return;
      const storage = pdf.annotationStorage as unknown as EditableAnnotationStorage;
      storage.onSetModified = markDirty;

      runtimeRef.current = { eventBus, linkService, findController, viewer, document: pdf };
      setDocument(pdf);
      setNumPages(pdf.numPages);
      linkService.setDocument(pdf);
      findController.setDocument(pdf);

      // An unavailable outline must not prevent the document itself from opening.
      void pdf
        .getOutline()
        .then((loadedOutline) => {
          if (active) setOutline((loadedOutline || []) as OutlineItem[]);
        })
        .catch(() => {
          if (active) setOutline([]);
        });

      const pagesInitialized = new Promise<void>((resolve) => {
        eventBus.on("pagesinit", () => resolve(), { once: true });
      });
      viewer.setDocument(pdf);
      await pagesInitialized;
      if (!active) return;

      viewer.pagesRotation = initialRotation;
      viewer.currentScaleValue = initialScaleValue;
      viewer.currentPageNumber = Math.min(Math.max(initialPage, 1), pdf.numPages);
      setPageNumber(viewer.currentPageNumber);
      pageDraftRef.current = String(viewer.currentPageNumber);
      setPageDraft(String(viewer.currentPageNumber));
      setScale(viewer.currentScale);
      setScaleValue(viewer.currentScaleValue || initialScaleValue);
      setRotation(viewer.pagesRotation);
      readyRef.current = true;
      setCanAnnotate(
        (viewer.annotationEditorMode as unknown as number) !== AnnotationEditorType.DISABLE,
      );
      setReady(true);
      setLoadingProgress(100);
    };

    void open().catch((reason: unknown) => {
      if (!active) return;
      setError(reason instanceof Error ? reason.message : t("pdf.openFailed"));
    });

    return () => {
      active = false;
      window.clearTimeout(pageUpdateTimeout);
      readyRef.current = false;
      setReady(false);
      eventBus.off("pagechanging", onPageChanging);
      eventBus.off("scalechanging", onScaleChanging);
      eventBus.off("rotationchanging", onRotationChanging);
      eventBus.off("updatefindmatchescount", onFindCount);
      eventBus.off("updatefindcontrolstate", onFindState);
      eventBus.off("editingstateschanged", onEditingState);
      eventBus.off("annotationeditorparamschanged", onEditorParams);
      eventBus.off("annotationeditormodechanged", onEditorMode);
      resizeObserver.disconnect();
      resolutionMedia.removeEventListener("change", onResolutionChange);
      window.removeEventListener("resize", onResolutionChange);
      cancelAnimationFrame(resizeFrame);
      const pdf = runtimeRef.current?.document;
      if (pdf) (pdf.annotationStorage as unknown as EditableAnnotationStorage).onSetModified = null;
      viewer.setDocument(null);
      linkService.setDocument(null);
      viewerAbort.abort();
      runtimeRef.current = null;
      setDocument(undefined);
      if (loadingTask) void loadingTask.destroy().catch(() => undefined);
    };
  }, [file.id, loadAttempt, markDirty, url]);

  const dispatchFind = useCallback(
    (type: "" | "again" | "highlightallchange" = "", previous = false) => {
      const eventBus = runtimeRef.current?.eventBus;
      if (!eventBus) return;
      if (!query.trim()) {
        setFindCount({ current: 0, total: 0 });
        eventBus.dispatch("findbarclose", { source: containerRef.current });
        return;
      }
      eventBus.dispatch("find", {
        source: containerRef.current,
        type,
        query,
        phraseSearch: true,
        caseSensitive,
        entireWord: wholeWord,
        highlightAll: true,
        findPrevious: previous,
        matchDiacritics: false,
      });
    },
    [caseSensitive, query, wholeWord],
  );

  useEffect(() => {
    if (!ready || !searchOpen) return;
    const timer = window.setTimeout(() => dispatchFind(""), 180);
    return () => window.clearTimeout(timer);
  }, [dispatchFind, ready, searchOpen]);

  const closeSearch = useCallback(() => {
    setSearchOpen(false);
    setQuery("");
    setFindCount({ current: 0, total: 0 });
    setFindState(FindState.FOUND);
    runtimeRef.current?.eventBus.dispatch("findbarclose", { source: containerRef.current });
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target instanceof HTMLElement ? event.target : null;
      if (inNestedOverlay(target) || savingRef.current) return;
      if (target && !readerRef.current?.contains(target) && target !== window.document.body) return;
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
        event.preventDefault();
        event.stopPropagation();
        setSearchOpen(true);
        return;
      }
      if (
        event.key === "Escape" &&
        searchOpen &&
        (!isEditableTarget(target) || target?.closest("[data-pdf-findbar]"))
      ) {
        event.preventDefault();
        event.stopPropagation();
        closeSearch();
        return;
      }
      if (event.key === "Escape" && target?.hasAttribute("data-pdf-page-input")) {
        event.preventDefault();
        event.stopPropagation();
        pageDraftRef.current = String(runtimeRef.current?.viewer.currentPageNumber || 1);
        setPageDraft(pageDraftRef.current);
        target.blur();
        containerRef.current?.focus({ preventScroll: true });
        return;
      }
      // PDF.js owns text editing and annotation-selection keyboard shortcuts.
      if (isEditableTarget(target) || target?.closest(".annotationEditorLayer")) return;
      if (
        event.key === "Escape" &&
        annotationToolRef.current !== "select" &&
        !inNestedOverlay(target)
      ) {
        // Escape leaves the annotation tool before it closes the reader.
        event.preventDefault();
        event.stopPropagation();
        setTool("select");
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        requestClose();
        return;
      }
      if (target?.closest("button, a, [role=button], [role=tab], [role=slider]")) return;
      if (target && target !== window.document.body && !containerRef.current?.contains(target))
        return;
      const runtime = runtimeRef.current;
      if (!runtime) return;
      if (event.key === "+" || event.key === "=") {
        event.preventDefault();
        setPdfScale(runtime.viewer.currentScale * 1.1);
      } else if (event.key === "-") {
        event.preventDefault();
        setPdfScale(runtime.viewer.currentScale / 1.1);
      } else if (event.key === "PageDown" || event.key === "ArrowRight") {
        event.preventDefault();
        runtime.viewer.currentPageNumber = Math.min(
          runtime.viewer.currentPageNumber + 1,
          runtime.document.numPages,
        );
      } else if (event.key === "PageUp" || event.key === "ArrowLeft") {
        event.preventDefault();
        runtime.viewer.currentPageNumber = Math.max(runtime.viewer.currentPageNumber - 1, 1);
      }
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [closeSearch, searchOpen]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || annotationTool !== "pan" || saving) return;
    let pointer: { id: number; x: number; y: number } | undefined;
    const onDown = (event: PointerEvent) => {
      if (event.button !== 0 || !event.isPrimary) return;
      if (
        event.target instanceof HTMLElement &&
        event.target.closest("a, button, input, textarea, select")
      )
        return;
      event.preventDefault();
      pointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
      container.setPointerCapture(event.pointerId);
      container.dataset.dragging = "true";
      container.focus({ preventScroll: true });
    };
    const onMove = (event: PointerEvent) => {
      if (!pointer || event.pointerId !== pointer.id) return;
      event.preventDefault();
      runtimeRef.current?.viewer.panBy(event.clientX - pointer.x, event.clientY - pointer.y);
      pointer.x = event.clientX;
      pointer.y = event.clientY;
    };
    const onUp = (event: PointerEvent) => {
      if (!pointer || event.pointerId !== pointer.id) return;
      if (container.hasPointerCapture(pointer.id)) container.releasePointerCapture(pointer.id);
      pointer = undefined;
      delete container.dataset.dragging;
    };
    container.addEventListener("pointerdown", onDown, true);
    container.addEventListener("pointermove", onMove);
    container.addEventListener("pointerup", onUp);
    container.addEventListener("pointercancel", onUp);
    return () => {
      if (pointer && container.hasPointerCapture(pointer.id))
        container.releasePointerCapture(pointer.id);
      container.removeEventListener("pointerdown", onDown, true);
      container.removeEventListener("pointermove", onMove);
      container.removeEventListener("pointerup", onUp);
      container.removeEventListener("pointercancel", onUp);
      delete container.dataset.dragging;
    };
  }, [annotationTool, saving]);

  const setPdfScale = (next: number) => {
    const viewer = runtimeRef.current?.viewer;
    if (!viewer) return;
    viewer.currentScale = Math.min(5, Math.max(0.25, next));
    setToolsOpen(false);
  };

  const setPdfScaleValue = (next: string) => {
    const viewer = runtimeRef.current?.viewer;
    if (!viewer) return;
    viewer.currentScaleValue = next;
    setToolsOpen(false);
  };

  const goToPage = (next: number) => {
    const runtime = runtimeRef.current;
    if (!runtime || !readyRef.current || savingRef.current) return;
    runtime.viewer.currentPageNumber = Math.min(
      Math.max(Math.round(next), 1),
      runtime.document.numPages,
    );
  };

  const commitPageDraft = () => {
    const runtime = runtimeRef.current;
    if (!runtime) return;
    const draft = pageDraftRef.current.trim();
    const next = Number(draft);
    if (draft && Number.isFinite(next)) goToPage(next);
    // PDF.js doesn't emit pagechanging when clamping to the current page.
    pageDraftRef.current = String(runtime.viewer.currentPageNumber);
    setPageDraft(pageDraftRef.current);
  };

  const rotate = () => {
    const viewer = runtimeRef.current?.viewer;
    if (!viewer) return;
    viewer.pagesRotation = normalizedRotation(viewer.pagesRotation + 90);
    setToolsOpen(false);
  };

  const setTool = (tool: AnnotationTool) => {
    const runtime = runtimeRef.current;
    if (!runtime || !readyRef.current || savingRef.current) return;
    if (pendingToolRef.current?.tool === tool) return;
    // PDF.js's getter returns a number; its generated declaration incorrectly
    // uses the object-shaped setter type for both accessors.
    const currentMode = runtime.viewer.annotationEditorMode as unknown as number;
    if (isEditorTool(tool) && currentMode === AnnotationEditorType.DISABLE) return;
    annotationToolRef.current = tool;
    setAnnotationTool(tool);
    const mode = annotationEditorMode(tool);
    setToolSwitching(isEditorTool(tool) && currentMode !== mode);
    if (isEditorTool(tool)) {
      pendingToolRef.current = { tool, settings: { ...editorSettingsRef.current[tool] } };
    } else pendingToolRef.current = null;
    if (currentMode === mode && isEditorTool(tool)) {
      pendingToolRef.current = null;
      applyEditorSettings(runtime.eventBus, tool, editorSettingsRef.current[tool]);
    }
    if (currentMode !== AnnotationEditorType.DISABLE)
      runtime.viewer.annotationEditorMode = { mode };
    setToolsOpen(false);
  };

  const changeEditorSetting = <K extends keyof ToolSettings>(
    tool: EditorTool,
    key: K,
    value: ToolSettings[K],
  ) => {
    const eventBus = runtimeRef.current?.eventBus;
    if (!eventBus || savingRef.current) return;
    const settings = { ...editorSettingsRef.current[tool], [key]: value };
    const next = { ...editorSettingsRef.current, [tool]: settings };
    editorSettingsRef.current = next;
    setEditorSettings(next);
    if (pendingToolRef.current?.tool === tool) {
      pendingToolRef.current = { tool, settings };
    } else dispatchEditorParam(eventBus, editorParamType(tool, key), value);
  };

  const editAction = (name: "undo" | "redo") => {
    if (savingRef.current) return;
    runtimeRef.current?.eventBus.dispatch("editingaction", { source: readerRef.current, name });
    setToolsOpen(false);
  };

  const saveModified = async () => {
    const pdf = runtimeRef.current?.document;
    if (!pdf || savingRef.current) return false;
    setToolsOpen(false);
    if (window.document.activeElement instanceof HTMLElement) window.document.activeElement.blur();
    const revision = editRevisionRef.current;
    savingRef.current = true;
    setSaving(true);
    setSaveError(undefined);
    try {
      const data = pdf.annotationStorage.size ? await pdf.saveDocument() : await pdf.getData();
      const copy = new Uint8Array(data);
      const objectUrl = URL.createObjectURL(new Blob([copy.buffer], { type: "application/pdf" }));
      const anchor = window.document.createElement("a");
      anchor.href = objectUrl;
      anchor.download = editedPdfName(file.name);
      anchor.click();
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1_000);
      if (editRevisionRef.current === revision) {
        dirtyRef.current = false;
        setDirty(false);
      }
      return true;
    } catch (reason) {
      setSaveError(reason instanceof Error ? reason.message : t("pdf.saveFailed"));
      return false;
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  const downloadOriginal = () => {
    setToolsOpen(false);
    const anchor = window.document.createElement("a");
    anchor.href = url;
    anchor.download = file.name;
    anchor.click();
  };

  const zoomLabel = useMemo(() => {
    if (scaleValue === "page-width") return t("pdf.fitWidth");
    if (scaleValue === "page-fit") return t("pdf.fitPage");
    if (scaleValue === "page-actual") return t("pdf.actualSize");
    return `${Math.round(scale * 100)}%`;
  }, [scale, scaleValue, t]);

  const sidebar = (
    <PdfSidebar
      file={file}
      document={document}
      pageNumber={pageNumber}
      outline={outline}
      selectedTab={sidebarTab}
      onTabChange={setSidebarTab}
      onPage={goToPage}
      lastDocumentScrollRef={lastDocumentScrollRef}
      onOutline={(item) => {
        const runtime = runtimeRef.current;
        if (!runtime) return;
        if (item.url) {
          window.open(item.url, "_blank", "noopener,noreferrer");
          return;
        }
        if (item.dest) {
          const container = containerRef.current;
          const horizontalPosition = container?.scrollLeft;
          void runtime.linkService.goToDestination(item.dest as string | unknown[]).then(() => {
            // Named destinations can carry an X coordinate. Keep bookmark navigation
            // vertical without shifting the page horizontally under the sidebar.
            if (container && horizontalPosition !== undefined)
              container.scrollLeft = horizontalPosition;
          });
        }
      }}
      onNavigateMobile={() => drawerState.close()}
    />
  );

  return (
    <div
      ref={readerRef}
      data-pdf-reader
      data-pdf-hand-active={annotationTool === "pan" || undefined}
      onInputCapture={(event) => {
        if (containerRef.current?.contains(event.target as Node)) markDirty();
      }}
      className="flex h-dvh min-h-0 w-full flex-col overflow-hidden bg-background text-foreground"
    >
      <header
        data-pdf-toolbar
        className="relative z-30 flex min-h-14 shrink-0 items-center gap-1.5 border-b border-border bg-background px-2 sm:px-3"
      >
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("pdf.close")}
          isDisabled={saving}
          onPress={requestClose}
        >
          <CloseIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant={sidebarOpen ? "secondary" : "ghost"}
          aria-label={t("pdf.toggleSidebar")}
          className="hidden lg:inline-flex"
          onPress={() => setSidebarOpen((value) => !value)}
        >
          <MenuIcon className="size-4" />
        </Button>
        <Drawer state={drawerState}>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.openSidebar")}
            className="lg:hidden"
          >
            <MenuIcon className="size-4" />
          </Button>
          <Drawer.Backdrop variant="blur">
            <Drawer.Content placement="left" className="w-[min(88vw,20rem)]">
              <Drawer.Dialog>
                <Drawer.Header className="border-b border-border">
                  <Drawer.Heading>{t("pdf.docNav")}</Drawer.Heading>
                  <Drawer.CloseTrigger />
                </Drawer.Header>
                <Drawer.Body className="min-h-0 p-0">{sidebar}</Drawer.Body>
              </Drawer.Dialog>
            </Drawer.Content>
          </Drawer.Backdrop>
        </Drawer>

        <div className="mr-1 hidden min-w-0 max-w-64 lg:block xl:max-w-80">
          <p className="truncate text-xs font-semibold">{file.name}</p>
          <p className="text-[10px] text-muted">PDF · {formatBytes(file.size || 0)}</p>
        </div>

        <div className="hidden h-6 w-px bg-border sm:block" />

        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("pdf.prevPage")}
          isDisabled={!ready || pageNumber <= 1}
          onPress={() => goToPage(pageNumber - 1)}
        >
          <LeftIcon className="size-4" />
        </Button>
        <div className="flex items-center gap-1 text-xs tabular-nums">
          <InputGroup className="w-14" variant="secondary">
            <InputGroup.Input
              data-pdf-page-input
              aria-label={t("pdf.pageNumber")}
              inputMode="numeric"
              value={pageDraft}
              onChange={(event) => {
                pageDraftRef.current = event.target.value.replace(/[^0-9]/g, "");
                setPageDraft(pageDraftRef.current);
              }}
              onBlur={commitPageDraft}
              onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
                if (event.key === "Enter") {
                  commitPageDraft();
                  event.currentTarget.blur();
                }
              }}
              className="h-8 min-w-0 w-full text-center text-xs tabular-nums"
            />
          </InputGroup>
          <span className="min-w-8 text-muted">/ {numPages || "—"}</span>
        </div>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("pdf.nextPage")}
          isDisabled={!ready || pageNumber >= numPages}
          onPress={() => goToPage(pageNumber + 1)}
        >
          <RightIcon className="size-4" />
        </Button>

        <div className="hidden h-6 w-px bg-border md:block" />
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("pdf.zoomOut")}
          className="hidden md:inline-flex"
          isDisabled={!ready}
          onPress={() => setPdfScale(scale / 1.1)}
        >
          <ZoomOutIcon className="size-4" />
        </Button>
        <Popover>
          <Button
            size="sm"
            variant="ghost"
            className="hidden min-w-20 px-2 text-xs md:inline-flex"
            aria-label={t("pdf.zoomOptions")}
            isDisabled={!ready}
          >
            {zoomLabel}
          </Button>
          <Popover.Content placement="bottom" offset={8} className="w-44">
            <Popover.Dialog className="space-y-1 p-1.5">
              {[
                [t("pdf.fitWidth"), "page-width"],
                [t("pdf.fitPage"), "page-fit"],
                [t("pdf.actualSize"), "page-actual"],
              ].map(([label, value]) => (
                <Button
                  key={value}
                  size="sm"
                  variant={scaleValue === value ? "secondary" : "ghost"}
                  className="w-full justify-start"
                  onPress={() => setPdfScaleValue(value)}
                >
                  {label}
                </Button>
              ))}
              <div className="border-t border-border pt-1">
                {[75, 100, 125, 150, 200].map((percent) => (
                  <Button
                    key={percent}
                    size="sm"
                    variant="ghost"
                    className="w-full justify-start"
                    onPress={() => setPdfScale(percent / 100)}
                  >
                    {percent}%
                  </Button>
                ))}
              </div>
              <div className="border-t border-border px-2 pt-2 pb-1">
                <PdfBrightnessControl
                  value={brightness}
                  disabled={!ready}
                  onChange={setBrightness}
                />
              </div>
            </Popover.Dialog>
          </Popover.Content>
        </Popover>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("pdf.zoomIn")}
          className="hidden md:inline-flex"
          isDisabled={!ready}
          onPress={() => setPdfScale(scale * 1.1)}
        >
          <ZoomInIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("pdf.rotate")}
          className="hidden md:inline-flex"
          isDisabled={!ready}
          onPress={rotate}
        >
          <RotateIcon className="size-4" />
        </Button>

        <div className="hidden h-6 w-px bg-border xl:block" />
        <div
          className="hidden items-center gap-0.5 xl:flex"
          role="toolbar"
          aria-label={t("pdf.tools")}
        >
          <ToolButton
            label={t("pdf.toolSelect")}
            active={annotationTool === "select"}
            onPress={() => setTool("select")}
          >
            <TextIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label={t("pdf.toolHand")}
            active={annotationTool === "pan"}
            onPress={() => setTool("pan")}
          >
            <HandIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label={t("pdf.toolHighlight")}
            disabled={!canAnnotate || saving || toolSwitching}
            active={annotationTool === "highlight"}
            onPress={() => setTool("highlight")}
          >
            <BrushIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label={t("pdf.toolText")}
            disabled={!canAnnotate || saving || toolSwitching}
            active={annotationTool === "text"}
            onPress={() => setTool("text")}
          >
            <TextIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label={t("pdf.toolDraw")}
            disabled={!canAnnotate || saving || toolSwitching}
            active={annotationTool === "ink"}
            onPress={() => setTool("ink")}
          >
            <PencilIcon className="size-4" />
          </ToolButton>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.undo")}
            isDisabled={!canUndo || saving}
            onPress={() => editAction("undo")}
          >
            <UndoIcon className="size-4" />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("pdf.redo")}
            isDisabled={!canRedo || saving}
            onPress={() => editAction("redo")}
          >
            <RotateIcon className="size-4" />
          </Button>
        </div>

        <Popover isOpen={toolsOpen} onOpenChange={setToolsOpen}>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            className="xl:hidden"
            aria-label={t("pdf.readerTools")}
            isDisabled={!ready}
            // Restore focus to the document, not a previously edited annotation,
            // when the mobile tools menu closes after a tool switch.
            onPressStart={() => containerRef.current?.focus({ preventScroll: true })}
          >
            <EllipsisIcon className="size-4" />
          </Button>
          <Popover.Content
            placement="bottom end"
            offset={8}
            className="max-h-[75dvh] w-[min(92vw,17rem)] overflow-y-auto"
          >
            <Popover.Dialog className="p-2">
              <p className="px-2 pb-1.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted">
                {t("pdf.viewSection")}
              </p>
              <div className="grid grid-cols-2 gap-1">
                <Button
                  size="sm"
                  variant={scaleValue === "page-width" ? "secondary" : "ghost"}
                  onPress={() => setPdfScaleValue("page-width")}
                >
                  {t("pdf.fitWidth")}
                </Button>
                <Button
                  size="sm"
                  variant={scaleValue === "page-fit" ? "secondary" : "ghost"}
                  onPress={() => setPdfScaleValue("page-fit")}
                >
                  {t("pdf.fitPage")}
                </Button>
                <Button size="sm" variant="ghost" onPress={() => setPdfScale(scale / 1.1)}>
                  <ZoomOutIcon className="size-4" /> {t("pdf.zoomOut")}
                </Button>
                <Button size="sm" variant="ghost" onPress={() => setPdfScale(scale * 1.1)}>
                  <ZoomInIcon className="size-4" /> {t("pdf.zoomIn")}
                </Button>
                <Button size="sm" variant="ghost" className="col-span-2" onPress={rotate}>
                  <RotateIcon className="size-4" /> {t("pdf.rotate")}
                </Button>
                <Button
                  size="sm"
                  variant={annotationTool === "pan" ? "secondary" : "ghost"}
                  className="col-span-2"
                  onPress={() => setTool("pan")}
                >
                  <HandIcon className="size-4" /> {t("pdf.toolHand")}
                </Button>
              </div>
              <div className="mt-2 border-t border-border px-2 pt-2">
                <PdfBrightnessControl
                  value={brightness}
                  disabled={!ready}
                  onChange={setBrightness}
                />
              </div>

              <div className="mt-2 border-t border-border pt-2">
                <p className="px-2 pb-1.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted">
                  {t("pdf.annotateSection")}
                </p>
                <div className="grid grid-cols-2 gap-1">
                  <Button
                    size="sm"
                    variant={annotationTool === "select" ? "secondary" : "ghost"}
                    onPress={() => setTool("select")}
                  >
                    <TextIcon className="size-4" /> {t("pdf.select")}
                  </Button>
                  <Button
                    size="sm"
                    variant={annotationTool === "highlight" ? "secondary" : "ghost"}
                    isDisabled={!canAnnotate || saving || toolSwitching}
                    onPress={() => setTool("highlight")}
                  >
                    <BrushIcon className="size-4" /> {t("pdf.highlight")}
                  </Button>
                  <Button
                    size="sm"
                    variant={annotationTool === "text" ? "secondary" : "ghost"}
                    isDisabled={!canAnnotate || saving || toolSwitching}
                    onPress={() => setTool("text")}
                  >
                    <TextIcon className="size-4" /> {t("pdf.addText")}
                  </Button>
                  <Button
                    size="sm"
                    variant={annotationTool === "ink" ? "secondary" : "ghost"}
                    isDisabled={!canAnnotate || saving || toolSwitching}
                    onPress={() => setTool("ink")}
                  >
                    <PencilIcon className="size-4" /> {t("pdf.draw")}
                  </Button>
                </div>
              </div>

              <div className="mt-2 grid grid-cols-2 gap-1 border-t border-border pt-2">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={t("pdf.undo")}
                  isDisabled={!canUndo || saving}
                  onPress={() => editAction("undo")}
                >
                  <UndoIcon className="size-4" /> {t("pdf.undoShort")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={t("pdf.redo")}
                  isDisabled={!canRedo || saving}
                  onPress={() => editAction("redo")}
                >
                  <RotateIcon className="size-4" /> {t("pdf.redoShort")}
                </Button>
              </div>

              <div className="mt-2 grid grid-cols-2 gap-1 border-t border-border pt-2">
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={saving}
                  onPress={() => void saveModified()}
                >
                  {saving ? <Spinner size="sm" /> : <SaveIcon className="size-4" />}{" "}
                  {t("pdf.saveCopy")}
                </Button>
                <Button size="sm" variant="ghost" onPress={downloadOriginal}>
                  <DownloadIcon className="size-4" /> {t("pdf.original")}
                </Button>
              </div>
            </Popover.Dialog>
          </Popover.Content>
        </Popover>

        <div className="flex-1" />
        <Button
          isIconOnly
          size="sm"
          variant={searchOpen ? "secondary" : "ghost"}
          aria-label={t("pdf.search")}
          onPress={() => {
            if (searchOpen) closeSearch();
            else setSearchOpen(true);
          }}
        >
          <SearchIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          className="hidden xl:inline-flex"
          aria-label={t("pdf.saveCopyHint")}
          isDisabled={!ready || saving}
          onPress={() => void saveModified()}
        >
          {saving ? <Spinner size="sm" /> : <SaveIcon className="size-4" />}
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          className="hidden xl:inline-flex"
          aria-label={t("pdf.downloadOriginal")}
          onPress={downloadOriginal}
        >
          <DownloadIcon className="size-4" />
        </Button>
      </header>

      {activeEditorTool ? (
        <PdfAnnotationSettings
          tool={activeEditorTool}
          settings={editorSettings[activeEditorTool]}
          disabled={saving || toolSwitching}
          onChange={(key, value) => changeEditorSetting(activeEditorTool, key, value)}
        />
      ) : null}

      {saveError && !closePrompt ? (
        <div
          role="alert"
          className="flex shrink-0 items-center justify-between gap-3 border-b border-danger/20 bg-danger/10 px-3 py-2 text-sm"
        >
          <span>{t("pdf.saveError", { error: saveError })}</span>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={saving}
            onPress={() => void saveModified()}
          >
            {t("pdf.retry")}
          </Button>
        </div>
      ) : null}

      {searchOpen ? (
        <PdfFindBar
          query={query}
          onQuery={setQuery}
          count={findCount}
          state={findState}
          caseSensitive={caseSensitive}
          wholeWord={wholeWord}
          onCaseSensitive={setCaseSensitive}
          onWholeWord={setWholeWord}
          onPrevious={() => dispatchFind("again", true)}
          onNext={() => dispatchFind("again", false)}
          onClose={closeSearch}
        />
      ) : null}

      <div className="relative flex min-h-0 flex-1 overflow-hidden">
        {sidebarOpen ? (
          <aside className="absolute inset-y-0 left-0 z-20 hidden w-64 border-r border-border bg-surface shadow-xl lg:block xl:w-72">
            {sidebar}
          </aside>
        ) : null}
        <main className="relative min-w-0 flex-1 overflow-hidden bg-default/35">
          {!ready && !error && !passwordChallenge ? (
            <div className="absolute inset-0 z-20 grid place-items-center bg-background/70 backdrop-blur-sm">
              <div className="text-center">
                <Spinner size="lg" aria-label={t("pdf.loadingPdf")} />
                <p className="mt-3 text-xs text-muted">
                  {loadingProgress === undefined
                    ? t("pdf.opening")
                    : t("pdf.loadingPct", { progress: loadingProgress })}
                </p>
              </div>
            </div>
          ) : null}
          {passwordChallenge ? (
            <div className="absolute inset-0 z-30 grid place-items-center bg-background/80 p-5 backdrop-blur-md">
              <div className="w-full max-w-sm rounded-2xl border border-border bg-surface p-5 shadow-2xl">
                <p className="text-sm font-semibold">{t("pdf.protected")}</p>
                <p className="mt-1 text-xs leading-5 text-muted">
                  {passwordChallenge.incorrect ? t("pdf.badPassword") : t("pdf.needPassword")}
                </p>
                <InputGroup className="mt-4" variant="secondary">
                  <InputGroup.Input
                    autoFocus
                    type="password"
                    aria-label={t("pdf.passwordAria")}
                    placeholder={t("pdf.passwordPlaceholder")}
                    value={passwordDraft}
                    onChange={(event) => setPasswordDraft(event.target.value)}
                    onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
                      if (event.key === "Enter" && passwordDraft) {
                        passwordChallenge.submit(passwordDraft);
                        setPasswordChallenge(undefined);
                      }
                    }}
                  />
                </InputGroup>
                <div className="mt-4 flex justify-end gap-2">
                  <Button size="sm" variant="ghost" onPress={requestClose}>
                    {t("dialog.cancel")}
                  </Button>
                  <Button
                    size="sm"
                    variant="primary"
                    isDisabled={!passwordDraft}
                    onPress={() => {
                      passwordChallenge.submit(passwordDraft);
                      setPasswordChallenge(undefined);
                    }}
                  >
                    {t("pdf.unlock")}
                  </Button>
                </div>
              </div>
            </div>
          ) : null}
          {error ? (
            <div className="absolute inset-0 z-20 grid place-items-center p-6 text-center">
              <div className="max-w-lg">
                <p className="font-semibold">{t("pdf.unable")}</p>
                <p className="mt-2 text-sm text-muted">{error}</p>
                <Button
                  className="mt-4"
                  variant="secondary"
                  onPress={() => setLoadAttempt((value) => value + 1)}
                >
                  {t("pdf.retry")}
                </Button>
              </div>
            </div>
          ) : null}
          {/* biome-ignore lint/a11y/useSemanticElements: PDF.js requires a DIV scroll container. */}
          <div
            ref={containerRef}
            data-pdf-viewer-container
            role="region"
            aria-label={t("pdf.docAria")}
            // biome-ignore lint/a11y/noNoninteractiveTabindex: The document viewport is keyboard navigable.
            tabIndex={0}
            inert={saving}
            className="absolute inset-3 overflow-auto outline-none sm:inset-4"
          >
            <div
              ref={viewerRef}
              className="pdfViewer teldrive-pdf-viewer"
              data-pdf-brightness={brightness}
              style={brightness === 100 ? undefined : { filter: `brightness(${brightness / 100})` }}
            />
          </div>
          {saving ? (
            <div className="absolute inset-0 z-30 grid place-items-center bg-background/50 backdrop-blur-sm">
              <Spinner size="lg" aria-label={t("pdf.saving")} />
            </div>
          ) : null}
        </main>
      </div>
      {selectionAction ? (
        <Popover
          isOpen
          onOpenChange={(open) => {
            if (!open) setSelectionAction(null);
          }}
        >
          <Button
            isIconOnly
            size="sm"
            variant="secondary"
            aria-label="PDF selection actions"
            className="fixed z-40 size-7 -translate-x-1/2 -translate-y-full shadow-md"
            style={{ left: selectionAction.x, top: selectionAction.y }}
          >
            <EllipsisIcon className="size-4" />
          </Button>
          <Popover.Content placement="top" offset={5} className="w-auto">
            <Popover.Dialog className="flex items-center gap-1 p-1">
              <Button
                size="sm"
                variant="ghost"
                onPress={() => {
                  void copyText(selectionAction.text);
                  setSelectionAction(null);
                }}
              >
                {t("share.copy")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onPress={() => {
                  setQuery(selectionAction.text);
                  setSearchOpen(true);
                  setSelectionAction(null);
                }}
              >
                {t("pdf.find")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                isDisabled={!canAnnotate}
                onPress={() => {
                  setTool("highlight");
                  setSelectionAction(null);
                }}
              >
                {t("pdf.toolHighlight")}
              </Button>
            </Popover.Dialog>
          </Popover.Content>
        </Popover>
      ) : null}
      <AppDialog
        open={closePrompt}
        onOpenChange={setClosePrompt}
        isCloseDisabled={saving}
        isDismissable={!saving}
        title={t("pdf.unsavedTitle")}
        footer={
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="tertiary" isDisabled={saving} onPress={() => setClosePrompt(false)}>
              {t("pdf.keepEditing")}
            </Button>
            <Button
              variant="danger"
              isDisabled={saving}
              onPress={() => {
                dirtyRef.current = false;
                setDirty(false);
                closeRef.current();
              }}
            >
              {t("pdf.discard")}
            </Button>
            <Button
              variant="primary"
              isPending={saving}
              onPress={() =>
                void saveModified().then((saved) => {
                  if (saved) closeRef.current();
                })
              }
            >
              {t("pdf.saveClose")}
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted">{t("pdf.unsavedHint")}</p>
        {saveError ? (
          <p role="alert" className="mt-3 text-sm text-danger">
            {t("pdf.saveError", { error: saveError })}
          </p>
        ) : null}
      </AppDialog>
    </div>
  );
}

function PdfBrightnessControl({
  value,
  disabled,
  onChange,
}: {
  value: number;
  disabled: boolean;
  onChange: (value: number) => void;
}) {
  const { t } = useAppLocale();
  return (
    <div className="flex items-center gap-2 text-xs">
      {t("pdf.brightness")}
      <Slider
        aria-label={t("pdf.brightnessAria")}
        minValue={50}
        maxValue={150}
        step={5}
        value={value}
        isDisabled={disabled}
        className="w-24 flex-1"
        onChange={(next) => {
          if (typeof next === "number") onChange(next);
        }}
      >
        <Slider.Track>
          <Slider.Fill />
          <Slider.Thumb />
        </Slider.Track>
      </Slider>
      <span className="min-w-9 tabular-nums">{value}%</span>
    </div>
  );
}

function PdfAnnotationSettings({
  tool,
  settings,
  disabled,
  onChange,
}: {
  tool: EditorTool;
  settings: ToolSettings;
  disabled: boolean;
  onChange: (key: keyof ToolSettings, value: string | number) => void;
}) {
  const { t } = useAppLocale();
  const colors = tool === "highlight" ? HIGHLIGHT_COLORS : ANNOTATION_COLORS;
  return (
    <div
      role="toolbar"
      aria-label={t("pdf.annotSettings")}
      className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b border-border bg-surface px-3 py-2"
    >
      <span className="text-xs font-semibold">
        {tool === "highlight"
          ? t("pdf.highlight")
          : tool === "text"
            ? t("pdf.textName")
            : t("pdf.draw")}
      </span>
      <fieldset
        aria-label={t("pdf.annotColors")}
        className="flex items-center gap-1"
        disabled={disabled}
      >
        {colors.map((color) => (
          <Button
            key={color}
            isIconOnly
            size="sm"
            variant={settings.color === color ? "secondary" : "ghost"}
            aria-label={t("pdf.useColor", { color })}
            aria-pressed={settings.color === color}
            isDisabled={disabled}
            onPress={() => onChange("color", color)}
          >
            <span
              className="size-4 rounded-full border border-foreground/20"
              style={{ background: color }}
            />
          </Button>
        ))}
      </fieldset>
      {tool === "text" ? (
        <div className="flex items-center gap-2 text-xs">
          {t("pdf.fontSize")}
          <Slider
            aria-label={t("pdf.fontSizeAria")}
            minValue={6}
            maxValue={72}
            step={1}
            value={settings.size}
            isDisabled={disabled}
            className="w-24"
            onChange={(value) => {
              if (typeof value === "number") onChange("size", value);
            }}
          >
            <Slider.Track>
              <Slider.Fill />
              <Slider.Thumb />
            </Slider.Track>
          </Slider>
          <span className="min-w-9 tabular-nums">{settings.size} pt</span>
        </div>
      ) : null}
      {tool === "ink" ? (
        <>
          <div className="flex items-center gap-2 text-xs">
            {t("pdf.strokeWidth")}
            <Slider
              aria-label={t("pdf.strokeAria")}
              minValue={1}
              maxValue={20}
              step={0.5}
              value={settings.thickness}
              isDisabled={disabled}
              className="w-24"
              onChange={(value) => {
                if (typeof value === "number") onChange("thickness", value);
              }}
            >
              <Slider.Track>
                <Slider.Fill />
                <Slider.Thumb />
              </Slider.Track>
            </Slider>
            <span className="min-w-9 tabular-nums">{settings.thickness} pt</span>
          </div>
          <div className="flex items-center gap-2 text-xs">
            {t("pdf.opacity")}
            <Slider
              aria-label={t("pdf.opacityAria")}
              minValue={0}
              maxValue={100}
              step={5}
              value={Math.round(settings.opacity * 100)}
              isDisabled={disabled}
              className="w-24"
              onChange={(value) => {
                if (typeof value === "number") onChange("opacity", value / 100);
              }}
            >
              <Slider.Track>
                <Slider.Fill />
                <Slider.Thumb />
              </Slider.Track>
            </Slider>
            <span className="min-w-9 tabular-nums">{Math.round(settings.opacity * 100)}%</span>
          </div>
        </>
      ) : null}
    </div>
  );
}

function PdfFindBar({
  query,
  onQuery,
  count,
  state,
  caseSensitive,
  wholeWord,
  onCaseSensitive,
  onWholeWord,
  onPrevious,
  onNext,
  onClose,
}: {
  query: string;
  onQuery: (value: string) => void;
  count: FindCount;
  state: number;
  caseSensitive: boolean;
  wholeWord: boolean;
  onCaseSensitive: (value: boolean) => void;
  onWholeWord: (value: boolean) => void;
  onPrevious: () => void;
  onNext: () => void;
  onClose: () => void;
}) {
  const { t } = useAppLocale();
  return (
    <div
      data-pdf-findbar
      className="z-20 flex min-h-12 shrink-0 items-center gap-1.5 border-b border-border bg-surface/95 px-2 backdrop-blur-xl sm:px-3"
    >
      <SearchIcon className="hidden size-4 text-muted sm:block" />
      <InputGroup className="max-w-md flex-1" variant="secondary">
        <InputGroup.Input
          autoFocus
          aria-label={t("pdf.find")}
          placeholder={t("pdf.findPlaceholder")}
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
            if (event.key === "Enter") {
              event.preventDefault();
              if (event.shiftKey) onPrevious();
              else onNext();
            }
          }}
          className="h-8 text-sm"
        />
        <InputGroup.Suffix className="text-[11px] tabular-nums text-muted">
          {query && state === FindState.NOT_FOUND
            ? t("pdf.noMatches")
            : query
              ? `${count.current} / ${count.total}`
              : ""}
        </InputGroup.Suffix>
      </InputGroup>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={t("pdf.prevResult")}
        isDisabled={!count.total}
        onPress={onPrevious}
      >
        <LeftIcon className="size-4" />
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={t("pdf.nextResult")}
        isDisabled={!count.total}
        onPress={onNext}
      >
        <RightIcon className="size-4" />
      </Button>
      <Button
        size="sm"
        variant={caseSensitive ? "secondary" : "ghost"}
        className="hidden min-w-8 px-2 text-xs font-semibold sm:inline-flex"
        aria-label={t("pdf.matchCase")}
        onPress={() => onCaseSensitive(!caseSensitive)}
      >
        Aa
      </Button>
      <Button
        size="sm"
        variant={wholeWord ? "secondary" : "ghost"}
        className="hidden px-2 text-xs sm:inline-flex"
        aria-label={t("pdf.matchWords")}
        onPress={() => onWholeWord(!wholeWord)}
      >
        {t("pdf.wordBtn")}
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={t("pdf.closeSearch")}
        onPress={onClose}
      >
        <CloseIcon className="size-4" />
      </Button>
    </div>
  );
}

function PdfSidebar({
  file,
  document,
  pageNumber,
  outline,
  selectedTab,
  onTabChange,
  onPage,
  onOutline,
  onNavigateMobile,
  lastDocumentScrollRef,
}: {
  file: FileEntry;
  document?: PDFDocumentProxy;
  pageNumber: number;
  outline: OutlineItem[];
  selectedTab: SidebarTab;
  onTabChange: (tab: SidebarTab) => void;
  onPage: (page: number) => void;
  onOutline: (item: OutlineItem) => void;
  onNavigateMobile: () => void;
  lastDocumentScrollRef: { current: number };
}) {
  const { t } = useAppLocale();
  return (
    <div className="flex h-full min-h-0 flex-col bg-surface/75">
      <div className="border-b border-border px-4 py-3">
        <p className="truncate text-xs font-semibold">{file.name}</p>
        <p className="mt-0.5 text-[10px] text-muted">
          {document ? t("pdf.pagesCount", { count: document.numPages }) : t("pdf.loadingDoc")}
        </p>
      </div>
      <Tabs
        selectedKey={selectedTab}
        onSelectionChange={(key) => onTabChange(key === "outline" ? "outline" : "thumbnails")}
        className="flex min-h-0 flex-1 flex-col px-2 pt-2"
      >
        <Tabs.ListContainer>
          <Tabs.List aria-label={t("pdf.sidebarAria")} className="w-full">
            <Tabs.Tab id="thumbnails" className="flex-1 gap-1.5 text-xs">
              <MenuIcon className="size-3.5" /> {t("pdf.tabPages")}
            </Tabs.Tab>
            <Tabs.Tab id="outline" className="flex-1 gap-1.5 text-xs">
              <BookmarkIcon className="size-3.5" /> {t("pdf.tabOutline")}
            </Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>
        <Tabs.Panel id="thumbnails" className="min-h-0 flex-1">
          {document && selectedTab === "thumbnails" ? (
            <PdfThumbnailList
              key={file.id}
              document={document}
              pageNumber={pageNumber}
              lastDocumentScrollRef={lastDocumentScrollRef}
              onPage={(page) => {
                onPage(page);
                onNavigateMobile();
              }}
            />
          ) : (
            <SidebarEmpty label={t("pdf.preparing")} />
          )}
        </Tabs.Panel>
        <Tabs.Panel id="outline" className="min-h-0 flex-1 overflow-y-auto py-2">
          {outline.length ? (
            <div key={file.id} className="space-y-0.5 px-1 pb-3">
              <OutlineItems
                items={outline}
                depth={0}
                path="outline"
                onSelect={(item) => {
                  onOutline(item);
                  onNavigateMobile();
                }}
              />
            </div>
          ) : (
            <SidebarEmpty label={t("pdf.noOutline")} />
          )}
        </Tabs.Panel>
      </Tabs>
    </div>
  );
}

const THUMBNAIL_ROW_HEIGHT = 224;
// Reuse completed previews when React Aria remounts a virtual row.
type ThumbnailPreview = {
  bitmap: ImageBitmap;
  width: number;
  height: number;
  cssWidth: string;
  cssHeight: string;
};

function createThumbnailCache() {
  const images = new Map<number, ThumbnailPreview>();
  return {
    get(page: number) {
      const preview = images.get(page);
      if (preview) {
        images.delete(page);
        images.set(page, preview);
      }
      return preview;
    },
    set(page: number, preview: ThumbnailPreview) {
      images.get(page)?.bitmap.close();
      images.delete(page);
      images.set(page, preview);
      if (images.size > 16) {
        const oldest = images.keys().next().value;
        if (oldest !== undefined) {
          images.get(oldest)?.bitmap.close();
          images.delete(oldest);
        }
      }
    },
    dispose() {
      for (const preview of images.values()) preview.bitmap.close();
      images.clear();
    },
  };
}

type ThumbnailCache = ReturnType<typeof createThumbnailCache>;

function PdfThumbnailList({
  document,
  pageNumber,
  onPage,
  lastDocumentScrollRef,
}: {
  document: PDFDocumentProxy;
  pageNumber: number;
  onPage: (page: number) => void;
  lastDocumentScrollRef: { current: number };
}) {
  const { t } = useAppLocale();
  const lastScrollRef = useRef(0);
  const scrollElementRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = scrollElementRef.current;
    if (!element) return;
    const recordScroll = () => {
      lastScrollRef.current = performance.now();
    };
    element.addEventListener("scroll", recordScroll, { passive: true });
    return () => element.removeEventListener("scroll", recordScroll);
  }, []);
  const pages = useMemo(
    () => Array.from({ length: document.numPages }, (_, index) => ({ id: index + 1 })),
    [document],
  );
  const previewCache = useMemo(createThumbnailCache, [document]);
  useEffect(() => () => previewCache.dispose(), [previewCache]);
  const onPageRef = useRef(onPage);
  onPageRef.current = onPage;
  const handlePage = useCallback((page: number) => onPageRef.current(page), []);
  const shouldPauseThumbnail = useCallback(
    () => performance.now() - Math.max(lastScrollRef.current, lastDocumentScrollRef.current) < 160,
    [lastDocumentScrollRef],
  );
  const queue = useMemo(() => {
    let tail = Promise.resolve();
    return (render: () => Promise<void>, isCancelled: () => boolean) => {
      // Never race all thumbnail canvases against the main document renderer.
      tail = tail
        .then(async () => {
          // Ignore virtual rows that disappeared before reaching the queue.
          while (!isCancelled() && shouldPauseThumbnail()) {
            await new Promise<void>((resolve) => window.setTimeout(resolve, 100));
          }
          if (!isCancelled()) await render();
        })
        .catch(() => undefined);
    };
  }, [document, shouldPauseThumbnail]);
  return (
    <Virtualizer layout={ListLayout} layoutOptions={{ rowSize: THUMBNAIL_ROW_HEIGHT }}>
      <GridList
        ref={scrollElementRef}
        data-pdf-thumbnails
        aria-label={t("pdf.tabPages")}
        items={pages}
        selectionMode="none"
        onAction={(key) => handlePage(Number(key))}
        className="h-full min-h-0 overflow-y-auto outline-none"
      >
        {(item) => (
          <GridListItem
            id={item.id}
            textValue={String(item.id)}
            className="px-1 outline-none"
            style={{ height: "100%" }}
          >
            <PdfThumbnail
              document={document}
              pageNumber={item.id}
              current={item.id === pageNumber}
              onPage={handlePage}
              queue={queue}
              shouldPause={shouldPauseThumbnail}
              previewCache={previewCache}
            />
          </GridListItem>
        )}
      </GridList>
    </Virtualizer>
  );
}

const PdfThumbnail = memo(function PdfThumbnail({
  document,
  pageNumber,
  current,
  onPage,
  queue,
  shouldPause,
  previewCache,
}: {
  document: PDFDocumentProxy;
  pageNumber: number;
  current: boolean;
  onPage: (page: number) => void;
  queue: (render: () => Promise<void>, isCancelled: () => boolean) => void;
  shouldPause: () => boolean;
  previewCache: ThumbnailCache;
}) {
  const { t } = useAppLocale();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    let active = true;
    const cached = previewCache.get(pageNumber);
    if (cached) {
      canvas.width = cached.width;
      canvas.height = cached.height;
      canvas.style.width = cached.cssWidth;
      canvas.style.height = cached.cssHeight;
      const context = canvas.getContext("2d");
      if (context) {
        context.drawImage(cached.bitmap, 0, 0);
        setLoaded(true);
        return () => {
          canvas.width = 0;
          canvas.height = 0;
        };
      }
    }
    let renderTask:
      | ReturnType<Awaited<ReturnType<PDFDocumentProxy["getPage"]>>["render"]>
      | undefined;
    let resumeFrame = 0;
    let resumeTimer = 0;
    let previewTimer = 0;
    let previewIdle = 0;
    queue(
      async () => {
        if (!active) return;
        const page = await document.getPage(pageNumber);
        if (!active) return;
        const natural = page.getViewport({ scale: 1 });
        const cssWidth = Math.min(142, (184 * natural.width) / natural.height);
        const scale = cssWidth / natural.width;
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const viewport = page.getViewport({ scale: scale * dpr });
        canvas.width = Math.ceil(viewport.width);
        canvas.height = Math.ceil(viewport.height);
        canvas.style.width = `${cssWidth}px`;
        canvas.style.height = `${Math.round(cssWidth * (natural.height / natural.width))}px`;
        renderTask = page.render({ canvas, viewport });
        renderTask.onContinue = (resume: () => void) => {
          const continueWhenIdle = () => {
            if (!active) return;
            if (shouldPause()) resumeTimer = window.setTimeout(continueWhenIdle, 80);
            else
              resumeFrame = requestAnimationFrame(() => {
                if (active) resume();
              });
          };
          continueWhenIdle();
        };
        await renderTask.promise;
        if (active) {
          setLoaded(true);
          if (typeof createImageBitmap === "function") {
            const cachePreview = async () => {
              if (!active) return;
              if (shouldPause()) {
                previewTimer = window.setTimeout(() => void cachePreview(), 100);
                return;
              }
              try {
                const bitmap = await createImageBitmap(canvas);
                if (!active) {
                  bitmap.close();
                  return;
                }
                previewCache.set(pageNumber, {
                  bitmap,
                  width: canvas.width,
                  height: canvas.height,
                  cssWidth: canvas.style.width,
                  cssHeight: canvas.style.height,
                });
              } catch {
                // The rendered canvas remains usable if caching is unavailable.
              }
            };
            // Display the finished canvas first; copying it into the small
            // preview cache must not compete with active scrolling or input.
            if (typeof window.requestIdleCallback === "function") {
              previewIdle = window.requestIdleCallback(
                () => {
                  previewIdle = 0;
                  void cachePreview();
                },
                { timeout: 500 },
              );
            } else {
              previewTimer = window.setTimeout(() => void cachePreview(), 100);
            }
          }
        }
      },
      () => !active,
    );
    return () => {
      active = false;
      renderTask?.cancel();
      cancelAnimationFrame(resumeFrame);
      window.clearTimeout(resumeTimer);
      window.clearTimeout(previewTimer);
      if (previewIdle) window.cancelIdleCallback(previewIdle);
      canvas.width = 0;
      canvas.height = 0;
    };
  }, [document, pageNumber, queue, shouldPause, previewCache]);

  return (
    <div className="flex justify-center py-1">
      <Button
        variant={current ? "secondary" : "ghost"}
        className={cn(
          "h-auto w-full flex-col gap-2 rounded-xl px-2 py-2",
          current && "ring-1 ring-accent/40",
        )}
        aria-label={t("pdf.goToPage", { page: pageNumber })}
        onPress={() => onPage(pageNumber)}
      >
        <div className="relative flex h-46 w-35.5 max-w-full items-center justify-center overflow-hidden rounded-sm border border-border bg-white shadow-sm">
          <canvas ref={canvasRef} className={cn("block max-w-full", !loaded && "opacity-0")} />
          {!loaded ? <div className="absolute inset-0 bg-default/20" /> : null}
        </div>
        <span className="text-[10px] font-medium tabular-nums text-muted">{pageNumber}</span>
      </Button>
    </div>
  );
});

function OutlineItems({
  items,
  depth,
  path,
  onSelect,
}: {
  items: OutlineItem[];
  depth: number;
  path: string;
  onSelect: (item: OutlineItem) => void;
}) {
  return items.map((item, index) => (
    <OutlineNode
      // biome-ignore lint/suspicious/noArrayIndexKey: PDF outlines expose no stable ids and never reorder within a document.
      key={`${path}/${index}`}
      item={item}
      depth={depth}
      path={`${path}/${index}`}
      onSelect={onSelect}
    />
  ));
}

function OutlineNode({
  item,
  depth,
  path,
  onSelect,
}: {
  item: OutlineItem;
  depth: number;
  path: string;
  onSelect: (item: OutlineItem) => void;
}) {
  const children = item.items ?? [];
  const hasChildren = children.length > 0;
  // Top-level parents start expanded; deeper levels start collapsed like Adobe.
  const [expanded, setExpanded] = useState(depth < 1);
  const { t } = useAppLocale();
  const title = item.title || t("pdf.untitled");
  return (
    <div>
      <div
        className="flex items-center gap-0.5"
        style={{ paddingInlineStart: `${2 + depth * 14}px` }}
      >
        {hasChildren ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            className="h-7 w-7 shrink-0"
            aria-label={t(expanded ? "pdf.collapseSection" : "pdf.expandSection", { title })}
            aria-expanded={expanded}
            onPress={() => setExpanded((value) => !value)}
          >
            {expanded ? <DownIcon className="size-3.5" /> : <RightIcon className="size-3.5" />}
          </Button>
        ) : (
          <span className="w-7 shrink-0" aria-hidden="true" />
        )}
        <Button
          size="sm"
          variant="ghost"
          className="h-auto min-w-0 flex-1 justify-start whitespace-normal py-2 text-left text-xs"
          onPress={() => onSelect(item)}
        >
          <span className="line-clamp-2">{title}</span>
        </Button>
      </div>
      {hasChildren && expanded ? (
        <OutlineItems items={children} depth={depth + 1} path={path} onSelect={onSelect} />
      ) : null}
    </div>
  );
}

function ToolButton({
  label,
  active,
  disabled = false,
  onPress,
  children,
}: {
  label: string;
  active: boolean;
  disabled?: boolean;
  onPress: () => void;
  children: React.ReactNode;
}) {
  return (
    <Button
      isIconOnly
      size="sm"
      variant={active ? "secondary" : "ghost"}
      isDisabled={disabled}
      aria-label={label}
      onPress={onPress}
    >
      {children}
    </Button>
  );
}

function SidebarEmpty({ label }: { label: string }) {
  return <p className="px-3 py-8 text-center text-xs text-muted">{label}</p>;
}

function annotationEditorMode(tool: AnnotationTool) {
  if (tool === "highlight") return AnnotationEditorType.HIGHLIGHT;
  if (tool === "text") return AnnotationEditorType.FREETEXT;
  if (tool === "ink") return AnnotationEditorType.INK;
  return AnnotationEditorType.NONE;
}

function isEditorTool(tool: AnnotationTool): tool is EditorTool {
  return tool === "highlight" || tool === "text" || tool === "ink";
}

function editorToolFromMode(mode: number | undefined): EditorTool | undefined {
  if (mode === AnnotationEditorType.HIGHLIGHT) return "highlight";
  if (mode === AnnotationEditorType.FREETEXT) return "text";
  if (mode === AnnotationEditorType.INK) return "ink";
  return undefined;
}

function editorToolFromParams(details: [number, unknown][]): EditorTool | undefined {
  if (
    details.some(
      ([type]) =>
        type === AnnotationEditorParamsType.FREETEXT_COLOR ||
        type === AnnotationEditorParamsType.FREETEXT_SIZE,
    )
  )
    return "text";
  if (
    details.some(
      ([type]) =>
        type === AnnotationEditorParamsType.INK_COLOR ||
        type === AnnotationEditorParamsType.INK_THICKNESS ||
        type === AnnotationEditorParamsType.INK_OPACITY,
    )
  )
    return "ink";
  if (details.some(([type]) => type === AnnotationEditorParamsType.HIGHLIGHT_COLOR))
    return "highlight";
  return undefined;
}

function editorParamType(tool: EditorTool, key: keyof ToolSettings): number | undefined {
  if (key === "color") {
    if (tool === "highlight") return AnnotationEditorParamsType.HIGHLIGHT_COLOR;
    if (tool === "text") return AnnotationEditorParamsType.FREETEXT_COLOR;
    return AnnotationEditorParamsType.INK_COLOR;
  }
  if (key === "size" && tool === "text") return AnnotationEditorParamsType.FREETEXT_SIZE;
  if (key === "thickness" && tool === "ink") return AnnotationEditorParamsType.INK_THICKNESS;
  if (key === "opacity" && tool === "ink") return AnnotationEditorParamsType.INK_OPACITY;
  return undefined;
}

function dispatchEditorParam(eventBus: EventBus, type: number | undefined, value: string | number) {
  if (type !== undefined) eventBus.dispatch("switchannotationeditorparams", { type, value });
}

function applyEditorSettings(eventBus: EventBus, tool: EditorTool, settings: ToolSettings) {
  for (const key of Object.keys(settings) as (keyof ToolSettings)[]) {
    dispatchEditorParam(eventBus, editorParamType(tool, key), settings[key]);
  }
}

function readEditorParams(previous: EditorSettings, details: [number, unknown][]): EditorSettings {
  let next = previous;
  for (const tool of ["highlight", "text", "ink"] as const) {
    for (const key of ["color", "size", "thickness", "opacity"] as const) {
      const type = editorParamType(tool, key);
      if (type === undefined) continue;
      const entry = details.find(([parameter]) => parameter === type);
      if (!entry) continue;
      const value = entry[1];
      if (key === "color" && typeof value === "string") {
        const color = normalizedColor(value);
        if (color && color !== next[tool].color)
          next = { ...next, [tool]: { ...next[tool], color } };
      } else if (
        key !== "color" &&
        typeof value === "number" &&
        Number.isFinite(value) &&
        (key === "opacity" ? value >= 0 : value > 0) &&
        value !== next[tool][key]
      ) {
        next = { ...next, [tool]: { ...next[tool], [key]: value } };
      }
    }
  }
  return next;
}

function normalizedColor(color: string): string | undefined {
  if (/^#[\da-f]{6}$/i.test(color)) return color.toLowerCase();
  if (/^#[\da-f]{3}$/i.test(color))
    return `#${color
      .slice(1)
      .split("")
      .map((part) => part + part)
      .join("")}`.toLowerCase();
  const rgb = color.match(/^rgb\(\s*(\d+)[,\s]+(\d+)[,\s]+(\d+)\s*\)$/);
  if (rgb)
    return `#${rgb
      .slice(1)
      .map((part) => Math.min(255, Number(part)).toString(16).padStart(2, "0"))
      .join("")}`;
  return undefined;
}

function normalizedRotation(value: unknown) {
  const number = typeof value === "number" && Number.isFinite(value) ? value : 0;
  return (((Math.round(number / 90) * 90) % 360) + 360) % 360;
}

function positiveInt(value: unknown, fallback: number) {
  const number = typeof value === "number" ? value : Number(value);
  return Number.isFinite(number) && number > 0 ? Math.round(number) : fallback;
}

function positiveNumber(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : fallback;
}

function editedPdfName(name: string) {
  const index = name.toLowerCase().lastIndexOf(".pdf");
  return index >= 0 ? `${name.slice(0, index)}-edited.pdf` : `${name}-edited.pdf`;
}

function isEditableTarget(target: EventTarget | null) {
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    (target instanceof HTMLElement && target.isContentEditable)
  );
}

// A portalled overlay (popover, drawer, nested dialog) that is not the reader
// itself owns Escape while it is open.
function inNestedOverlay(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  const overlay = target.closest('[role="dialog"]');
  return overlay !== null && overlay.querySelector("[data-pdf-reader]") === null;
}
