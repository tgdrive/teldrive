import { VideoPlayer } from "@vplayer/react";
import "@vplayer/react/player.css";
import { useEffect, useRef } from "react";
import type { FileEntry } from "@/api/types";
import { previewMedia } from "@/features/files/preview-support";

export function VideoViewer({ file, url }: { file: FileEntry; url: string }) {
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
      <div className="w-full max-w-384">
        <VideoPlayer
          src={url}
          type={previewMedia(file)?.type || file.mimeType}
          title={file.name}
          autoPlay
          defaultHotkeys
          persistPreferences
          className="w-full"
        />
      </div>
    </div>
  );
}
