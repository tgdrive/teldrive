import { VideoPlayer } from "@vplayer/react";
import "@vplayer/react/player.css";
import { useEffect, useRef } from "react";
import type { FileEntry } from "@/api/types";
import { previewMedia } from "@/features/files/preview-support";
import { spanishPlayerLabels } from "./player-labels";

export function VideoViewer({
  file,
  url,
  type,
  onError,
  subtitles,
}: {
  file: FileEntry;
  url: string;
  type?: string;
  onError?: () => void;
  subtitles?: { id: string; src: string; lang: string; label: string; default: boolean }[];
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
    <div
      ref={rootRef}
      onErrorCapture={onError}
      className="video-preview relative z-0 flex h-full min-h-0 w-full items-center justify-center overflow-hidden bg-black"
    >
      <div className="h-full min-h-0 w-full max-w-384 overflow-hidden">
        <VideoPlayer
          src={url}
          type={type || previewMedia(file)?.type || file.mimeType}
          title={file.name}
          autoPlay
          defaultHotkeys
          persistPreferences
          labels={spanishPlayerLabels}
          subtitles={subtitles}
          className="h-full max-h-full min-h-0 w-full overflow-hidden"
        />
      </div>
    </div>
  );
}
