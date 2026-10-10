import { $api } from "@/utils/api"
import { filesize } from "@/utils/common"
import { storageUsage } from "@/utils/storage"
import { useSettingsStore } from "@/utils/stores/settings"
import { Link } from "@tanstack/react-router"
import IconCloud from "~icons/ic/outline-cloud"

export function StorageSummary({ compact = false }: { compact?: boolean }) {
  const { data, isError, isPending } = $api.useQuery("get", "/files/categories", undefined, { refetchInterval: 30_000, refetchOnWindowFocus: true })
  const limit = useSettingsStore((state) => Number(state.settings.storageLimitGB))
  const { used, total, available, percent, exceeded } = storageUsage(data || [], limit)
  return <section aria-label="Uso de almacenamiento" className={compact ? "space-y-3 px-3 text-xs" : "space-y-3 rounded-3xl bg-surface-container-low border border-outline-variant/50 p-6"}>
    {!compact && <h2 className="flex items-center gap-2 font-semibold text-xl"><IconCloud className="size-5" aria-hidden="true" />Almacenamiento</h2>}
    {isPending ? <p className="text-on-surface-variant">Calculando uso…</p> : isError ? <p className="text-on-surface-variant">No se pudo consultar el almacenamiento.</p> : <>
      <div className="h-2 rounded-full overflow-hidden bg-surface-container-highest" aria-hidden="true"><div className={`h-full rounded-full ${exceeded ? "bg-error" : "bg-primary"}`} style={{ width: total ? `${percent}%` : "100%", opacity: total ? 1 : 0.35 }} /></div>
      {total > 0 && <progress className="sr-only" aria-label="Almacenamiento utilizado" value={Math.min(used, total)} max={total} />}
      <p className="text-on-surface-variant">{filesize(used)} utilizados{total > 0 ? ` de ${filesize(total)}` : ""}</p>
      <p className={exceeded ? "text-error" : "text-on-surface-variant"}>{total ? exceeded ? "Límite de referencia superado" : `${filesize(available)} disponibles` : "Sin límite configurado"}</p>
    </>}
    <Link className="inline-block text-primary rounded-full border border-outline-variant px-4 py-2 hover:bg-primary/5" to="/settings/$tabId" params={{ tabId: "general" }}>Configurar límite</Link>
  </section>
}
