import { Button } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useTheme, visualThemes, type ThemeMode } from "@/lib/theme";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";

export const Route = createFileRoute("/_settings/settings/appearance")({ component: AppearanceSettings });
function AppearanceSettings() {
  const preferences = useTheme();
  return <div className="space-y-6">
    <SettingsPageHeader title="Apariencia" description="Personaliza los colores, el modo y la densidad de tu unidad." />
    <SettingsSection title="Temas visuales" description="Los cambios se aplican inmediatamente y se guardan en este navegador.">
      <SettingsRow label="Modo">
        <div className="grid grid-cols-3 gap-2">{(["light", "dark", "system"] as ThemeMode[]).map((mode, index) => <Button key={mode} variant={preferences.mode === mode ? "primary" : "secondary"} onPress={() => preferences.setTheme(mode)}>{["Claro", "Oscuro", "Automático"][index]}</Button>)}</div>
      </SettingsRow>
      <SettingsRow label="Paleta de colores">
        <div className="grid grid-cols-2 gap-2">{visualThemes.map(theme => <Button key={theme.id} variant={preferences.palette === theme.id && !preferences.customColor ? "primary" : "secondary"} onPress={() => preferences.setPreferences({ palette: theme.id, customColor: "" })}><span className="size-4 rounded-full border border-white/40" style={{ backgroundColor: theme.color }} />{theme.label}</Button>)}</div>
      </SettingsRow>
      <SettingsRow label="Color personalizado">
        <div className="flex items-center gap-3"><input type="color" aria-label="Color personalizado" value={preferences.customColor || visualThemes.find(theme => theme.id === preferences.palette)!.color} onChange={event => preferences.setPreferences({ customColor: event.target.value })} /><Button variant="secondary" onPress={() => preferences.setPreferences({ customColor: "" })}>Restablecer</Button></div>
      </SettingsRow>
      <SettingsRow label="Densidad">
        <div className="flex gap-2">{(["comfortable", "compact"] as const).map((density, index) => <Button key={density} variant={preferences.density === density ? "primary" : "secondary"} onPress={() => preferences.setPreferences({ density })}>{["Cómoda", "Compacta"][index]}</Button>)}</div>
      </SettingsRow>
      <SettingsRow label="Límite de almacenamiento (GiB)" description="Referencia personal para mostrar espacio disponible. Telegram no ofrece una cuota total; deja el campo vacío si no quieres fijar un límite.">
        <input className="w-full rounded-lg border border-border bg-field-background p-3" type="number" min="0" step="1" aria-label="Límite de almacenamiento" value={preferences.storageLimitGiB ?? ""} onChange={event => { const value = Number(event.target.value); preferences.setPreferences({ storageLimitGiB: Number.isFinite(value) && value > 0 ? value : null }); }} />
      </SettingsRow>
    </SettingsSection>
  </div>;
}
