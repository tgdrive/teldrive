import { t } from "@/lib/localization";
import { Button, Input } from "@heroui/react";
import { useRef, useState } from "react";
import { SettingsSection } from "./settings-layout";
import { desktopRequest, isDesktop } from "@/lib/desktop";
import { nativeTextSave } from "@/lib/mobile";
import { parseRcloneConfig, privateRcloneOption, setRcloneOption } from "@/lib/rclone-config";

export function RcloneConfigEditor({ onSaved }: { onSaved: () => void }) {
  const [content, setContent] = useState("");
  const [remote, setRemote] = useState("");
  const [raw, setRaw] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const picker = useRef<HTMLInputElement>(null);
  let sections: ReturnType<typeof parseRcloneConfig> = [];
  let invalid = "";
  try {
    sections = parseRcloneConfig(content);
  } catch (error) {
    invalid = (error as Error).message;
  }
  const current = sections.find((item) => item.name === remote) ?? sections[0];
  const load = (text: string) => {
    const parsed = parseRcloneConfig(text);
    setContent(text);
    setRemote(parsed[0]?.name ?? "");
  };
  const action = async (operation: () => Promise<void>) => {
    setBusy(true);
    setMessage("");
    try {
      await operation();
    } catch (error) {
      setMessage((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const exportFile = async () => {
    if (await nativeTextSave("rclone.conf", content, "text/plain")) return;
    const url = URL.createObjectURL(new Blob([content], { type: "text/plain;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "rclone.conf";
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <SettingsSection
      title="rclone.conf personalizado"
      description={
        isDesktop()
          ? t(
              "Importa y edita todas las conexiones. Al guardar se valida el archivo y se conserva un respaldo.",
            )
          : t(
              "Importa un archivo de tu equipo, edita sus opciones y descarga la configuración actualizada para utilizarla con rclone.",
            )
      }
    >
      <div className="space-y-4 p-5">
        <div className="flex flex-wrap gap-2">
          <Button
            isDisabled={busy}
            onPress={() => {
              if (!isDesktop()) {
                picker.current?.click();
                return;
              }
              void action(async () => {
                const result = await desktopRequest<{ content?: string; locked?: boolean } | null>(
                  "rclone.config-import",
                );
                if (!result) return;
                if (result.locked) {
                  setMessage(
                    t(
                      "Archivo cifrado importado. Desbloquéalo en el explorador de rclone y pulsa Cargar configuración.",
                    ),
                  );
                  onSaved();
                  return;
                }
                load(result.content ?? "");
                onSaved();
                setMessage(t("Configuración importada; el archivo original se conserva."));
              });
            }}
          >
            Seleccionar rclone.conf
          </Button>
          {isDesktop() ? (
            <Button
              isDisabled={busy}
              onPress={() =>
                void action(async () => {
                  const result = await desktopRequest<{ content: string }>("rclone.config-read");
                  load(result.content);
                })
              }
            >
              {t("Cargar configuración")}
            </Button>
          ) : null}
          <Button
            isDisabled={busy || !content || Boolean(invalid)}
            onPress={() => void action(exportFile)}
          >
            {t("Exportar archivo editado")}
          </Button>
          {isDesktop() ? (
            <Button
              isDisabled={busy || !content || Boolean(invalid)}
              onPress={() =>
                void action(async () => {
                  await desktopRequest("rclone.config-save", { content });
                  onSaved();
                  setMessage(t("Configuración guardada. Respaldo: rclone.conf.previous."));
                })
              }
            >
              {t("Guardar en Desktop")}
            </Button>
          ) : null}
        </div>
        <input
          ref={picker}
          type="file"
          accept=".conf,text/plain"
          hidden
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file)
              void action(async () => {
                if (file.size > 2 * 1024 * 1024) throw new Error(t("El archivo supera 2 MB."));
                load(await file.text());
                setMessage(t("Archivo cargado localmente. Descárgalo cuando termines de editar."));
              });
            event.target.value = "";
          }}
        />
        {invalid ? (
          <p role="alert" className="text-danger">
            {invalid}
          </p>
        ) : null}
        {message ? (
          <p role="status" className="text-sm text-muted">
            {message}
          </p>
        ) : null}
        {sections.length ? (
          <label className="flex flex-wrap items-center gap-3">
            {t("Conexión")}
            <select
              aria-label="Conexión del archivo rclone.conf"
              className="min-h-11 rounded-lg border border-border bg-field-background px-3"
              value={current?.name}
              onChange={(event) => setRemote(event.target.value)}
            >
              {sections.map((section) => (
                <option key={section.name} value={section.name}>
                  {section.name}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          {current?.options.map((option) => (
            <label key={option.line} htmlFor={`rclone-option-${option.line}`} className="space-y-1">
              <span className="text-sm">{option.key}</span>
              <Input
                id={`rclone-option-${option.line}`}
                aria-label={`rclone ${option.key}`}
                type={privateRcloneOption(option.key) ? "password" : "text"}
                autoComplete="off"
                value={option.value}
                onChange={(event) =>
                  setContent(setRcloneOption(content, option, event.target.value))
                }
              />
            </label>
          ))}
        </div>
        <label className="flex min-h-11 items-center gap-2">
          <input type="checkbox" checked={raw} onChange={(event) => setRaw(event.target.checked)} />
          {t("Editar contenido completo")}
        </label>
        {raw ? (
          <textarea
            aria-label="Contenido de rclone.conf"
            spellCheck={false}
            className="min-h-64 w-full rounded-xl border border-border bg-field-background p-3 font-mono text-sm"
            value={content}
            onChange={(event) => setContent(event.target.value)}
          />
        ) : null}
      </div>
    </SettingsSection>
  );
}
