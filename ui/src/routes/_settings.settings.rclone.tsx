import { t } from "@/lib/localization";
import { Button, Input } from "@heroui/react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { RcloneConfigEditor } from "@/components/rclone-config-editor";
import { RcloneBrowser } from "@/components/rclone-browser";
import { isDesktop, desktopRequest } from "@/lib/desktop";
import { nativeTextSave } from "@/lib/mobile";

export const Route = createFileRoute("/_settings/settings/rclone")({ component: RcloneSettings });

function RcloneSettings() {
  const [host, setHost] = useState(window.location.origin);
  const [key, setKey] = useState("");
  const [remote, setRemote] = useState("teldrive");
  const [platform, setPlatform] = useState("windows");
  const [mount, setMount] = useState("T:");
  const [chunk, setChunk] = useState(512);
  const [concurrency, setConcurrency] = useState(4);
  const [encrypt, setEncrypt] = useState(false);
  const [hash, setHash] = useState(true);
  const [linkPassword, setLinkPassword] = useState("");
  const [connectionVersion, setConnectionVersion] = useState(0);
  const [saving, setSaving] = useState(false);
  let validHost = false;
  try {
    const url = new URL(host);
    validHost =
      ["http:", "https:"].includes(url.protocol) &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash;
  } catch {
    /* Display validation below. */
  }
  const valid =
    validHost &&
    /^[a-zA-Z][\w-]{0,63}$/.test(remote) &&
    key.trim().length > 0 &&
    !/[\r\n]/.test(host + key + linkPassword) &&
    chunk >= 64 &&
    chunk <= 2000 &&
    chunk % 16 === 0 &&
    concurrency >= 1 &&
    concurrency <= 32 &&
    Number.isInteger(concurrency);
  const config = `[${remote}]\ntype = teldrive\napi_host = ${host.replace(/\/$/, "")}\napi_key = ${key.trim()}\nchunk_size = ${chunk}Mi\nupload_concurrency = ${concurrency}\nencrypt_files = ${encrypt}\nhash_enabled = ${hash}\npage_size = 200\n${linkPassword ? `link_password = ${linkPassword}\n` : ""}`;
  const quote = (value: string) =>
    platform === "windows" ? `'${value.replace(/'/g, "''")}'` : `'${value.replace(/'/g, "'\\''")}'`;
  const executable = platform === "windows" ? ".\\rclone.exe" : "./rclone";
  const prefix = `${executable} --config ./rclone.conf`;
  const mountCommand =
    platform === "macos"
      ? "# El montaje requiere una compilación nativa con CGO y macFUSE; pendiente de validación en macOS."
      : `${prefix} mount ${quote(`${remote}:`)} ${quote(mount)} --vfs-cache-mode full --vfs-cache-max-size 10Gi --vfs-cache-max-age 24h`;
  const commands = `${prefix} lsf ${quote(`${remote}:`)}\n${mountCommand}\n${prefix} backend trash-list ${quote(`${remote}:`)}`;
  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t("Copiado al portapapeles"));
    } catch {
      toast.error(t("No se pudo copiar. Selecciona el texto y cópialo manualmente."));
    }
  };
  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Rclone"
        description={t(
          "Conecta Teldrive v2, transfiere archivos y monta una unidad con el cliente compatible.",
        )}
      />
      <SettingsSection
        title={t("Conexión de Teldrive v2")}
        description={t(
          "La clave se mantiene únicamente en esta página hasta que la cierres. El archivo descargado contiene tu credencial.",
        )}
      >
        <SettingsRow label={t("Servidor")}>
          <Input
            aria-label={t("Servidor de rclone")}
            value={host}
            onChange={(event) => setHost(event.target.value)}
          />
        </SettingsRow>
        <SettingsRow label={t("Nombre de la conexión")}>
          <Input
            aria-label={t("Nombre de la conexión")}
            value={remote}
            onChange={(event) => setRemote(event.target.value)}
          />
        </SettingsRow>
        <SettingsRow
          label={t("Clave de API")}
          description={t(
            "Crea una clave específica para este equipo y revócala cuando dejes de utilizarlo.",
          )}
        >
          <div className="space-y-2">
            <Input
              aria-label={t("Clave de API de rclone")}
              type="password"
              autoComplete="off"
              value={key}
              onChange={(event) => setKey(event.target.value)}
            />
            <Link to="/settings/api-keys" className="text-sm text-accent underline">
              {t("Administrar claves de API")}
            </Link>
          </div>
        </SettingsRow>
        <SettingsRow
          label={t("Tamaño de fragmento (MiB)")}
          description={t("Entre 64 y 2000, en múltiplos de 16.")}
        >
          <Input
            aria-label={t("Tamaño de fragmento")}
            type="number"
            min={64}
            max={2000}
            step={16}
            value={String(chunk)}
            onChange={(event) => setChunk(Number(event.target.value))}
          />
        </SettingsRow>
        <SettingsRow
          label={t("Subidas simultáneas")}
          description={t("Más concurrencia requiere más memoria.")}
        >
          <Input
            aria-label={t("Subidas simultáneas")}
            type="number"
            min={1}
            max={32}
            value={String(concurrency)}
            onChange={(event) => setConcurrency(Number(event.target.value))}
          />
        </SettingsRow>
        <SettingsRow label={t("Cifrado de Teldrive")}>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={encrypt}
              onChange={(event) => setEncrypt(event.target.checked)}
            />
            {t("Cifrar los archivos nuevos")}
          </label>
        </SettingsRow>
        <SettingsRow label={t("Integridad de los archivos")}>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={hash}
              onChange={(event) => setHash(event.target.checked)}
            />
            {t("Verificar mediante BLAKE3")}
          </label>
        </SettingsRow>
        <SettingsRow
          label={t("Contraseña de enlaces públicos")}
          description={t("Opcional; se aplica a los enlaces creados por rclone.")}
        >
          <Input
            type="password"
            autoComplete="new-password"
            aria-label={t("Contraseña de enlaces públicos")}
            value={linkPassword}
            onChange={(event) => setLinkPassword(event.target.value)}
          />
        </SettingsRow>
        <div className="flex flex-wrap gap-3 p-5">
          {isDesktop() ? (
            <Button
              isDisabled={!valid || saving || host.replace(/\/$/, "") !== window.location.origin}
              onPress={async () => {
                setSaving(true);
                try {
                  await desktopRequest("rclone.configure", {
                    apiKey: key.trim(),
                    chunkMiB: chunk,
                    concurrency,
                    encrypt,
                    hash,
                    linkPassword,
                  });
                  setConnectionVersion((value) => value + 1);
                  toast.success(t("Conexión guardada en este equipo"));
                } catch (cause) {
                  toast.error(
                    cause instanceof Error ? cause.message : t("No se pudo guardar la conexión."),
                  );
                } finally {
                  setSaving(false);
                }
              }}
            >
              {t("Guardar conexión en este equipo")}
            </Button>
          ) : null}
          <Button
            isDisabled={!valid}
            onPress={async () => {
              try {
                if (await nativeTextSave(config, "rclone.conf")) return;
              } catch (cause) {
                toast.error(
                  cause instanceof Error ? cause.message : t("No se pudo guardar el archivo."),
                );
                return;
              }
              const url = URL.createObjectURL(
                new Blob([config], { type: "text/plain;charset=utf-8" }),
              );
              const anchor = document.createElement("a");
              anchor.href = url;
              anchor.download = "rclone.conf";
              anchor.click();
              setTimeout(() => URL.revokeObjectURL(url), 1000);
            }}
          >
            Descargar rclone.conf
          </Button>
          <Button variant="secondary" isDisabled={!valid} onPress={() => void copy(config)}>
            {t("Copiar configuración")}
          </Button>
        </div>
        {!valid ? (
          <p className="px-5 pb-4 text-sm text-muted">
            {t(
              "Introduce una URL válida, una clave y un nombre de conexión sin espacios. Revisa también los límites de fragmentos y concurrencia.",
            )}
          </p>
        ) : null}
      </SettingsSection>
      {isDesktop() ? <RcloneBrowser connectionVersion={connectionVersion} /> : null}
      <SettingsSection
        title={t("Montar una unidad")}
        description={t(
          "Coloca rclone.conf junto al ejecutable compatible. Los comandos se ejecutan en tu equipo.",
        )}
      >
        <SettingsRow label={t("Sistema operativo")}>
          <select
            aria-label={t("Sistema operativo")}
            className="w-full rounded-lg border border-border bg-surface p-2"
            value={platform}
            onChange={(event) => {
              setPlatform(event.target.value);
              setMount(
                event.target.value === "windows"
                  ? "T:"
                  : event.target.value === "macos"
                    ? "/Users/usuario/Teldrive"
                    : "/home/usuario/Teldrive",
              );
            }}
          >
            <option value="windows">{t("Windows (PowerShell)")}</option>
            <option value="linux">{t("Linux")}</option>
            <option value="macos">macOS</option>
          </select>
        </SettingsRow>
        <SettingsRow label={t("Unidad o carpeta de montaje")}>
          <Input
            aria-label={t("Carpeta de montaje")}
            value={mount}
            onChange={(event) => setMount(event.target.value)}
          />
        </SettingsRow>
        <div className="space-y-3 p-5">
          <p className="text-sm text-muted">
            {platform === "windows"
              ? t(
                  "Windows requiere el controlador WinFsp para montar una unidad. Puedes usar las transferencias y la UI sin ese controlador.",
                )
              : platform === "macos"
                ? t(
                    "Las transferencias funcionan en macOS. El montaje necesita macFUSE y una compilación nativa adicional; todavía no está validado en esta edición.",
                  )
                : t("Linux requiere FUSE para el montaje.")}
          </p>
          <pre className="overflow-x-auto rounded-lg bg-default/30 p-4 text-xs">
            <code>{commands}</code>
          </pre>
          <Button variant="secondary" onPress={() => void copy(commands)}>
            {t("Copiar comandos")}
          </Button>
          <p className="text-sm text-muted">
            {t(
              "La edición 1.73.1 usa access_token. Para v2 utiliza el rclone de esta edición con api_key.",
            )}
          </p>
        </div>
      </SettingsSection>
      <RcloneConfigEditor onSaved={() => setConnectionVersion((value) => value + 1)} />
    </div>
  );
}
