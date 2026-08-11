import { VideoPlayer } from "@vplayer/react";
import "@vplayer/react/player.css";
import type { FileEntry } from "@/api/types";
import { previewMedia } from "@/features/files/preview-support";

export function VideoViewer({ file, url }: { file: FileEntry; url: string }) {
  return (
    <div className="flex h-full w-full items-center justify-center bg-black">
      <div className="w-full max-w-384">
        <VideoPlayer
          src={url}
          type={previewMedia(file)?.type || file.mimeType}
          title={file.name}
          autoPlay
          defaultHotkeys
          persistPreferences
          playbackProgress={{ id: file.id }}
          className="w-full"
        />
      </div>
    </div>
  );
}
