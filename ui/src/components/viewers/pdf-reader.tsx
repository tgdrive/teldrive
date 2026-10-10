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
  type KeyboardEvent as ReactKeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import type { FileEntry } from "@/api/types";
import { AppDialog } from "@/components/dialogs/app-dialog";
import UndoIcon from "~icons/gravity-ui/arrow-rotate-left";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import RotateIcon from "~icons/gravity-ui/arrow-rotate-right";
import MenuIcon from "~icons/gravity-ui/bars";
import BookmarkIcon from "~icons/gravity-ui/bookmark";
import BrushIcon from "~icons/gravity-ui/brush";
import LeftIcon from "~icons/gravity-ui/chevron-left";
import RightIcon from "~icons/gravity-ui/chevron-right";
import DownIcon from "~icons/gravity-ui/chevron-down";
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

  closeRef.current = onClose;
  const activeEditorTool =
    selectedEditorTool || (isEditorTool(annotationTool) ? annotationTool : undefined);

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
    const linkService = new PDFLinkService({ eventBus });
    const findController = new PDFFindController({ eventBus, linkService });
    const viewerOptions = {
      container,
      viewer: viewerElement,
      eventBus,
      linkService,
      findController,
      textLayerMode: 1,
      annotationMode: AnnotationMode.ENABLE_FORMS,
      annotationEditorMode: AnnotationEditorType.NONE,
      annotationEditorHighlightColors: PDFJS_HIGHLIGHT_COLORS,
      removePageBorders: true,
      abortSignal: viewerAbort.signal,
    };
    const viewer = new PDFViewer(viewerOptions);
    linkService.setViewer(viewer);

    const onPageChanging = (event: { pageNumber?: number }) => {
      const next = positiveInt(event.pageNumber, viewer.currentPageNumber || 1);
      setPageNumber(next);
      pageDraftRef.current = String(next);
      setPageDraft(String(next));
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

    let resizeFrame = 0;
    const resizeObserver = new ResizeObserver(() => {
      cancelAnimationFrame(resizeFrame);
      resizeFrame = requestAnimationFrame(() => {
        if (!active || !viewer.pdfDocument) return;
        const preset = viewer.currentScaleValue;
        if (preset === "page-width" || preset === "page-fit" || preset === "auto") {
          viewer.currentScaleValue = preset;
        }
        viewer.update();
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
      setError(reason instanceof Error ? reason.message : "This PDF could not be opened.");
    });

    return () => {
      active = false;
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
      if (event.key === "Escape" && target?.getAttribute("aria-label") === "PDF page number") {
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
      setSaveError(
        reason instanceof Error ? reason.message : "The edited PDF could not be saved. Try again.",
      );
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
    if (scaleValue === "page-width") return "Fit width";
    if (scaleValue === "page-fit") return "Fit page";
    if (scaleValue === "page-actual") return "Actual size";
    return `${Math.round(scale * 100)}%`;
  }, [scale, scaleValue]);

  const sidebar = (
    <PdfSidebar
      file={file}
      document={document}
      pageNumber={pageNumber}
      outline={outline}
      selectedTab={sidebarTab}
      onTabChange={setSidebarTab}
      onPage={goToPage}
      onOutline={(item) => {
        const runtime = runtimeRef.current;
        if (!runtime) return;
        if (item.url) {
          window.open(item.url, "_blank", "noopener,noreferrer");
          return;
        }
        if (item.dest) void runtime.linkService.goToDestination(item.dest as string | unknown[]);
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
        className="relative z-30 flex min-h-14 shrink-0 items-center gap-1.5 border-b border-border bg-background/95 px-2 backdrop-blur-xl sm:px-3"
      >
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Close PDF reader"
          isDisabled={saving}
          onPress={requestClose}
        >
          <CloseIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant={sidebarOpen ? "secondary" : "ghost"}
          aria-label="Toggle PDF sidebar"
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
            aria-label="Open PDF sidebar"
            className="lg:hidden"
          >
            <MenuIcon className="size-4" />
          </Button>
          <Drawer.Backdrop variant="blur">
            <Drawer.Content placement="left" className="w-[min(88vw,20rem)]">
              <Drawer.Dialog>
                <Drawer.Header className="border-b border-border">
                  <Drawer.Heading>Document navigation</Drawer.Heading>
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
          aria-label="Previous PDF page"
          isDisabled={!ready || pageNumber <= 1}
          onPress={() => goToPage(pageNumber - 1)}
        >
          <LeftIcon className="size-4" />
        </Button>
        <div className="flex items-center gap-1 text-xs tabular-nums">
          <InputGroup className="w-14" variant="secondary">
            <InputGroup.Input
              aria-label="PDF page number"
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
          aria-label="Next PDF page"
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
          aria-label="Zoom out"
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
            aria-label="PDF zoom options"
            isDisabled={!ready}
          >
            {zoomLabel}
          </Button>
          <Popover.Content placement="bottom" offset={8} className="w-44">
            <Popover.Dialog className="space-y-1 p-1.5">
              {[
                ["Fit width", "page-width"],
                ["Fit page", "page-fit"],
                ["Actual size", "page-actual"],
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
          aria-label="Zoom in"
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
          aria-label="Rotate PDF clockwise"
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
          aria-label="PDF annotation tools"
        >
          <ToolButton
            label="Select text"
            active={annotationTool === "select"}
            onPress={() => setTool("select")}
          >
            <TextIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label="Hand tool"
            active={annotationTool === "pan"}
            onPress={() => setTool("pan")}
          >
            <HandIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label="Highlight"
            disabled={!canAnnotate || saving || toolSwitching}
            active={annotationTool === "highlight"}
            onPress={() => setTool("highlight")}
          >
            <BrushIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label="Add text"
            disabled={!canAnnotate || saving || toolSwitching}
            active={annotationTool === "text"}
            onPress={() => setTool("text")}
          >
            <TextIcon className="size-4" />
          </ToolButton>
          <ToolButton
            label="Draw"
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
            aria-label="Undo PDF edit"
            isDisabled={!canUndo || saving}
            onPress={() => editAction("undo")}
          >
            <UndoIcon className="size-4" />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Redo PDF edit"
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
            aria-label="PDF reader tools"
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
                View
              </p>
              <div className="grid grid-cols-2 gap-1">
                <Button
                  size="sm"
                  variant={scaleValue === "page-width" ? "secondary" : "ghost"}
                  onPress={() => setPdfScaleValue("page-width")}
                >
                  Fit width
                </Button>
                <Button
                  size="sm"
                  variant={scaleValue === "page-fit" ? "secondary" : "ghost"}
                  onPress={() => setPdfScaleValue("page-fit")}
                >
                  Fit page
                </Button>
                <Button size="sm" variant="ghost" onPress={() => setPdfScale(scale / 1.1)}>
                  <ZoomOutIcon className="size-4" /> Zoom out
                </Button>
                <Button size="sm" variant="ghost" onPress={() => setPdfScale(scale * 1.1)}>
                  <ZoomInIcon className="size-4" /> Zoom in
                </Button>
                <Button size="sm" variant="ghost" className="col-span-2" onPress={rotate}>
                  <RotateIcon className="size-4" /> Rotate clockwise
                </Button>
                <Button
                  size="sm"
                  variant={annotationTool === "pan" ? "secondary" : "ghost"}
                  className="col-span-2"
                  onPress={() => setTool("pan")}
                >
                  <HandIcon className="size-4" /> Hand tool
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
                  Annotate
                </p>
                <div className="grid grid-cols-2 gap-1">
                  <Button
                    size="sm"
                    variant={annotationTool === "select" ? "secondary" : "ghost"}
                    onPress={() => setTool("select")}
                  >
                    <TextIcon className="size-4" /> Select
                  </Button>
                  <Button
                    size="sm"
                    variant={annotationTool === "highlight" ? "secondary" : "ghost"}
                    isDisabled={!canAnnotate || saving || toolSwitching}
                    onPress={() => setTool("highlight")}
                  >
                    <BrushIcon className="size-4" /> Highlight
                  </Button>
                  <Button
                    size="sm"
                    variant={annotationTool === "text" ? "secondary" : "ghost"}
                    isDisabled={!canAnnotate || saving || toolSwitching}
                    onPress={() => setTool("text")}
                  >
                    <TextIcon className="size-4" /> Add text
                  </Button>
                  <Button
                    size="sm"
                    variant={annotationTool === "ink" ? "secondary" : "ghost"}
                    isDisabled={!canAnnotate || saving || toolSwitching}
                    onPress={() => setTool("ink")}
                  >
                    <PencilIcon className="size-4" /> Draw
                  </Button>
                </div>
              </div>

              <div className="mt-2 grid grid-cols-2 gap-1 border-t border-border pt-2">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label="Undo PDF edit"
                  isDisabled={!canUndo || saving}
                  onPress={() => editAction("undo")}
                >
                  <UndoIcon className="size-4" /> Undo
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label="Redo PDF edit"
                  isDisabled={!canRedo || saving}
                  onPress={() => editAction("redo")}
                >
                  <RotateIcon className="size-4" /> Redo
                </Button>
              </div>

              <div className="mt-2 grid grid-cols-2 gap-1 border-t border-border pt-2">
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={saving}
                  onPress={() => void saveModified()}
                >
                  {saving ? <Spinner size="sm" /> : <SaveIcon className="size-4" />} Save copy
                </Button>
                <Button size="sm" variant="ghost" onPress={downloadOriginal}>
                  <DownloadIcon className="size-4" /> Original
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
          aria-label="Search in PDF"
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
          aria-label="Save edited PDF copy"
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
          aria-label="Download original PDF"
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
          <span>Could not save the PDF: {saveError}</span>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={saving}
            onPress={() => void saveModified()}
          >
            Try again
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

      <div className="flex min-h-0 flex-1 overflow-hidden">
        {sidebarOpen ? (
          <aside className="hidden w-64 shrink-0 border-r border-border bg-surface/80 lg:block xl:w-72">
            {sidebar}
          </aside>
        ) : null}
        <main className="relative min-w-0 flex-1 overflow-hidden bg-default/35">
          {!ready && !error && !passwordChallenge ? (
            <div className="absolute inset-0 z-20 grid place-items-center bg-background/70 backdrop-blur-sm">
              <div className="text-center">
                <Spinner size="lg" aria-label="Loading PDF" />
                <p className="mt-3 text-xs text-muted">
                  {loadingProgress === undefined
                    ? "Opening document"
                    : `Loading ${loadingProgress}%`}
                </p>
              </div>
            </div>
          ) : null}
          {passwordChallenge ? (
            <div className="absolute inset-0 z-30 grid place-items-center bg-background/80 p-5 backdrop-blur-md">
              <div className="w-full max-w-sm rounded-2xl border border-border bg-surface p-5 shadow-2xl">
                <p className="text-sm font-semibold">Protected PDF</p>
                <p className="mt-1 text-xs leading-5 text-muted">
                  {passwordChallenge.incorrect
                    ? "That password was not accepted. Try again."
                    : "Enter the document password to open this PDF."}
                </p>
                <InputGroup className="mt-4" variant="secondary">
                  <InputGroup.Input
                    autoFocus
                    type="password"
                    aria-label="PDF password"
                    placeholder="Document password"
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
                    Cancel
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
                    Unlock
                  </Button>
                </div>
              </div>
            </div>
          ) : null}
          {error ? (
            <div className="absolute inset-0 z-20 grid place-items-center p-6 text-center">
              <div className="max-w-lg">
                <p className="font-semibold">Unable to open this PDF</p>
                <p className="mt-2 text-sm text-muted">{error}</p>
                <Button
                  className="mt-4"
                  variant="secondary"
                  onPress={() => setLoadAttempt((value) => value + 1)}
                >
                  Try again
                </Button>
              </div>
            </div>
          ) : null}
          {/* biome-ignore lint/a11y/useSemanticElements: PDF.js requires a DIV scroll container. */}
          <div
            ref={containerRef}
            data-pdf-viewer-container
            role="region"
            aria-label="PDF document"
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
              <Spinner size="lg" aria-label="Saving PDF copy" />
            </div>
          ) : null}
        </main>
      </div>
      <AppDialog
        open={closePrompt}
        onOpenChange={setClosePrompt}
        isCloseDisabled={saving}
        isDismissable={!saving}
        title="Unsaved PDF changes"
        footer={
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="tertiary" isDisabled={saving} onPress={() => setClosePrompt(false)}>
              Keep editing
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
              Discard changes
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
              Save copy and close
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted">
          Your changes are only in this reader. Save an edited copy before closing, or discard them.
          The original file is unchanged.
        </p>
        {saveError ? (
          <p role="alert" className="mt-3 text-sm text-danger">
            Could not save the PDF: {saveError}
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
  return (
    <div className="flex items-center gap-2 text-xs">
      Brightness
      <Slider
        aria-label="PDF page brightness"
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
  const colors = tool === "highlight" ? HIGHLIGHT_COLORS : ANNOTATION_COLORS;
  return (
    <div
      role="toolbar"
      aria-label="Annotation settings"
      className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b border-border bg-surface px-3 py-2"
    >
      <span className="text-xs font-semibold">
        {tool === "highlight" ? "Highlight" : tool === "text" ? "Text" : "Draw"}
      </span>
      <fieldset
        aria-label="Annotation colors"
        className="flex items-center gap-1"
        disabled={disabled}
      >
        {colors.map((color) => (
          <Button
            key={color}
            isIconOnly
            size="sm"
            variant={settings.color === color ? "secondary" : "ghost"}
            aria-label={`Use annotation color ${color}`}
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
          Font size
          <Slider
            aria-label="PDF text font size"
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
            Stroke width
            <Slider
              aria-label="PDF ink stroke width"
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
            Opacity
            <Slider
              aria-label="PDF ink opacity"
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
  return (
    <div
      data-pdf-findbar
      className="z-20 flex min-h-12 shrink-0 items-center gap-1.5 border-b border-border bg-surface/95 px-2 backdrop-blur-xl sm:px-3"
    >
      <SearchIcon className="hidden size-4 text-muted sm:block" />
      <InputGroup className="max-w-md flex-1" variant="secondary">
        <InputGroup.Input
          autoFocus
          aria-label="Find in PDF"
          placeholder="Find in document"
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
            ? "No matches"
            : query
              ? `${count.current} / ${count.total}`
              : ""}
        </InputGroup.Suffix>
      </InputGroup>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label="Previous search result"
        isDisabled={!count.total}
        onPress={onPrevious}
      >
        <LeftIcon className="size-4" />
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label="Next search result"
        isDisabled={!count.total}
        onPress={onNext}
      >
        <RightIcon className="size-4" />
      </Button>
      <Button
        size="sm"
        variant={caseSensitive ? "secondary" : "ghost"}
        className="hidden min-w-8 px-2 text-xs font-semibold sm:inline-flex"
        aria-label="Match case"
        onPress={() => onCaseSensitive(!caseSensitive)}
      >
        Aa
      </Button>
      <Button
        size="sm"
        variant={wholeWord ? "secondary" : "ghost"}
        className="hidden px-2 text-xs sm:inline-flex"
        aria-label="Match whole words"
        onPress={() => onWholeWord(!wholeWord)}
      >
        Word
      </Button>
      <Button isIconOnly size="sm" variant="ghost" aria-label="Close PDF search" onPress={onClose}>
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
}) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-surface/75">
      <div className="border-b border-border px-4 py-3">
        <p className="truncate text-xs font-semibold">{file.name}</p>
        <p className="mt-0.5 text-[10px] text-muted">
          {document ? `${document.numPages} pages` : "Loading document"}
        </p>
      </div>
      <Tabs
        selectedKey={selectedTab}
        onSelectionChange={(key) => onTabChange(key === "outline" ? "outline" : "thumbnails")}
        className="flex min-h-0 flex-1 flex-col px-2 pt-2"
      >
        <Tabs.ListContainer>
          <Tabs.List aria-label="PDF sidebar" className="w-full">
            <Tabs.Tab id="thumbnails" className="flex-1 gap-1.5 text-xs">
              <MenuIcon className="size-3.5" /> Pages
            </Tabs.Tab>
            <Tabs.Tab id="outline" className="flex-1 gap-1.5 text-xs">
              <BookmarkIcon className="size-3.5" /> Outline
            </Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>
        <Tabs.Panel id="thumbnails" className="min-h-0 flex-1 overflow-y-auto py-2">
          {document ? (
            <div className="grid grid-cols-1 gap-2 px-1 pb-3">
              {Array.from({ length: document.numPages }, (_, index) => index + 1).map((page) => (
                <PdfThumbnail
                  key={page}
                  document={document}
                  pageNumber={page}
                  current={page === pageNumber}
                  onPress={() => {
                    onPage(page);
                    onNavigateMobile();
                  }}
                />
              ))}
            </div>
          ) : (
            <SidebarEmpty label="Preparing pages" />
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
            <SidebarEmpty label="This PDF has no outline" />
          )}
        </Tabs.Panel>
      </Tabs>
    </div>
  );
}

function PdfThumbnail({
  document,
  pageNumber,
  current,
  onPress,
}: {
  document: PDFDocumentProxy;
  pageNumber: number;
  current: boolean;
  onPress: () => void;
}) {
  const hostRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [visible, setVisible] = useState(false);
  const [ratio, setRatio] = useState(1.294);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) setVisible(true);
      },
      { rootMargin: "300px" },
    );
    observer.observe(host);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!visible || loaded) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    let active = true;
    let renderTask:
      | ReturnType<Awaited<ReturnType<PDFDocumentProxy["getPage"]>>["render"]>
      | undefined;
    void document
      .getPage(pageNumber)
      .then((page) => {
        if (!active) return;
        const natural = page.getViewport({ scale: 1 });
        setRatio(natural.height / natural.width);
        const cssWidth = 142;
        const scale = cssWidth / natural.width;
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const viewport = page.getViewport({ scale: scale * dpr });
        canvas.width = Math.ceil(viewport.width);
        canvas.height = Math.ceil(viewport.height);
        canvas.style.width = `${cssWidth}px`;
        canvas.style.height = `${Math.round(cssWidth * (natural.height / natural.width))}px`;
        renderTask = page.render({ canvas, viewport });
        return renderTask.promise;
      })
      .then(() => {
        if (active) setLoaded(true);
      })
      .catch(() => undefined);
    return () => {
      active = false;
      renderTask?.cancel();
    };
  }, [document, loaded, pageNumber, visible]);

  return (
    <div ref={hostRef} className="flex justify-center py-1">
      <Button
        variant={current ? "secondary" : "ghost"}
        className={cn(
          "h-auto w-full flex-col gap-2 rounded-xl px-2 py-2",
          current && "ring-1 ring-accent/40",
        )}
        aria-label={`Go to page ${pageNumber}`}
        onPress={onPress}
      >
        <div
          className="relative w-35.5 max-w-full overflow-hidden rounded-sm border border-border bg-white shadow-sm"
          style={{ aspectRatio: `1 / ${ratio}` }}
        >
          <canvas ref={canvasRef} className={cn("block max-w-full", !loaded && "opacity-0")} />
          {!loaded ? <div className="absolute inset-0 animate-pulse bg-default/20" /> : null}
        </div>
        <span className="text-[10px] font-medium tabular-nums text-muted">{pageNumber}</span>
      </Button>
    </div>
  );
}

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
  const title = item.title || "Untitled section";
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
            aria-label={`${expanded ? "Collapse" : "Expand"} ${title}`}
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

function formatBytes(value: number) {
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
