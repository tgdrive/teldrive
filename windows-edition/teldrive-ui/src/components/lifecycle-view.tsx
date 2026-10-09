import { filesize } from "@/utils/common"
import { type LifecycleItem, lifecycleChange, lifecycleList } from "@/utils/lifecycle"
import { categoryLabels } from "@/utils/locale"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Button, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Spinner } from "@tw-material/react"
import { useState } from "react"
import toast from "react-hot-toast"
import IconFolder from "~icons/ic/outline-folder"
import IconGrid from "~icons/ic/outline-grid-view"
import IconFile from "~icons/ic/outline-insert-drive-file"
import IconList from "~icons/ic/round-view-list"

function itemCategory(item: LifecycleItem) {
  if (item.type === "folder") return "folder"
  for (const category of ["image", "video", "audio"]) if (item.mimeType?.startsWith(`${category}/`)) return category
  if (/\.(pdf|txt|docx?|xlsx?|pptx?|odt|ods|csv)$/i.test(item.name)) return "document"
  if (/\.(zip|rar|7z|tar|gz)$/i.test(item.name)) return "archive"
  return "other"
}

export function LifecycleView({ state }: { state: "trash" | "spam" }) {
  const client = useQueryClient()
  const { data: items = [], isPending, isError, refetch } = useQuery({ queryKey: ["Lifecycle", state], queryFn: ({ signal }) => lifecycleList(state, signal) })
  const [selection, setSelection] = useState<string[]>([])
  const [confirm, setConfirm] = useState<string[] | null>(null)
  const [type, setType] = useState("")
  const [modified, setModified] = useState("")
  const [from, setFrom] = useState("")
  const [until, setUntil] = useState("")
  const [view, setView] = useState(localStorage.getItem("viewId") === "enable_grid_view" ? "grid" : "list")
  const visible = items.filter((item) => {
    if (type && itemCategory(item) !== type) return false
    const date = new Date(item.updatedAt || item.movedAt).getTime()
    if (modified === "custom") return (!from || date >= new Date(`${from}T00:00:00`).getTime()) && (!until || date <= new Date(`${until}T23:59:59.999`).getTime())
    return !modified || date >= Date.now() - Number(modified) * 86400000
  })
  const selected = selection.filter((id) => visible.some((item) => item.id === id))
  function select(id: string, checked: boolean) { setSelection((previous) => checked ? [...new Set([...previous, id])] : previous.filter((value) => value !== id)) }
  function setLayout(mode: string) { setView(mode); localStorage.setItem("viewId", mode === "grid" ? "enable_grid_view" : "enable_list_view") }
  const change = useMutation({ mutationFn: async ({ ids, action }: { ids: string[]; action: "restore" | "delete" }) => {
    for (let i = 0; i < ids.length; i += 500) await lifecycleChange(ids.slice(i, i + 500), action, state)
  }, onSuccess: (_, variables) => {
    setSelection([]); setConfirm(null)
    void client.invalidateQueries({ queryKey: ["Lifecycle"] }); void client.invalidateQueries({ queryKey: ["Files_list"] }); void client.invalidateQueries({ queryKey: ["get", "/files/categories"] })
    toast.success(variables.action === "restore" ? "Archivos restaurados" : "Archivos eliminados definitivamente")
  }, onError: (error: Error) => { void refetch(); toast.error(error.message) } })
  const control = "rounded-xl bg-surface-container-low border border-outline-variant px-3 py-2 text-sm"
  return <div className="h-full flex flex-col gap-4">
    <h1 className="text-2xl">{state === "trash" ? "Papelera" : "Spam"}</h1>
    <p className="text-sm text-on-surface-variant">{state === "trash" ? "Puedes restaurar estos elementos durante 30 días. Después se eliminan automáticamente cuando el servidor ejecuta la limpieza programada." : "Elementos marcados manualmente como no deseados. No aparecen en búsquedas. Puedes restaurarlos durante 30 días; después se eliminan con la limpieza programada."}</p>
    <div className="flex flex-wrap gap-3 items-center">
      <label className="flex items-center gap-2 text-sm">Tipo<select className={control} value={type} onChange={(e) => { setType(e.target.value); setSelection([]) }}><option value="">Todos los tipos</option>{Object.entries(categoryLabels).map(([id, label]) => <option key={id} value={id}>{label}</option>)}</select></label>
      <label className="flex items-center gap-2 text-sm">Modificado<select className={control} value={modified} onChange={(e) => { setModified(e.target.value); setSelection([]) }}><option value="">Cualquier fecha</option><option value="1">Últimas 24 horas</option><option value="7">Últimos 7 días</option><option value="30">Últimos 30 días</option><option value="90">Últimos 90 días</option><option value="custom">Personalizado</option></select></label>
      {modified === "custom" && <><label className="text-sm">Desde <input className={control} type="date" value={from} max={until || undefined} onChange={(e) => { setFrom(e.target.value); setSelection([]) }} /></label><label className="text-sm">Hasta <input className={control} type="date" value={until} min={from || undefined} onChange={(e) => { setUntil(e.target.value); setSelection([]) }} /></label></>}
      {(type || modified) && <Button size="sm" variant="text" onPress={() => { setType(""); setModified(""); setFrom(""); setUntil(""); setSelection([]) }}>Limpiar filtros</Button>}
      <fieldset aria-label="Vista de archivos" className="flex ml-auto border border-outline-variant/50 rounded-full p-0.5"><Button isIconOnly size="sm" variant={view === "list" ? "filledTonal" : "text"} aria-label="Vista de lista" aria-pressed={view === "list"} onPress={() => setLayout("list")}><IconList /></Button><Button isIconOnly size="sm" variant={view === "grid" ? "filledTonal" : "text"} aria-label="Vista de cuadrícula" aria-pressed={view === "grid"} onPress={() => setLayout("grid")}><IconGrid /></Button></fieldset>
    </div>
    <div className="flex flex-wrap items-center gap-2 border-b border-outline-variant/40 pb-3">
      <span className="text-sm mr-auto">{selected.length ? `${selected.length} ${selected.length === 1 ? "seleccionado" : "seleccionados"}` : `${visible.length} ${visible.length === 1 ? "elemento" : "elementos"}`}</span>
      <Button size="sm" variant="filledTonal" isDisabled={!selected.length || change.isPending} onPress={() => change.mutate({ ids: selected, action: "restore" })}>{state === "trash" ? "Restaurar" : "No es spam"}</Button>
      <Button size="sm" variant="text" isDisabled={!selected.length || change.isPending} onPress={() => setConfirm(selected)}>Eliminar definitivamente</Button>
      <Button size="sm" variant="outlined" isDisabled={!items.length || change.isPending} onPress={() => setConfirm(items.map((item) => item.id))}>{state === "trash" ? "Vaciar papelera" : "Vaciar spam"}</Button>
    </div>
    {isPending ? <Spinner aria-label="Cargando archivos" /> : isError ? <div><p>No se pudo consultar esta carpeta.</p><Button variant="text" onPress={() => void refetch()}>Reintentar</Button></div> : visible.length === 0 ? <p className="m-auto text-on-surface-variant">{items.length ? "No hay archivos con estos filtros" : state === "trash" ? "La papelera está vacía" : "No hay archivos en Spam"}</p> : <div className="flex-1 overflow-auto">
      {view === "grid" ? <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4 p-1">{visible.map((item) => <label key={item.id} className={`border rounded-2xl p-4 space-y-3 cursor-pointer ${selected.includes(item.id) ? "border-primary bg-secondary-container/40" : "border-outline-variant/40 bg-surface-container-low"}`}><div className="flex justify-between"><span className="size-12 flex items-center justify-center rounded-xl bg-secondary-container text-on-secondary-container">{item.type === "folder" ? <IconFolder className="size-7" /> : <IconFile className="size-7" />}</span><input type="checkbox" aria-label={`Seleccionar ${item.name}`} checked={selected.includes(item.id)} onChange={(e) => select(item.id, e.target.checked)} /></div><span className="block truncate font-medium" title={item.name}>{item.name}</span><span className="block text-xs text-on-surface-variant">{categoryLabels[itemCategory(item)]} · {item.type === "folder" ? "Carpeta" : filesize(item.size || 0)}</span><span className="block text-xs text-on-surface-variant">Modificado: {new Date(item.updatedAt || item.movedAt).toLocaleDateString("es-CL")}</span></label>)}</div> : <table className="w-full text-sm text-left"><thead><tr className="text-on-surface-variant border-b border-outline-variant/30"><th className="p-3"><input type="checkbox" aria-label="Seleccionar todos los archivos visibles" checked={selected.length === visible.length} onChange={(e) => setSelection(e.target.checked ? visible.map((item) => item.id) : [])} /></th><th className="p-3">Nombre</th><th className="p-3">Tamaño</th><th className="p-3 whitespace-nowrap">Modificado</th><th className="p-3 whitespace-nowrap">Fecha de ingreso</th></tr></thead><tbody>{visible.map((item) => <tr key={item.id} className={`border-b border-outline-variant/20 ${selected.includes(item.id) ? "bg-secondary-container/40" : "hover:bg-surface-container"}`}><td className="p-3"><input type="checkbox" aria-label={`Seleccionar ${item.name}`} checked={selected.includes(item.id)} onChange={(e) => select(item.id, e.target.checked)} /></td><td className="p-3"><span className="flex gap-3 items-center">{item.type === "folder" ? <IconFolder className="size-5 shrink-0 text-primary" /> : <IconFile className="size-5 shrink-0" />}{item.name}</span></td><td className="p-3 whitespace-nowrap">{item.type === "folder" ? "—" : filesize(item.size || 0)}</td><td className="p-3 whitespace-nowrap">{new Date(item.updatedAt || item.movedAt).toLocaleDateString("es-CL")}</td><td className="p-3 whitespace-nowrap">{new Date(item.movedAt).toLocaleDateString("es-CL")}</td></tr>)}</tbody></table>}
    </div>}
    <p className="text-xs text-on-surface-variant">Restaurar una carpeta recupera su contenido. Si la ubicación original no existe, vuelve a Mi unidad. La limpieza automática requiere tareas programadas activas.</p>
    <Modal isOpen={confirm !== null} onClose={() => { if (!change.isPending) setConfirm(null) }}><ModalContent><ModalHeader>Eliminar definitivamente</ModalHeader><ModalBody><p>¿Eliminar {confirm?.length} elementos y el contenido de las carpetas seleccionadas?</p><p className="text-sm text-on-surface-variant">No podrás restaurarlos. La eliminación de los datos de Telegram termina durante la limpieza programada del servidor.</p></ModalBody><ModalFooter><Button variant="text" isDisabled={change.isPending} onPress={() => setConfirm(null)}>Cancelar</Button><Button variant="filled" isLoading={change.isPending} onPress={() => { if (confirm) change.mutate({ ids: confirm, action: "delete" }) }}>Eliminar definitivamente</Button></ModalFooter></ModalContent></Modal>
  </div>
}
