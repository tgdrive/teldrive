import { Button } from "@heroui/react";
import { lazy, Suspense, useState, useEffect, useRef, useCallback } from "react";
import { apiFetch } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { CastDialog } from "./cast-dialog";
import type { AirPlayVideo } from "@/lib/casting";

const VideoViewer = lazy(() => import("./video-viewer").then((m) => ({ default: m.VideoViewer })));
export type PlaybackSession = { ticket: string; expiresAt: string; conversionAvailable: boolean };
export function playbackUrl(
  ticket: string,
  mode = "original",
  start = 0,
  audio?: number,
  subtitle?: number,
) {
  return `/api/v1/playback?${new URLSearchParams({ ticket, mode, start: String(start), ...(audio !== undefined ? { audio: String(audio) } : {}), ...(subtitle !== undefined ? { subtitle: String(subtitle) } : {}) })}`;
}
type Track = {
  index: number;
  kind: "audio" | "video" | "subtitle";
  codec: string;
  language: string;
  title: string;
  default: boolean;
  bitmap: boolean;
};
function trackLabel(track: Track) {
  return `${track.title || `${track.kind === "audio" ? "Audio" : "Subtítulos"} ${track.index + 1}`} · ${track.language || "sin idioma"} · ${track.codec}${track.default ? " (predeterminada)" : ""}${track.bitmap ? " · imagen" : ""}`;
}

