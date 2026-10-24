import { Button, cn, Modal, Spinner } from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import { lazy, Suspense, useEffect, useRef, useState } from "react";
import type { FileEntry } from "@/api/types";
import { fileContentUrl } from "@/features/files/download";
import { previewMedia, supportsCodePreview } from "@/features/files/preview-support";
import { readerKind } from "@/features/files/reader-support";
import { useFormatBytes } from "@/lib/format";
import { useAppLocale } from "@/lib/locale";

type ViewerKind = "image" | "video" | "audio" | "pdf" | "ebook" | "text";

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

export type FilePreviewSource = {
  /** URL the viewers load the file from. */
  contentUrl: string;
  /** URL of the header download link, which is a real anchor so its address can be copied. */
  downloadUrl: string;
  /**
   * Public shares only: whether the owner lets the video player read the audio tracks and subtitles
   * stored in the file. Visitors have no settings of their own, so this replaces the browser setting.
   */
  embeddedTracks?: boolean;
};

export function FilePreviewDialog({
  file,
  source,
  onOpenChange,
}: {
  file?: FileEntry;
  /** Where to load and download the file from; defaults to the signed-in file endpoints. */
  source?: FilePreviewSource;
  onOpenChange: (open: boolean) => void;
}) {
  const closeFrame = useRef<number>(undefined);
  const { t } = useAppLocale();
  const formatBytes = useFormatBytes();
  const contentUrl = file ? (source?.contentUrl ?? fileContentUrl(file)) : "";
  const downloadUrl = file ? (source?.downloadUrl ?? fileContentUrl(file, true)) : "";
  const kind = file ? viewerKind(file) : undefined;
  const isReader = kind === "pdf" || kind === "ebook";

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

  if (kind === "pdf") {
    return (
      <Modal.Backdrop
        isOpen
        onOpenChange={changeOpen}
        isDismissable={false}
        isKeyboardDismissDisabled
        variant="opaque"
        data-content-viewer
        className="bg-background"
      >
        <Modal.Container size="full" scroll="inside" className="h-dvh max-h-dvh p-0">
          <Modal.Dialog
            aria-label={file.name}
            className="h-dvh max-h-dvh w-screen max-w-none overflow-hidden rounded-none bg-background p-0 text-foreground"
          >
            <Modal.Heading className="sr-only">{file.name}</Modal.Heading>
            <Suspense fallback={<ViewerLoading label={t("preview.loadingPdf")} />}>
              <PdfReader
                key={file.id}
                file={file}
                url={contentUrl}
                downloadUrl={downloadUrl}
                onClose={() => changeOpen(false)}
              />
            </Suspense>
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
            <Suspense fallback={<ViewerLoading label={t("preview.loadingEpub")} />}>
              <EpubReader
                key={file.id}
                file={file}
                url={contentUrl}
                downloadUrl={downloadUrl}
                onClose={() => changeOpen(false)}
              />
            </Suspense>
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
        {/* Edge to edge, like the readers above. The `dark` scope keeps the viewer's header and
            controls legible in a light theme: the header's glass style is a fixed dark tint. */}
        <Modal.Dialog className="dark flex h-dvh max-h-dvh w-screen max-w-none flex-col overflow-hidden rounded-none bg-background p-0 text-foreground">
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
              aria-label={t("preview.close")}
              onPress={() => changeOpen(false)}
            >
              <CloseIcon className="size-5" />
            </Button>
            <div className="min-w-0 flex-1">
              <Modal.Heading className="truncate text-sm font-semibold tracking-[-0.01em]">
                {file.name}
              </Modal.Heading>
              <p className="truncate text-[11px] text-muted">
                {formatLabel(kind, t)} · {formatBytes(file.size || 0)}
              </p>
            </div>
            {/* A real link, not a button: right-click offers "Copy link address" and middle-click works. */}
            <a
              href={downloadUrl}
              download={file.name}
              aria-label={t("preview.download")}
              className={buttonVariants({ variant: isReader ? "ghost" : "secondary", size: "sm" })}
            >
              <DownloadIcon className="size-4" />
              <span className="hidden sm:inline">{t("preview.download")}</span>
            </a>
          </Modal.Header>
          <Modal.Body
            className={cn(
              // The body's default negative margin (room for focus rings) would push it 3px past
              // the window edge.
              "relative m-0 min-h-0 flex-1 overflow-hidden p-0",
              isReader
                ? "bg-background"
                : "bg-[radial-gradient(circle_at_50%_20%,color-mix(in_oklch,var(--muted-background)_70%,transparent),var(--background)_65%)]",
            )}
          >
            {kind === "image" ? <ImageViewer file={file} url={contentUrl} /> : null}
            {kind === "video" ? (
              <Suspense fallback={<ViewerLoading label={t("preview.loadingVideo")} />}>
                <VideoViewer file={file} url={contentUrl} embeddedTracks={source?.embeddedTracks} />
              </Suspense>
            ) : null}
            {kind === "audio" ? <AudioViewer file={file} url={contentUrl} /> : null}
            {kind === "text" ? <TextViewer url={contentUrl} /> : null}
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function ImageViewer({ file, url }: { file: FileEntry; url: string }) {
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const { t } = useAppLocale();
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
          aria-label={t("preview.zoomOut")}
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
          aria-label={t("preview.zoomIn")}
          onPress={() => setZoom((value) => Math.min(5, value + 0.25))}
        >
          <ZoomInIcon className="size-4" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={t("preview.rotate")}
          onPress={() => setRotation((value) => value + 90)}
        >
          <RotateIcon className="size-4" />
        </Button>
      </div>
    </div>
  );
}

function AudioViewer({ file, url }: { file: FileEntry; url: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="glass-panel w-full max-w-xl rounded-3xl p-8 text-center">
        <div className="mx-auto mb-6 grid size-28 place-items-center rounded-full border border-accent/20 bg-accent/10 text-4xl">
          ♪
        </div>
        <h3 className="truncate text-lg font-semibold">{file.name}</h3>
        {/* biome-ignore lint/a11y/useMediaCaption: user-provided audio does not have a separate caption resource */}
        <audio className="mt-7 w-full" src={url} controls />
      </div>
    </div>
  );
}

function TextViewer({ url }: { url: string }) {
  const [text, setText] = useState<string>();
  const [error, setError] = useState<string>();
  const { t } = useAppLocale();
  useEffect(() => {
    const controller = new AbortController();
    void fetch(url, { signal: controller.signal })
      .then((response) => response.text())
      .then((value) => setText(value.slice(0, 1_000_000)))
      .catch((reason: unknown) => {
        if (!controller.signal.aborted)
          setError(reason instanceof Error ? reason.message : t("preview.failed"));
      });
    return () => controller.abort();
  }, [url]);
  if (error) return <ViewerError message={error} />;
  if (text === undefined) return <ViewerLoading label={t("preview.loadingDoc")} />;
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
  const { t } = useAppLocale();
  return (
    <div className="grid h-full place-items-center p-6 text-center">
      <div>
        <p className="font-semibold">{t("preview.unable")}</p>
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
function formatLabel(kind: ViewerKind, t: (key: string) => string) {
  return {
    image: t("preview.kindImage"),
    video: t("preview.kindVideo"),
    audio: t("preview.kindAudio"),
    pdf: t("preview.kindPdf"),
    ebook: t("preview.kindEbook"),
    text: t("preview.kindText"),
  }[kind];
}
export function isPreviewable(file: FileEntry) {
  return viewerKind(file) !== undefined;
}
