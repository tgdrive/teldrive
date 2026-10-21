import { VideoPlayer } from "@vplayer/react";
import "@vplayer/react/player.css";
import { useEffect, useRef } from "react";
import type { FileEntry } from "@/api/types";
import { previewMedia } from "@/features/files/preview-support";
import { useEmbeddedTracks } from "@/lib/player-settings";

export function VideoViewer({ file, url }: { file: FileEntry; url: string }) {
  const rootRef = useRef<HTMLDivElement>(null);
  // Off unless it is turned on in Settings → Appearance; while it is off the player does not look
  // inside the file at all.
  const [embeddedTracks] = useEmbeddedTracks();

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
          src={url}
          type={previewMedia(file)?.type || file.mimeType}
          title={file.name}
          autoPlay
          defaultHotkeys
          persistPreferences
          embeddedTracks={embeddedTracks}
          // Fill the window and letterbox the picture inside, instead of the player's own
          // fixed 16:9 box, which left a small strip on a phone and overran a short window.
          className="aspect-auto! h-full max-h-none! w-full"
        />
      </div>
    </div>
  );
}
