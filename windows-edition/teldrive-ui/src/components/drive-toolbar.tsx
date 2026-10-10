import { CustomActions } from "@/hooks/use-file-action"
import { FbActions, type FileBrowserHandle, selectFileViewConfig, selectSelectedFiles, selectSelectionSize, useParamSelector } from "@tw-material/file-browser"
import { Button, Dropdown, DropdownItem, DropdownMenu, DropdownTrigger } from "@tw-material/react"
import type { RefObject } from "react"
import IconShare from "~icons/fluent/share-24-regular"
import IconDownload from "~icons/ic/outline-file-download"
import IconGrid from "~icons/ic/outline-grid-view"
import IconMore from "~icons/ic/round-more-vert"
import IconSort from "~icons/ic/round-sort"
import IconList from "~icons/ic/round-view-list"

export function DriveToolbar({ browserRef, readOnly = false }: { browserRef: RefObject<FileBrowserHandle>; readOnly?: boolean }) {
  const count = useParamSelector(() => selectSelectionSize)
  const selectedFiles = useParamSelector(() => selectSelectedFiles)
  const layout = useParamSelector(() => selectFileViewConfig)
  function request(action: typeof FbActions.DownloadFiles | typeof FbActions.RenameFile | typeof FbActions.DeleteFiles | typeof FbActions.EnableGridView | typeof FbActions.EnableListView | typeof FbActions.SortFilesByName | typeof FbActions.SortFilesBySize | typeof FbActions.SortFilesByDate) {
    void browserRef.current?.requestFileAction(action, undefined)
  }
  return (
    <div className="flex flex-wrap items-center gap-2 py-3 mb-1 border-b border-outline-variant/40" aria-label="Acciones de archivos">
      <span className="text-sm text-on-surface-variant mr-auto" aria-live="polite">{count ? `${count} seleccionado${count === 1 ? "" : "s"}` : "Todos los archivos"}</span>
      {count > 0 && <>
        {!readOnly && <Button size="sm" variant="filledTonal" isDisabled={count !== 1} startContent={<IconShare className="size-4" />} onPress={() => void browserRef.current?.requestFileAction(CustomActions.ShareFiles, undefined)}>Compartir</Button>}
        <Button size="sm" variant="text" isDisabled={selectedFiles.every((file) => file.isDir)} title="Descargar archivos individuales" startContent={<IconDownload className="size-4" />} onPress={() => request(FbActions.DownloadFiles)}>Descargar</Button>
        {!readOnly && <Dropdown><DropdownTrigger><Button isIconOnly size="sm" variant="text" aria-label="Más acciones"><IconMore /></Button></DropdownTrigger><DropdownMenu aria-label="Más acciones" onAction={(key) => key === "spam" ? void browserRef.current?.requestFileAction(CustomActions.MarkSpam, undefined) : request(key === "rename" ? FbActions.RenameFile : FbActions.DeleteFiles)} disabledKeys={count === 1 ? [] : ["rename"]}><DropdownItem key="rename">Cambiar nombre</DropdownItem><DropdownItem key="spam">Mover a Spam</DropdownItem><DropdownItem key="delete" className="text-error">Mover a la papelera</DropdownItem></DropdownMenu></Dropdown>}
      </>}
      <Dropdown><DropdownTrigger><Button size="sm" variant="text" aria-label="Ordenar archivos" startContent={<IconSort className="size-4" />}>Ordenar</Button></DropdownTrigger><DropdownMenu aria-label="Ordenar archivos" onAction={(key) => request(key === "name" ? FbActions.SortFilesByName : key === "size" ? FbActions.SortFilesBySize : FbActions.SortFilesByDate)}><DropdownItem key="name">Nombre</DropdownItem><DropdownItem key="date">Última modificación</DropdownItem><DropdownItem key="size">Tamaño</DropdownItem></DropdownMenu></Dropdown>
      <fieldset className="flex border border-outline-variant/50 rounded-full p-0.5" aria-label="Vista de archivos">
        <Button isIconOnly size="sm" variant={layout.mode === "list" ? "filledTonal" : "text"} aria-label="Vista de lista" aria-pressed={layout.mode === "list"} onPress={() => request(FbActions.EnableListView)}><IconList className="size-5" /></Button>
        <Button isIconOnly size="sm" variant={layout.mode === "grid" ? "filledTonal" : "text"} aria-label="Vista de cuadrícula" aria-pressed={layout.mode === "grid"} onPress={() => request(FbActions.EnableGridView)}><IconGrid className="size-5" /></Button>
      </fieldset>
    </div>
  )
}
