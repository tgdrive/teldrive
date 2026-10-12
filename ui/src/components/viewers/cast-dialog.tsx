import { t } from "@/lib/localization";
import { Button } from "@heroui/react";
import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { AppDialog } from "../dialogs/app-dialog";
import { apiFetch } from "@/api/client";
import { userMessage } from "@/api/errors";
import { desktopRequest, isDesktop } from "@/lib/desktop";
import {
  googleCast,
  loopbackHost,
  playbackCastUrl,
  prepareGoogleCast,
  stopGoogleCast,
  type AirPlayVideo,
} from "@/lib/casting";
import type { PlaybackSession } from "./compatible-media";

type Device = { id: string; name: string; protocol: "dlna" };
export function CastDialog({
  open,
  onOpenChange,
  grant,
  title,
  mime,
  video,
  audio,
  subtitle,
  start,
  onAirPlaySource,
}: {
  open: boolean;
  onOpenChange: (value: boolean) => void;
  grant?: PlaybackSession;
  title: string;
  mime: string;
  video: () => AirPlayVideo | null;
  audio?: number;
  subtitle?: number;
  start: number;
  onAirPlaySource: (url: string) => void;
}) {
  const initialBase = loopbackHost(window.location.hostname)
    ? ""
    : `${window.location.origin}/api/v1`;
  const [base, setBase] = useState(initialBase);
  const [addresses, setAddresses] = useState<string[]>([]);
  const [compatible, setCompatible] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [devices, setDevices] = useState<Device[]>([]);
  const [activeDevice, setActiveDevice] = useState<string>();
  const [status, setStatus] = useState("");
  const ready = Boolean(grant && base && (!compatible || grant.conversionAvailable));
  let mediaUrl = "";
  try {
    if (ready && grant)
      mediaUrl = playbackCastUrl(
        base,
        grant.ticket,
        compatible ? "video" : "original",
        compatible ? audio : undefined,
        compatible ? subtitle : undefined,
        compatible ? start : 0,
      );
  } catch {
    /* Show validation on action. */
  }
  const execute = async (operation: () => Promise<void>) => {
    setPending(true);
    setError("");
    try {
      await operation();
    } catch (cause) {
      setError(
        cause instanceof Error && cause.name !== "ApiError" ? cause.message : userMessage(cause),
      );
    } finally {
      setPending(false);
    }
  };
  useEffect(() => {
    if (!open || !grant) return;
    let active = true;
    // Fetch a LAN-only relay on demand; the main server keeps its original bind address.
    apiFetch("/v1/casting/relay", {
      method: "POST",
      body: JSON.stringify({ ticket: grant.ticket }),
      headers: { "Content-Type": "application/json" },
    })
      .then((response) => response.json())
      .then((result: { addresses: string[] }) => {
        if (!active) return;
        setAddresses(result.addresses);
        if (!initialBase) setBase(result.addresses[0] ?? "");
      })
      .catch((cause) => {
        if (active && !initialBase) setError(userMessage(cause));
      });
    if (!isDesktop()) void prepareGoogleCast().catch(() => {});
    return () => {
      active = false;
    };
  }, [open, grant, initialBase]);
  const airplayAvailable = Boolean(video()?.webkitShowPlaybackTargetPicker);
  const browserPage = new URL("/cast", window.location.origin);
  if (mediaUrl)
    browserPage.hash = new URLSearchParams({
      url: mediaUrl,
      title,
      type: compatible ? "video/mp4" : mime,
    }).toString();
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("Transmitir vídeo")}
      description={t("Chromecast / Google Cast, AirPlay y televisores DLNA de la red local.")}
      className="sm:w-[min(94vw,44rem)]"
    >
      <div className="space-y-4">
        <label className="block space-y-1">
          <span className="text-sm">{t("Dirección accesible para el televisor")}</span>
          <input
            aria-label={t("Dirección para transmitir")}
            list="casting-addresses"
            value={base}
            onChange={(event) => setBase(event.target.value)}
            className="min-h-11 w-full rounded-lg border border-border bg-field-background px-3"
            placeholder="https://tu-servidor/api/v1"
          />
          <datalist id="casting-addresses">
            {[initialBase, ...addresses].filter(Boolean).map((address) => (
              <option key={address} value={address} />
            ))}
          </datalist>
        </label>
        <p className="text-sm text-muted">
          {t(
            "El televisor debe estar en la misma red que el servidor o poder acceder a su dirección pública. El enlace temporal solo permite reproducir este archivo y vence al terminar su sesión.",
          )}
        </p>
        <label className="flex min-h-11 items-center gap-2">
          <input
            type="checkbox"
            checked={compatible}
            onChange={(event) => setCompatible(event.target.checked)}
          />
          {t("Convertir a H.264/AAC compatible; aplicar las pistas seleccionadas")}
        </label>
        {compatible && grant && !grant.conversionAvailable ? (
          <p className="text-warning">
            {t(
              "El servidor necesita FFmpeg para convertir. Desktop ya lo incluye; en CLI instala FFmpeg o utiliza el formato original.",
            )}
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {isDesktop() ? (
            <Button
              isDisabled={!mediaUrl || pending}
              onPress={() =>
                void execute(async () => {
                  await desktopRequest("casting.open-browser", { url: browserPage.href });
                  setStatus(t("Transmisión abierta en Chrome."));
                })
              }
            >
              {t("Chromecast: abrir Chrome")}
            </Button>
          ) : (
            <Button
              isDisabled={!mediaUrl || pending}
              onPress={() =>
                void execute(async () => {
                  await googleCast(
                    mediaUrl,
                    compatible ? "video/mp4" : mime,
                    title,
                    compatible ? 0 : start,
                  );
                  video()?.pause();
                  setStatus(t("Vídeo enviado a Chromecast."));
                })
              }
            >
              {t("Chromecast / Google Cast")}
            </Button>
          )}
          <Button
            isDisabled={!mediaUrl || !airplayAvailable || pending}
            onPress={() => {
              if (!mediaUrl) return;
              onAirPlaySource(mediaUrl);
              video()?.webkitShowPlaybackTargetPicker?.();
              setStatus(t("Selecciona un dispositivo AirPlay."));
            }}
          >
            AirPlay
          </Button>
          <Button
            isDisabled={pending}
            onPress={() =>
              void execute(async () => {
                const result = (await (await apiFetch("/v1/casting/devices")).json()) as {
                  devices: Device[];
                };
                setDevices(result.devices);
                setStatus(
                  result.devices.length
                    ? t("Selecciona tu televisor DLNA.")
                    : t(
                        "No se encontraron televisores DLNA. Comprueba que estén encendidos en la red del servidor.",
                      ),
                );
              })
            }
          >
            {t("Buscar televisores DLNA")}
          </Button>
        </div>
        {!airplayAvailable ? (
          <p className="text-sm text-muted">
            {t(
              "AirPlay se utiliza desde Safari en iPhone, iPad o Mac. Abre el enlace del vídeo en Safari mediante el QR o cópialo; los controles del vídeo permiten elegir AirPlay.",
            )}
          </p>
        ) : null}
        {devices.map((device) => (
          <div
            key={device.id}
            className="flex items-center justify-between gap-2 rounded-xl border border-border p-3"
          >
            <span>{device.name}</span>
            <Button
              isDisabled={!mediaUrl || pending}
              onPress={() =>
                void execute(async () => {
                  await apiFetch(`/v1/casting/devices/${encodeURIComponent(device.id)}/play`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                      ticket: grant!.ticket,
                      serverUrl: base,
                      mode: compatible ? "video" : "original",
                      start,
                      ...(compatible && audio !== undefined ? { audio } : {}),
                      ...(compatible && subtitle !== undefined ? { subtitle } : {}),
                    }),
                  });
                  setActiveDevice(device.id);
                  video()?.pause();
                  setStatus(t("Reproduciendo en {0}", [device.name]));
                })
              }
            >
              {t("Transmitir a este TV")}
            </Button>
          </div>
        ))}
        {activeDevice ? (
          <div className="flex flex-wrap gap-2">
            {(
              [
                ["pause", t("Pausar")],
                ["play", t("Reanudar")],
                ["stop", t("Detener")],
              ] as const
            ).map(([command, label]) => (
              <Button
                key={command}
                isDisabled={pending}
                onPress={() =>
                  void execute(async () => {
                    await apiFetch(
                      `/v1/casting/devices/${encodeURIComponent(activeDevice)}/control`,
                      {
                        method: "POST",
                        headers: { "Content-Type": "application/json" },
                        body: JSON.stringify({ command }),
                      },
                    );
                    if (command === "stop") setActiveDevice(undefined);
                    setStatus(label);
                  })
                }
              >
                {label}
              </Button>
            ))}
          </div>
        ) : null}
        {mediaUrl ? (
          <div className="flex flex-wrap items-center gap-4 rounded-xl border border-border p-4">
            <div className="rounded-xl bg-white p-2">
              <QRCodeSVG value={mediaUrl} size={112} />
            </div>
            <div className="space-y-2">
              <p className="text-sm">{t("Abrir el vídeo en otro dispositivo")}</p>
              <Button
                onPress={() =>
                  void execute(async () => {
                    await navigator.clipboard.writeText(mediaUrl);
                    setStatus(t("Enlace temporal copiado."));
                  })
                }
              >
                {t("Copiar enlace del vídeo")}
              </Button>
              <Button
                onPress={() => {
                  stopGoogleCast();
                  setStatus(t("Transmisión de Chromecast detenida."));
                }}
              >
                {t("Detener Chromecast")}
              </Button>
            </div>
          </div>
        ) : null}
        {status ? (
          <p role="status" className="text-sm text-muted">
            {status}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        ) : null}
      </div>
    </AppDialog>
  );
}
