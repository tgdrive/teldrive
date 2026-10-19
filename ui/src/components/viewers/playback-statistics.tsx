import { useEffect, useState, type RefObject } from "react";
import { apiFetch } from "@/api/client";
import type { components } from "@/api/schema";
import type { FileEntry } from "@/api/types";
import { t } from "@/lib/localization";

type Statistics = components["schemas"]["PlaybackStatistics"];
const bytes = (n: number) => `${(n / 1048576).toFixed(2)} MiB`;
export function PlaybackStatistics({file, root, converted, tracks}: {file: FileEntry; root: RefObject<HTMLDivElement | null>; converted: boolean; tracks: {kind: string; codec: string; language: string; title: string}[]}) {
  const [server, setServer] = useState<Statistics>();
  const [failure, setFailure] = useState("");
  const [local, setLocal] = useState<Record<string, string>>({});
  useEffect(() => {
    const cancel = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const sample = async () => {
      const media = root.current?.querySelector("video, audio") as HTMLVideoElement | null;
      if (media) {
        let ahead = 0;
        const ranges: string[] = [];
        for (let i = 0; i < media.buffered.length; i++) {
          const start = media.buffered.start(i), end = media.buffered.end(i);
          ranges.push(`${start.toFixed(1)}–${end.toFixed(1)} s`);
          if (start <= media.currentTime && end >= media.currentTime) ahead = end - media.currentTime;
        }
        const quality = media.getVideoPlaybackQuality?.();
        setLocal({
          "Estado": media.error ? `Error ${media.error.code}` : media.paused ? "Pausado" : media.readyState < 3 ? "Esperando datos" : "Reproduciendo",
          "Posición / duración": `${media.currentTime.toFixed(2)} / ${Number.isFinite(media.duration) ? media.duration.toFixed(2) : "—"} s`,
          "Resolución": media.videoWidth ? `${media.videoWidth} × ${media.videoHeight}` : "Solo audio / no disponible",
          "Búfer por delante": `${ahead.toFixed(2)} s`,
          "Rangos en búfer": ranges.join(" · ") || "Vacío",
          "Fotogramas / perdidos": quality ? `${quality.totalVideoFrames} / ${quality.droppedVideoFrames}` : "No disponible",
          "Velocidad / volumen": `${media.playbackRate}× / ${Math.round(media.volume * 100)}%${media.muted ? " (silenciado)" : ""}`,
          "Estado de datos / red": `${media.readyState} / ${media.networkState}`,
        });
      }
      try {
        const response = await apiFetch(`/v1/files/${encodeURIComponent(file.id)}/playback/statistics`, {signal:cancel.signal});
        const next = await response.json() as Statistics;
        if (!cancel.signal.aborted) { setServer(next); setFailure(""); }
      } catch {
        if (!cancel.signal.aborted) setFailure(t("Las estadísticas de Telegram requieren acceso como propietario y un servidor actualizado."));
      }
      if (!cancel.signal.aborted) timer = setTimeout(() => void sample(), 1000);
    };
    void sample();
    return () => {cancel.abort();clearTimeout(timer);};
  }, [file.id, root]);
  return <section aria-label={t("Estadísticas de reproducción")} className="max-h-[55vh] shrink-0 overflow-auto border-t border-white/20 bg-black p-4 text-xs text-white">
    <h3 className="mb-2 font-semibold">{t("Desarrollador · estadísticas en tiempo real")}</h3>
    <p className="mb-2 break-all">{file.name} · {file.mimeType || "—"} · {bytes(file.size || 0)} · {file.encryption ? t("Cifrado") : t("Sin cifrar")} · {converted ? t("Conversión compatible") : t("Reproducción original")}</p>
    <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-1">{Object.entries(local).map(([key,value]) => <div key={key} className="contents"><dt>{t(key)}</dt><dd>{t(value)}</dd></div>)}</dl>
    {tracks.length > 0 && <p className="mt-2">{t("Pistas")}: {tracks.map(track => `${track.kind} · ${track.codec} · ${track.language || "und"} ${track.title}`).join(" / ")}</p>}
    {failure && <p role="status" className="mt-3">{failure} {server && t("La última muestra está desactualizada.")}</p>}
    {server && <>
      <p className="mt-3">Telegram: {bytes(server.bytesPerSecond)}/s · {bytes(server.bytes)} · {server.activeStreams} {t("flujos abiertos")} · {t("Actualizado")}: {new Date(server.sampledAt).toLocaleTimeString()}</p>
      <p className="my-2 text-white/70">{t("Media móvil de 2 s. Suma las lecturas de este archivo, incluidas las búsquedas y otras reproducciones simultáneas. Una velocidad cero puede indicar reproducción desde el búfer.")}</p>
      <div className="overflow-x-auto"><table className="w-full text-left"><thead><tr>{["Nombre · alias · ID", "Velocidad / recibido", "Solicitudes activas / total / errores", "Latencia", "Último fragmento"].map(label => <th className="p-2" key={label}>{t(label)}</th>)}</tr></thead><tbody>{server.accounts.map(account => <tr key={account.id} className="border-t border-white/20">
        <td className="p-2"><strong>{account.name || (account.kind === "bot" ? "Bot" : t("Cuenta de Telegram"))}</strong><br/>{account.alias ? `@${account.alias.replace(/^@/, "")}` : "—"}<br/>ID {account.id}</td>
        <td className="p-2">{bytes(account.bytesPerSecond)}/s<br/>{bytes(account.bytes)}</td>
        <td className="p-2">{account.activeRequests} / {account.requests} / {account.errors}</td>
        <td className="p-2">{account.latencyMs.toFixed(1)} ms<br/>{account.lastSeen ? new Date(account.lastSeen).toLocaleTimeString() : "—"}</td>
        <td className="p-2">{t("Canal")}: {account.channelId}<br/>{t("Mensaje")}: {account.messageId}<br/>Offset: {account.offset}</td>
      </tr>)}</tbody></table></div>
      {server.accounts.length === 0 && <p>{t("Todavía no hay solicitudes de Telegram registradas para este archivo.")}</p>}
    </>}
  </section>;
}
