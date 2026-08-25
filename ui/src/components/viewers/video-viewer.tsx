import { VideoPlayer } from "@vplayer/react";
import "@vplayer/react/player.css";
import { useEffect, useRef } from "react";
import type { FileEntry } from "@/api/types";
import { previewMedia } from "@/features/files/preview-support";

export function VideoViewer({
  file,
  url,
  initialTime = 0,
  onProgress,
}: {
  file: FileEntry;
  url: string;
  initialTime?: number;
  onProgress?: (seconds: number) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const lastTime = useRef(initialTime);
  const lastReported = useRef(initialTime);
  const progressCallback = useRef(onProgress);

  useEffect(() => {
    progressCallback.current = onProgress;
  }, [onProgress]);

  useEffect(() => {
    lastTime.current = initialTime;
    lastReported.current = initialTime;
    const root = containerRef.current;
    if (!root || initialTime <= 0) return;

    let video: HTMLVideoElement | null = null;
    const restore = () => {
      if (!video || initialTime <= 0) return;
      const duration =
        Number.isFinite(video.duration) && video.duration > 0 ? video.duration : initialTime;
      video.currentTime = Math.min(initialTime, Math.max(duration - 0.25, 0));
    };
    const bind = () => {
      video = root.querySelector("video");
      if (!video) return false;
      if (video.readyState >= 1) restore();
      else video.addEventListener("loadedmetadata", restore, { once: true });
      return true;
    };
    const observer = new MutationObserver(() => {
      if (bind()) observer.disconnect();
    });
    if (!bind()) observer.observe(root, { childList: true, subtree: true });
    return () => {
      observer.disconnect();
      video?.removeEventListener("loadedmetadata", restore);
    };
  }, [file.id, initialTime]);

  useEffect(
    () => () => {
      if (lastTime.current > 0) progressCallback.current?.(lastTime.current);
    },
    [file.id],
  );

  return (
    <div ref={containerRef} className="flex h-full w-full items-center justify-center bg-black">
      <div className="w-full max-w-384">
        <VideoPlayer
          src={url}
          type={previewMedia(file)?.type || file.mimeType}
          title={file.name}
          autoPlay
          defaultHotkeys
          persistPreferences
          onTimeUpdate={(time) => {
            lastTime.current = time;
            if (Math.abs(time - lastReported.current) >= 5) {
              lastReported.current = time;
              progressCallback.current?.(time);
            }
          }}
          className="w-full"
        />
      </div>
    </div>
  );
}
