import { Spinner } from "@heroui/react";
import { VideoPlayer } from "@vplayer/react";
import "@vplayer/react/player.css";
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { FileEntry } from "@/api/types";
import { previewMedia } from "@/features/files/preview-support";
import {
  EmbeddedTracks,
  isMatroskaFile,
  type PlaybackSource,
} from "@/features/player/embedded-tracks";
import {
  embeddedTracksPlugin,
  subtitleEntries,
  TracksProvider,
} from "@/features/player/embedded-tracks-plugin";
import { useAppLocale } from "@/lib/locale";

export function VideoViewer({ file, url }: { file: FileEntry; url: string }) {
  // Matroska and WebM files are where multiple audio tracks and embedded subtitles live. Every
  // other format plays exactly as before.
  if (isMatroskaFile(file)) return <TrackedVideo file={file} url={url} />;
  return <Player file={file} src={url} />;
}

const PLUGINS = [embeddedTracksPlugin];

function TrackedVideo({ file, url }: { file: FileEntry; url: string }) {
  const { t, locale } = useAppLocale();
  const [ready, setReady] = useState<{ tracks: EmbeddedTracks; source: PlaybackSource }>();

  useEffect(() => {
    const tracks = new EmbeddedTracks(url, locale, (number) => t("player.trackNumber", { number }));
    const abort = new AbortController();
    setReady(undefined);
    // If remuxed playback breaks, carry on with the plain file from the same position.
    const stopWatching = tracks.onFallback((source) => setReady({ tracks, source }));
    void tracks.load(abort.signal).then((source) => {
      if (!abort.signal.aborted) setReady({ tracks, source });
    });
    return () => {
      abort.abort();
      stopWatching();
      tracks.dispose();
    };
  }, [url]);

  if (!ready) {
    return (
      <div className="grid h-full place-items-center">
        <Spinner size="lg" aria-label={t("preview.loadingVideo")} />
      </div>
    );
  }
  return (
    <TracksProvider value={ready.tracks}>
      <TrackedPlayer file={file} tracks={ready.tracks} source={ready.source} />
    </TracksProvider>
  );
}

function TrackedPlayer({
  file,
  tracks,
  source,
}: {
  file: FileEntry;
  tracks: EmbeddedTracks;
  source: PlaybackSource;
}) {
  const { t } = useAppLocale();
  const snapshot = useSyncExternalStore(tracks.subscribe, tracks.getSnapshot);
  const subtitles = useMemo(
    () => subtitleEntries(snapshot.subtitles, (label) => t("player.subtitleImage", { label })),
    [snapshot.subtitles],
  );
  return <Player file={file} src={source.src} type={source.type} subtitles={subtitles} tracked />;
}

function Player({
  file,
  src,
  type,
  subtitles,
  tracked,
}: {
  file: FileEntry;
  src: string;
  type?: string;
  subtitles?: ReturnType<typeof subtitleEntries>;
  tracked?: boolean;
}) {
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const root = rootRef.current;
    return () => {
      const video = root?.querySelector("video");
      if (!video) return;
      video.pause();
      video.removeAttribute("src");
      video.load();
    };
  }, []);

  return (
    <div ref={rootRef} className="flex h-full w-full items-center justify-center bg-black">
      {/* The player wraps itself in one div whose height is automatic; let it fill instead. */}
      <div className="h-full w-full max-w-384 [&>div]:h-full!">
        <VideoPlayer
          src={src}
          type={type || previewMedia(file)?.type || file.mimeType}
          title={file.name}
          subtitles={subtitles}
          plugins={tracked ? PLUGINS : undefined}
          autoPlay
          defaultHotkeys
          persistPreferences
          // Fill the window and letterbox the picture inside, instead of the player's own
          // fixed 16:9 box, which left a small strip on a phone and overran a short window.
          className="aspect-auto! h-full max-h-none! w-full"
        />
      </div>
    </div>
  );
}
