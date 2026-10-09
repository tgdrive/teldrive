import { Button } from "@heroui/react";
import { lazy, Suspense, useState } from "react";
import { apiFetch } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";

const VideoViewer = lazy(() => import("./video-viewer").then((m) => ({ default: m.VideoViewer })));
export type PlaybackSession = { ticket: string; expiresAt: string; conversionAvailable: boolean };
export function playbackUrl(ticket: string, mode = "original", start = 0) {
  return `/api/v1/playback?${new URLSearchParams({ ticket, mode, start: String(start) })}`;
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
      const next =
        grant ??
        ((await (
          await apiFetch(`/v1/files/${encodeURIComponent(file.id)}/playback`, { method: "POST" })
        ).json()) as PlaybackSession);
      setGrant(next);
      if (!next.conversionAvailable)
        throw new Error(
          "La reproducción compatible necesita FFmpeg en el servidor. El programa de Windows ya lo incluye.",
        );
      setSource(playbackUrl(next.ticket, kind, seconds));
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
    <div className="flex h-full min-h-0 w-full flex-col bg-black text-white">
      <div className="min-h-0 flex-1">
        {kind === "video" ? (
          <Suspense fallback={<p className="p-6">Cargando vídeo…</p>}>
            <VideoViewer
              key={source}
              file={file}
              url={source}
              type={converted ? "video/mp4" : undefined}
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
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-t border-white/20 bg-black/90 p-3">
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
              setError(undefined);
            }}
          >
            Versión original
          </Button>
        ) : null}
      </div>
    </div>
  );
}
