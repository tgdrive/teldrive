import { createFileRoute } from "@tanstack/react-router";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { buildInfo } from "@/lib/version";
import { isDesktop } from "@/lib/desktop";

export const Route = createFileRoute("/_settings/settings/info")({ component: Information });

function Information() {
  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Información"
        description="Versión de esta interfaz, servidor conectado y recursos del proyecto."
      />
      <SettingsSection
        title="Teldrive v2"
        description="Edición comunitaria en español con temas, controles táctiles e integración de rclone."
      >
        <SettingsRow label="Versión de la interfaz">
          <span className="break-all text-sm">{buildInfo.version}</span>
        </SettingsRow>
        <SettingsRow label="Revisión del código">
          <span className="break-all font-mono text-xs">{buildInfo.commit}</span>
        </SettingsRow>
        <SettingsRow label="Fecha de compilación">
          <span className="text-sm">{new Date(buildInfo.date).toLocaleString("es")}</span>
        </SettingsRow>
        <SettingsRow label="Servidor conectado">
          <span className="break-all text-sm">{window.location.origin}</span>
        </SettingsRow>
        <SettingsRow label="Interfaz actual">
          <span className="text-sm">
            {isDesktop() ? "Teldrive Desktop para Windows" : "Interfaz web o app móvil"}
          </span>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title="Código y descargas"
        description="Consulta los binarios compatibles, el código fuente y las notas de cada edición."
      >
        <div className="flex flex-wrap gap-3 p-5">
          <a
            className="rounded-lg border border-border px-4 py-3 text-sm"
            href="https://github.com/webr0m/teldrive"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub
          </a>
          <a
            className="rounded-lg border border-border px-4 py-3 text-sm"
            href="https://gitlab.com/ridrogo/teldrive"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitLab
          </a>
          <a
            className="rounded-lg border border-border px-4 py-3 text-sm"
            href="https://github.com/tgdrive/teldrive/tree/v2"
            target="_blank"
            rel="noopener noreferrer"
          >
            Proyecto original
          </a>
        </div>
      </SettingsSection>
    </div>
  );
}
