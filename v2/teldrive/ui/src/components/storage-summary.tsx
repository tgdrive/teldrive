import { $api } from "@/api/client";
import { useTheme } from "@/lib/theme";
function bytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const unit = Math.min(4, Math.floor(Math.log(value) / Math.log(1024)));
  return `${(value / 1024 ** unit).toLocaleString("es", { maximumFractionDigits: 1 })} ${["B", "KiB", "MiB", "GiB", "TiB"][unit]}`;
}
export function StorageSummary() {
  const { data, isError } = $api.useQuery("get", "/v1/storage/stats", {}, { staleTime: 30_000 });
  const { storageLimitGiB } = useTheme();
  const used = data ? data.summary.logicalBytes + data.summary.trashBytes : 0;
  const total = storageLimitGiB ? storageLimitGiB * 1024 ** 3 : null;
  const percent = total ? Math.min(100, Math.round(used / total * 100)) : 0;
  return <div className="mb-3 space-y-2 px-3 text-xs text-muted" aria-label="Uso de almacenamiento">
    <div role="progressbar" aria-label="Almacenamiento utilizado" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent} aria-valuetext={data ? `${bytes(used)} utilizados${total ? ` de ${bytes(total)}` : "; sin límite configurado"}` : "Cargando almacenamiento"} className="h-1.5 overflow-hidden rounded-full bg-default"><div className="h-full rounded-full bg-accent transition-all" style={{ width: total ? `${percent}%` : data && used > 0 ? "100%" : "0%" }} /></div>
    <p>{isError ? "No se pudo consultar el almacenamiento" : !data ? "Calculando…" : `${bytes(used)} utilizados${total ? ` de ${bytes(total)}` : " · sin límite"}`}</p>
    {data && total ? <p>{bytes(Math.max(0, total - used))} disponibles</p> : null}
  </div>;
}
