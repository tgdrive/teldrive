import { Button, cn, Modal, Spinner } from "@heroui/react";
import { lazy, Suspense, useEffect, useRef, useState } from "react";
import type { FileEntry } from "@/api/types";
import { previewMedia, supportsCodePreview } from "@/features/files/preview-support";
import { readerKind } from "@/features/files/reader-support";
import {
  getViewState,
  putViewState,
  type ViewerKind,
  type ViewState,
} from "@/features/files/view-state";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import RotateIcon from "~icons/gravity-ui/arrow-rotate-left";
import ZoomOutIcon from "~icons/gravity-ui/magnifier-minus";
import ZoomInIcon from "~icons/gravity-ui/magnifier-plus";
import CloseIcon from "~icons/gravity-ui/xmark";

const VideoViewer = lazy(() =>
  import("@/components/viewers/video-viewer").then((module) => ({ default: module.VideoViewer })),
);
const PdfReader = lazy(() =>
  import("@/components/viewers/pdf-reader").then((module) => ({ default: module.PdfReader })),
);
const EpubReader = lazy(() =>
  import("@/components/viewers/epub-reader").then((module) => ({ default: module.EpubReader })),
);

export function FilePreviewDialog({
  file,
  onOpenChange,
}: {
  file?: FileEntry;
  onOpenChange: (open: boolean) => void;
}) {
  const [state, setState] = useState<ViewState>();
  const [loadingState, setLoadingState] = useState(false);
  const [stateLoadedFor, setStateLoadedFor] = useState<string>();
  const saveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const closeFrame = useRef<number>(undefined);
  const contentUrl = file ? `/api/v1/files/${encodeURIComponent(file.id)}/content` : "";
  const kind = file ? viewerKind(file) : undefined;
  const isReader = kind === "pdf" || kind === "ebook";

  useEffect(() => {
    if (!file) return;
    if (kind === "video") {
      clearTimeout(saveTimer.current);
      setLoadingState(false);
      setState(undefined);
      setStateLoadedFor(file.id);
      return () => clearTimeout(saveTimer.current);
    }
    const controller = new AbortController();
    setLoadingState(true);
    setStateLoadedFor(undefined);
    setState(undefined);
    void getViewState(file.id, controller.signal)
      .then(setState)
      .catch(() => setState(undefined))
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoadingState(false);
          setStateLoadedFor(file.id);
        }
      });
    return () => {
      controller.abort();
      clearTimeout(saveTimer.current);
    };
  }, [file, kind]);

  if (!file || !kind) return null;

  const changeOpen = (open: boolean) => {
    if (open || !isReader) {
      onOpenChange(open);
      return;
    }
    cancelAnimationFrame(closeFrame.current || 0);
    closeFrame.current = requestAnimationFrame(() => {
      closeFrame.current = requestAnimationFrame(() => onOpenChange(false));
    });
  };

  const savePosition = (
    position: Record<string, unknown>,
    preferences: Record<string, unknown> = {},
  ) => {
    setState((current) => ({
      fileId: file.id,
      kind,
      position,
      preferences,
      bookmarks: current?.bookmarks || [],
      updatedAt: new Date().toISOString(),
    }));
    clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      void putViewState(file.id, kind, position, preferences, state?.bookmarks).catch(
        () => undefined,
      );
    }, 800);
  };

  if (kind === "pdf") {
    return (
      <Modal.Backdrop
        isOpen
        onOpenChange={changeOpen}
        isDismissable
        variant="opaque"
        data-content-viewer
        className="bg-background"
      >
        <Modal.Container size="full" scroll="inside" className="h-dvh max-h-dvh p-0">
          <Modal.Dialog className="h-dvh max-h-dvh w-screen max-w-none overflow-hidden rounded-none bg-background p-0 text-foreground">
            <Modal.Heading className="sr-only">{file.name}</Modal.Heading>
            {stateLoadedFor === file.id ? (
              <Suspense fallback={<ViewerLoading label="Loading PDF engine" />}>
                <PdfReader
                  key={file.id}
                  file={file}
                  url={contentUrl}
                  state={state}
                  onPosition={savePosition}
                  onClose={() => changeOpen(false)}
                />
              </Suspense>
            ) : (
              <ViewerLoading label="Preparing PDF reader" />
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    );
  }

  if (kind === "ebook") {
    return (
      <Modal.Backdrop
        isOpen
        isDismissable={false}
        variant="opaque"
        data-content-viewer
        className="bg-background"
      >
        <Modal.Container size="full" scroll="inside" className="h-dvh max-h-dvh p-0">
          <Modal.Dialog className="h-dvh max-h-dvh w-screen max-w-none overflow-hidden rounded-none bg-background p-0 text-foreground">
            <Modal.Heading className="sr-only">{file.name}</Modal.Heading>
            {stateLoadedFor === file.id ? (
              <Suspense fallback={<ViewerLoading label="Loading EPUB reader" />}>
                <EpubReader
                  key={file.id}
                  file={file}
                  url={contentUrl}
                  state={state}
                  onPosition={savePosition}
                  onClose={() => changeOpen(false)}
                />
              </Suspense>
            ) : (
              <ViewerLoading label="Preparing EPUB reader" />
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    );
  }

  return (
    <Modal.Backdrop
      isOpen
      onOpenChange={changeOpen}
      isDismissable
      variant="opaque"
      data-content-viewer
      className="bg-background"
    >
      <Modal.Container size="full" scroll="inside" className="h-dvh max-h-dvh p-0">
        <Modal.Dialog className="flex h-dvh max-h-dvh w-screen max-w-none flex-col overflow-hidden rounded-none bg-background text-foreground">
          <Modal.Header
            data-reader-header={isReader || undefined}
            className={cn(
              "relative z-30 flex h-16 shrink-0 flex-row items-center gap-3 border-x-0 border-t-0 px-3 py-0 sm:px-5",
              isReader ? "border-b border-border bg-surface/90 backdrop-blur-xl" : "glass-panel",
            )}
          >
            <Button
              isIconOnly
              variant="ghost"
              size="sm"
              aria-label="Close viewer"
              onPress={() => changeOpen(false)}
            >
              <CloseIcon className="size-5" />
            </Button>
            <div className="min-w-0 flex-1">
              <Modal.Heading className="truncate text-sm font-semibold tracking-[-0.01em]">
                {file.name}
              </Modal.Heading>
              <p className="truncate text-[11px] text-muted">
                {formatLabel(kind)} · {formatBytes(file.size || 0)}
              </p>
            </div>
            {loadingState ? <Spinner size="sm" aria-label="Loading saved position" /> : null}
            <Button
              variant={isReader ? "ghost" : "secondary"}
              size="sm"
              onPress={() => download(file)}
            >
              <DownloadIcon className="size-4" />
              <span className="hidden sm:inline">Download</span>
            </Button>
          </Modal.Header>
          <Modal.Body
            className={cn(
              "relative min-h-0 flex-1 overflow-hidden p-0",
              isReader
                ? "bg-background"
                : "bg-[radial-gradient(circle_at_50%_20%,color-mix(in_oklch,var(--muted-background)_70%,transparent),var(--background)_65%)]",
            )}
          >
            {stateLoadedFor !== file.id ? <ViewerLoading label="Preparing viewer" /> : null}
            {stateLoadedFor === file.id && kind === "image" ? (
              <ImageViewer file={file} url={contentUrl} />
            ) : null}
            {stateLoadedFor === file.id && kind === "video" ? (
              <Suspense fallback={<ViewerLoading label="Loading video player" />}>
                <VideoViewer file={file} url={contentUrl} />
              </Suspense>
            ) : null}
            {stateLoadedFor === file.id && kind === "audio" ? (
              <AudioViewer
                file={file}
                url={contentUrl}
                initialTime={numberValue(state?.position.seconds)}
                onProgress={(seconds) => savePosition({ seconds })}
              />
            ) : null}
            {stateLoadedFor === file.id && kind === "text" ? <TextViewer url={contentUrl} /> : null}
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function ImageViewer({ file, url }: { file: FileEntry; url: string }) {
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  return (
    <div className="relative flex h-full items-center justify-center overflow-auto p-5 sm:p-10">
      <img
        src={url}
        alt={file.name}
        draggable={false}
        className="max-h-full max-w-full select-none object-contain shadow-2xl transition-transform duration-200"
        style={{ transform: `scale(${zoom}) rotate(${rotation}deg)` }}
      />
      <div className="glass-panel absolute bottom-4 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-full p-1.5">
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Zoom out"
          onPress={() => setZoom((value) => Math.max(0.25, value - 0.25))}
        >
          <ZoomOutIcon className="size-4" />
        </Button>
        <Button size="sm" variant="ghost" onPress={() => setZoom(1)}>
          {Math.round(zoom * 100)}%
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Zoom in"
          onPress={() => setZoom((value) => Math.min(5, value + 0.25))}
        >
          <ZoomInIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="Rotate image"
          onPress={() => setRotation((value) => value + 90)}
        >
          <RotateIcon className="size-4" />
        </Button>
      </div>
    </div>
  );
}

function AudioViewer({
  file,
  url,
  initialTime,
  onProgress,
}: {
  file: FileEntry;
  url: string;
  initialTime: number;
  onProgress: (seconds: number) => void;
}) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="glass-panel w-full max-w-xl rounded-3xl p-8 text-center">
        <div className="mx-auto mb-6 grid size-28 place-items-center rounded-full border border-accent/20 bg-accent/10 text-4xl">
          ♪
        </div>
        <h3 className="truncate text-lg font-semibold">{file.name}</h3>
        {/* biome-ignore lint/a11y/useMediaCaption: user-provided audio does not have a separate caption resource */}
        <audio
          className="mt-7 w-full"
          src={url}
          controls
          onLoadedMetadata={(event) => {
            if (initialTime > 0) event.currentTarget.currentTime = initialTime;
          }}
          onTimeUpdate={(event) => onProgress(event.currentTarget.currentTime)}
        />
      </div>
    </div>
  );
}

function TextViewer({ url }: { url: string }) {
  const [text, setText] = useState<string>();
  const [error, setError] = useState<string>();
  useEffect(() => {
    const controller = new AbortController();
    void fetch(url, { signal: controller.signal })
      .then((response) => response.text())
      .then((value) => setText(value.slice(0, 1_000_000)))
      .catch((reason: unknown) => {
        if (!controller.signal.aborted)
          setError(reason instanceof Error ? reason.message : "Preview failed");
      });
    return () => controller.abort();
  }, [url]);
  if (error) return <ViewerError message={error} />;
  if (text === undefined) return <ViewerLoading label="Loading document" />;
  return (
    <div className="h-full overflow-auto p-4 sm:p-8">
      <pre className="mx-auto min-h-full max-w-5xl whitespace-pre-wrap rounded-2xl border border-border bg-surface p-5 font-mono text-xs leading-6 shadow-xl sm:p-8">
        {text}
      </pre>
    </div>
  );
}
function ViewerLoading({ label }: { label: string }) {
  return (
    <div className="grid h-full place-items-center">
      <div className="text-center">
        <Spinner size="lg" aria-label={label} />
        <p className="mt-3 text-xs text-muted">{label}</p>
      </div>
    </div>
  );
}
function ViewerError({ message }: { message: string }) {
  return (
    <div className="grid h-full place-items-center p-6 text-center">
      <div>
        <p className="font-semibold">Unable to open this file</p>
        <p className="mt-2 max-w-lg text-sm text-muted">{message}</p>
      </div>
    </div>
  );
}
function viewerKind(file: FileEntry): ViewerKind | undefined {
  const reader = readerKind(file);
  if (reader) return reader;
  const media = previewMedia(file);
  if (media) return media.kind;
  if (supportsCodePreview(file)) return "text";
}
function formatLabel(kind: ViewerKind) {
  return {
    image: "Image",
    video: "Video",
    audio: "Audio",
    pdf: "PDF document",
    ebook: "Ebook",
    text: "Text document",
  }[kind];
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

export function isPreviewable(file: FileEntry) {
  return viewerKind(file) !== undefined;
}