export function CompatibleMedia({
  file,
  url,
  kind,
  session,
}: {
  file: FileEntry;
  url: string;
  kind: "audio" | "video";
  session?: PlaybackSession;
}) {
  const [grant, setGrant] = useState(session);
  const [source, setSource] = useState(url);
  const [converted, setConverted] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string>();
  const [start, setStart] = useState("0");
  const [tracks, setTracks] = useState<Track[]>([]);
  const [tracksNotice, setTracksNotice] = useState("");
  const [audio, setAudio] = useState<number>();
  const [subtitle, setSubtitle] = useState<number>();
  const [sourceStart, setSourceStart] = useState(0);
  const [castStart, setCastStart] = useState(0);
  const [castOpen, setCastOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const getVideo = () => root.current?.querySelector("video") as AirPlayVideo | null;
  const grantRef = useRef({ fileId: file.id, value: session });
  const ensureGrant = useCallback(async () => {
    if (grantRef.current.fileId !== file.id) grantRef.current = { fileId: file.id, value: session };
    const cached = grantRef.current.value;
    if (cached && new Date(cached.expiresAt).getTime() > Date.now()) {
      setGrant(cached);
      return cached;
    }
    const next = (await (
      await apiFetch(`/v1/files/${encodeURIComponent(file.id)}/playback`, { method: "POST" })
    ).json()) as PlaybackSession;
    grantRef.current.value = next;
    setGrant(next);
    return next;
  }, [file.id, session]);
  useEffect(() => {
    if (kind !== "video") return;
    let active = true;
    void ensureGrant()
      .then(async (next) => {
        const data = (await (
          await apiFetch(`/v1/playback/tracks?${new URLSearchParams({ ticket: next.ticket })}`)
        ).json()) as { available: boolean; tracks: Track[] };
        if (active) {
          setTracks(data.tracks);
          if (!data.available)
            setTracksNotice(
              "Instala FFprobe junto a FFmpeg en el servidor para detectar pistas de audio y subtítulos.",
            );
        }
      })
      .catch((cause) => {
        if (active) setTracksNotice(userMessage(cause));
      });
    return () => {
      active = false;
    };
  }, [ensureGrant, kind]);
  const chooseTracks = async (nextAudio: number | undefined, nextSubtitle: number | undefined) => {
    setAudio(nextAudio);
    setSubtitle(nextSubtitle);
    const selected = tracks.find((track) => track.index === nextSubtitle);
    if (nextAudio === undefined && !selected?.bitmap && !converted) return;
    setPending(true);
    setError(undefined);
    try {
      const next = await ensureGrant();
      if (!next.conversionAvailable)
        throw new Error(
          "La selección de pistas del contenedor necesita FFmpeg. Desktop lo incluye.",
        );
      const seconds = Math.min(86400, sourceStart + (getVideo()?.currentTime || 0));
      setSourceStart(seconds);
      setStart(String(Math.floor(seconds)));
      setSource(
        playbackUrl(
          next.ticket,
          "video",
          seconds,
          nextAudio,
          selected?.bitmap ? nextSubtitle : undefined,
        ),
      );
      setConverted(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : userMessage(cause));
    } finally {
      setPending(false);
    }
  };
  const convert = async () => {
    if (pending) return;
    const seconds = Number(start);
    if (!Number.isFinite(seconds) || seconds < 0 || seconds > 86400) {
      setError("Introduce una posición entre 0 y 86400 segundos.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      const next = await ensureGrant();
      setGrant(next);
      if (!next.conversionAvailable)
        throw new Error(
          "La reproducción compatible necesita FFmpeg en el servidor. El programa de Windows ya lo incluye.",
        );
      setSourceStart(seconds);
      setSource(
        playbackUrl(
          next.ticket,
          kind,
          seconds,
          audio,
          tracks.find((track) => track.index === subtitle)?.bitmap ? subtitle : undefined,
        ),
      );
      setConverted(true);
    } catch (cause) {
      setError(
        cause instanceof Error && cause.name !== "ApiError" ? cause.message : userMessage(cause),
      );
    } finally {
      setPending(false);
    }
  };
  return (
    <div ref={root} className="flex h-full min-h-0 w-full flex-col bg-black text-white">
      <div className="relative z-0 min-h-0 flex-1 overflow-hidden">
        {kind === "video" ? (
          <Suspense fallback={<p className="p-6">Cargando vídeo…</p>}>
            <VideoViewer
              file={file}
              url={source}
              type={converted ? "video/mp4" : undefined}
              subtitles={
                grant &&
                subtitle !== undefined &&
                !tracks.find((track) => track.index === subtitle)?.bitmap
                  ? [
                      {
                        id: `embedded-${subtitle}`,
                        src: `/api/v1/playback/subtitles?${new URLSearchParams({ ticket: grant.ticket, track: String(subtitle), start: String(converted ? sourceStart : 0) })}`,
                        lang: tracks.find((track) => track.index === subtitle)?.language || "und",
                        label: "Pista seleccionada",
                        default: true,
                      },
                    ]
                  : []
              }
              onError={() =>
                setError(
                  converted
                    ? "La conversión falló. Puedes reintentar o descargar el archivo."
                    : "El navegador no puede reproducir este formato o códec. Prueba la reproducción compatible.",
                )
              }
            />
          </Suspense>
        ) : (
          <div className="flex h-full items-center justify-center p-6">
            <div className="w-full max-w-xl text-center">
              <div className="mb-6 text-6xl">♪</div>
              <h3 className="truncate text-lg">{file.name}</h3>
              {/* biome-ignore lint/a11y/useMediaCaption: audio uploaded by the user has no caption resource */}
              <audio
                key={source}
                className="mt-7 w-full"
                src={source}
                controls
                onError={() =>
                  setError(
                    converted
                      ? "La conversión falló. Puedes reintentar o descargar el archivo."
                      : "Este audio necesita otro códec. Prueba la reproducción compatible.",
                  )
                }
              />
            </div>
          </div>
        )}
      </div>
      <div className="relative z-10 flex max-h-[45vh] shrink-0 flex-wrap items-center gap-3 overflow-y-auto border-t border-white/20 bg-black/90 p-3">
        {kind === "video" ? (
          <>
            <Button
              className="min-h-11"
              variant="secondary"
              isDisabled={pending}
              onPress={() => {
                void ensureGrant()
                  .then(() => {
                    setCastStart(sourceStart + (getVideo()?.currentTime || 0));
                    setCastOpen(true);
                  })
                  .catch((cause) => setError(userMessage(cause)));
              }}
            >
              Transmitir
            </Button>
            {tracks.some((track) => track.kind === "audio") ? (
              <label className="flex items-center gap-2 text-sm">
                Audio
                <select
                  aria-label="Pista de audio"
                  className="min-h-11 max-w-64 rounded-lg border border-white/30 bg-black px-3"
                  value={audio ?? ""}
                  disabled={pending}
                  onChange={(event) =>
                    void chooseTracks(
                      event.target.value === "" ? undefined : Number(event.target.value),
                      subtitle,
                    )
                  }
                >
                  <option value="">Predeterminada</option>
                  {tracks
                    .filter((track) => track.kind === "audio")
                    .map((track) => (
                      <option key={track.index} value={track.index}>
                        {trackLabel(track)}
                      </option>
                    ))}
                </select>
              </label>
            ) : null}
            {tracks.some((track) => track.kind === "subtitle") ? (
              <label className="flex items-center gap-2 text-sm">
                Subtítulos
                <select
                  aria-label="Pista de subtítulos"
                  className="min-h-11 max-w-64 rounded-lg border border-white/30 bg-black px-3"
                  value={subtitle ?? ""}
                  disabled={pending}
                  onChange={(event) =>
                    void chooseTracks(
                      audio,
                      event.target.value === "" ? undefined : Number(event.target.value),
                    )
                  }
                >
                  <option value="">Desactivados</option>
                  {tracks
                    .filter((track) => track.kind === "subtitle")
                    .map((track) => (
                      <option key={track.index} value={track.index}>
                        {trackLabel(track)}
                      </option>
                    ))}
                </select>
              </label>
            ) : null}
            {tracksNotice ? <p className="w-full text-xs text-white/70">{tracksNotice}</p> : null}
          </>
        ) : null}
        {error ? (
          <p role="alert" className="w-full text-sm text-amber-200">
            {error}
          </p>
        ) : null}
        <Button
          className="min-h-11"
          variant="secondary"
          isDisabled={pending}
          onPress={() => void convert()}
        >
          {pending
            ? "Preparando…"
            : converted
              ? "Reiniciar desde esta posición"
              : "Reproducción compatible"}
        </Button>
        <label className="flex items-center gap-2 text-sm">
          Comenzar en (segundos)
          <input
            aria-label="Comenzar en segundos"
            type="number"
            min="0"
            max="86400"
            className="min-h-11 w-28 rounded-lg border border-white/30 bg-black px-3"
            value={start}
            onChange={(event) => setStart(event.target.value)}
          />
        </label>
        {converted ? (
          <Button
            className="min-h-11"
            variant="secondary"
            onPress={() => {
              setSource(url);
              setConverted(false);
              setSourceStart(0);
              setAudio(undefined);
              setSubtitle(undefined);
              setError(undefined);
            }}
          >
            Versión original
          </Button>
        ) : null}
      </div>
      {kind === "video" ? (
        <CastDialog
          open={castOpen}
          onOpenChange={setCastOpen}
          grant={grant}
          title={file.name}
          mime={file.mimeType || "video/mp4"}
          video={getVideo}
          start={castStart}
          audio={audio}
          subtitle={subtitle}
          onAirPlaySource={(next) => {
            const element = getVideo();
            if (element) {
              element.src = next;
              element.load();
            }
            setSource(next);
            setConverted(true);
          }}
        />
      ) : null}
    </div>
  );
}
