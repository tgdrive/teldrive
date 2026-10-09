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
}: {
  file: FileEntry;
  url: string;
  type?: string;
  onError?: () => void;
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
      className="video-preview flex h-full w-full items-center justify-center bg-black"
    >
      <div className="w-full max-w-384">
        <VideoPlayer
          src={url}
          type={type || previewMedia(file)?.type || file.mimeType}
          title={file.name}
          autoPlay
          defaultHotkeys
          persistPreferences
          labels={spanishPlayerLabels}
          className="w-full"
        />
      </div>
    </div>
  );
}
