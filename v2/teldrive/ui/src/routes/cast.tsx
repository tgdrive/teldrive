import { Button } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { googleCast, prepareGoogleCast, stopGoogleCast, type AirPlayVideo } from "@/lib/casting";

export const Route = createFileRoute("/cast")({ component: CastingPage });
function CastingPage() {
  const data = new URLSearchParams(window.location.hash.slice(1));
  const title = data.get("title") ?? "Vídeo de Teldrive";
  const type = data.get("type") ?? "video/mp4";
  let url = "";
  try {
    const parsed = new URL(data.get("url") ?? "");
    if (["http:", "https:"].includes(parsed.protocol) && !parsed.username && !parsed.password)
      url = parsed.href;
  } catch {
    /* Display missing link. */
  }
  useEffect(() => {
    void prepareGoogleCast().catch(() => {});
  }, []);
  const video = useRef<AirPlayVideo>(null);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  return (
    <main className="mx-auto max-w-5xl space-y-5 p-5">
      <header className="flex items-center gap-3">
        <img src="/images/teldrive.png" className="size-12" alt="" />
        <h1 className="text-xl font-semibold">Transmitir con Teldrive</h1>
      </header>
      <h2 className="break-words">{title}</h2>
      {url ? (
        <>
          <video
            ref={video}
            src={url}
            controls
            playsInline
            className="max-h-[70vh] w-full rounded-xl bg-black"
          >
            <track kind="captions" />
          </video>
          <div className="flex flex-wrap gap-3">
            <Button
              isDisabled={pending}
              onPress={() => {
                setPending(true);
                setMessage("");
                void prepareGoogleCast()
                  .then(() => googleCast(url, type, title, video.current?.currentTime ?? 0))
                  .then(() => {
                    video.current?.pause();
                    setMessage("Vídeo enviado a Chromecast.");
                  })
                  .catch((error) =>
                    setMessage(error instanceof Error ? error.message : "No se pudo transmitir."),
                  )
                  .finally(() => setPending(false));
              }}
            >
              Chromecast / Google Cast
            </Button>
            <Button
              onPress={() => {
                if (video.current?.webkitShowPlaybackTargetPicker)
                  video.current.webkitShowPlaybackTargetPicker();
                else setMessage("AirPlay requiere Safari en un dispositivo Apple.");
              }}
            >
              AirPlay
            </Button>
            <Button onPress={stopGoogleCast}>Detener transmisión</Button>
          </div>
        </>
      ) : (
        <p role="alert">
          El enlace de reproducción no es válido. Vuelve a abrir Transmitir desde el vídeo.
        </p>
      )}
      {message ? <p role="status">{message}</p> : null}
      <p className="text-sm text-muted">
        El enlace temporal utiliza tu sesión de reproducción del servidor y solo da acceso a este
        vídeo.
      </p>
    </main>
  );
}
