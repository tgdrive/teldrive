import { useQuery } from "@tanstack/react-query"
import { Button } from "@tw-material/react"
import { useState } from "react"

export function MediaCompatibility({ id, mode, shareId, children }: { id: string; mode: "audio" | "video"; shareId?: string; children: (url?: string) => React.ReactNode }) {
  const [compatible, setCompatible] = useState(false)
  const [start, setStart] = useState("0")
  const [offset, setOffset] = useState(0)
  const { data } = useQuery({ queryKey: ["media-capabilities"], queryFn: async () => {
    const response = await fetch("/api/media/capabilities")
    if (!response.ok) return { compatiblePlayback: false }
    return response.json() as Promise<{ compatiblePlayback: boolean }>
  }, staleTime: 60_000, retry: false })
  const url = compatible ? `${shareId ? `/api/shares/${encodeURIComponent(shareId)}/media/${encodeURIComponent(id)}` : `/api/media/${encodeURIComponent(id)}/compatible`}?mode=${mode}&start=${offset}` : undefined
  return <div className="space-y-4 w-full m-auto">
    <div className="flex flex-wrap items-center justify-center gap-2 text-sm">
      <Button size="sm" variant={!compatible ? "filledTonal" : "text"} onPress={() => setCompatible(false)}>Original</Button>
      <Button size="sm" variant={compatible ? "filledTonal" : "text"} isDisabled={!data?.compatiblePlayback} onPress={() => setCompatible(true)}>Modo compatible (FFmpeg)</Button>
      {compatible && <><label className="text-white/80">Iniciar desde (segundos)<input className="ml-2 w-24 rounded-lg bg-white/10 border border-white/20 p-2" type="number" min="0" max="86400" value={start} onChange={(event) => setStart(event.target.value)} /></label><Button size="sm" variant="text" onPress={() => setOffset(Math.max(0, Math.min(86400, Number(start) || 0)))}>Ir</Button></>}
    </div>
    {compatible && <p className="text-xs text-center text-white/70 px-4">Conversión en el servidor a {mode === "audio" ? "MP3" : "MP4 con H.264 y AAC"}. Puede tardar unos segundos. El avance directo funciona sobre lo ya cargado; usa «Iniciar desde» para saltar a otro momento.</p>}
    {data && !data.compatiblePlayback && <p className="text-xs text-center text-white/70">Para convertir formatos no admitidos, instala FFmpeg en el servidor y reinicia Teldrive. Configura su ruta en Servidor y CLI.</p>}
    <div key={`${compatible}-${offset}`}>{children(url)}</div>
  </div>
}
