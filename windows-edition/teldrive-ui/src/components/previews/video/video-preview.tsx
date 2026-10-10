import type Artplayer from "artplayer";
import type { Option } from "artplayer";
import spanish from "artplayer/i18n/es";
import { memo, useMemo, useRef } from "react";
import { Player } from "./art-player";

interface VideoPlayerProps {
  url: string;
}
const VideoPlayer = memo(({ url, ...props }: VideoPlayerProps) => {
  const artInstance = useRef<Artplayer | null>(null);
  const artOptions: Option = useMemo(() => ({
    container: "",
    url,
    lang: "es",
    i18n: { es: spanish },
    volume: 0.6,
    muted: false,
    autoplay: false,
    pip: true,
    autoSize: false,
    autoMini: false,
    screenshot: true,
    setting: true,
    flip: true,
    playbackRate: true,
    aspectRatio: true,
    fullscreen: true,
    fullscreenWeb: true,
    mutex: true,
    backdrop: true,
    hotkey: true,
    playsInline: true,
    autoPlayback: true,
    airplay: true,
    lock: true,
    fastForward: true,
    autoOrientation: true,
    moreVideoAttr: {
      playsInline: true,
    },
  }), [url]);

  return (
    <div className="space-y-3">
    <Player
      style={{ aspectRatio: "16 /9" }}
      ref={artInstance}
      option={artOptions}
      {...props}
    />
    <p className="text-xs text-center text-white/60 px-4">La reproducción depende del formato y códec admitido por tu navegador. Si no se reproduce, descarga el archivo para abrirlo en VLC.</p>
    </div>
  );
});

export default memo(VideoPlayer);
