type CastSession = { loadMedia: (request: unknown) => Promise<void> };
type CastContext = {
  setOptions: (options: unknown) => void;
  requestSession: () => Promise<void>;
  getCurrentSession: () => CastSession | null;
  endCurrentSession: (stop: boolean) => void;
};
type CastWindow = Window & {
  __onGCastApiAvailable?: (available: boolean) => void;
  cast?: { framework: { CastContext: { getInstance: () => CastContext } } };
  chrome?: {
    cast?: {
      AutoJoinPolicy: { ORIGIN_SCOPED: string };
      media: {
        DEFAULT_MEDIA_RECEIVER_APP_ID: string;
        MediaInfo: new (url: string, mime: string) => { metadata?: { title: string } };
        GenericMediaMetadata: new () => { title: string };
        LoadRequest: new (media: unknown) => { currentTime?: number };
      };
    };
  };
};
let loading: Promise<void> | undefined;
export function prepareGoogleCast(): Promise<void> {
  const target = window as CastWindow;
  if (target.cast?.framework && target.chrome?.cast) return Promise.resolve();
  if (loading) return loading;
  loading = new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(
      () => reject(new Error("Google Cast no respondió. Utiliza Chrome y revisa la conexión.")),
      15000,
    );
    target.__onGCastApiAvailable = (available) => {
      clearTimeout(timeout);
      if (!available || !target.cast?.framework || !target.chrome?.cast) {
        reject(
          new Error("Chromecast requiere un navegador compatible con Google Cast, como Chrome."),
        );
        return;
      }
      resolve();
    };
    const loadScript = (url: string, onload?: () => void) => {
      const script = document.createElement("script");
      script.src = url;
      script.async = true;
      script.onload = () => onload?.();
      script.onerror = () => {
        clearTimeout(timeout);
        reject(new Error("No se pudo cargar Google Cast. Comprueba la conexión a Internet."));
      };
      document.head.append(script);
    };
    if (window.location.protocol === "http:") {
      // Google's bootstrap chooses protocol-relative URLs. Load its two official
      // SDK components over HTTPS when Teldrive is served on a local HTTP origin.
      let baseAvailable = false;
      let frameworkLoaded = false;
      const ready = () => {
        if (baseAvailable && frameworkLoaded && target.cast?.framework && target.chrome?.cast) {
          clearTimeout(timeout);
          resolve();
        }
      };
      const available = target.__onGCastApiAvailable;
      target.__onGCastApiAvailable = (value) => {
        if (!value) {
          available?.(false);
          return;
        }
        baseAvailable = true;
        ready();
      };
      loadScript("https://www.gstatic.com/cast/sdk/libs/sender/1.0/cast_framework.js", () => {
        frameworkLoaded = true;
        ready();
      });
      loadScript("https://www.gstatic.com/eureka/clank/cast_sender.js");
    } else {
      loadScript("https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1");
    }
  }).catch((error) => {
    loading = undefined;
    throw error;
  });
  return loading!;
}
export async function googleCast(url: string, mime: string, title: string, start = 0) {
  await prepareGoogleCast();
  const target = window as CastWindow;
  const api = target.chrome!.cast!;
  const context = target.cast!.framework.CastContext.getInstance();
  context.setOptions({
    receiverApplicationId: api.media.DEFAULT_MEDIA_RECEIVER_APP_ID,
    autoJoinPolicy: api.AutoJoinPolicy.ORIGIN_SCOPED,
  });
  if (!context.getCurrentSession()) await context.requestSession();
  const session = context.getCurrentSession();
  if (!session) throw new Error("No se seleccionó ningún dispositivo Chromecast.");
  const media = new api.media.MediaInfo(url, mime);
  const metadata = new api.media.GenericMediaMetadata();
  metadata.title = title;
  media.metadata = metadata;
  const request = new api.media.LoadRequest(media);
  request.currentTime = start;
  await session.loadMedia(request);
}
export function stopGoogleCast() {
  (window as CastWindow).cast?.framework.CastContext.getInstance().endCurrentSession(true);
}
export type AirPlayVideo = HTMLVideoElement & {
  webkitShowPlaybackTargetPicker?: () => void;
  webkitCurrentPlaybackTargetIsWireless?: boolean;
};
export function playbackCastUrl(
  base: string,
  ticket: string,
  mode: "original" | "video",
  audio?: number,
  subtitle?: number,
  start = 0,
) {
  const url = new URL(`${base.replace(/\/$/, "")}/playback`);
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password)
    throw new Error("Revisa la dirección del servidor para transmitir.");
  url.search = new URLSearchParams({
    ticket,
    mode,
    start: String(start),
    ...(audio !== undefined ? { audio: String(audio) } : {}),
    ...(subtitle !== undefined ? { subtitle: String(subtitle) } : {}),
  }).toString();
  return url.href;
}
export function loopbackHost(host: string) {
  return ["localhost", "127.0.0.1", "::1", "[::1]"].includes(host) || host.endsWith(".localhost");
}
