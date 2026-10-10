import { useFileUploadStore, useModalStore } from "@/utils/stores"
import { useNavigate, useParams } from "@tanstack/react-router"
import { FbActions, type FileData } from "@tw-material/file-browser"
import { Button, Dropdown, DropdownItem, DropdownMenu, DropdownTrigger } from "@tw-material/react"
import toast from "react-hot-toast"
import IconUpload from "~icons/ic/outline-file-upload"
import IconFolder from "~icons/ic/outline-folder-open"
import IconPlus from "~icons/ic/round-add"

export function NewMenu() {
  const navigate = useNavigate()
  const { view } = useParams({ strict: false }) as { view?: string }
  async function handleAction(key: React.Key) {
    try {
      if (view !== "my-drive") await navigate({ to: "/$view", params: { view: "my-drive" }, search: { path: "/" } })
      if (key === "folder") {
        useModalStore.getState().actions.set({ open: true, operation: FbActions.CreateFolder.id, currentFile: { name: "" } as FileData })
        return
      }
      const actions = useFileUploadStore.getState().actions
      actions.setFolderDialogOpen(key === "upload-folder")
      actions.setFileDialogOpen(key === "upload-file")
      actions.setUploadOpen(true)
    } catch {
      toast.error("No se pudo abrir Mi unidad. Inténtalo de nuevo.")
    }
  }
  return (
    <Dropdown>
      <DropdownTrigger>
        <Button variant="filledTonal" className="rounded-2xl h-12 md:h-14 px-5 md:px-6 shadow-sm bg-surface-container-low text-on-surface gap-3" startContent={<IconPlus className="size-7" />}>Nuevo</Button>
      </DropdownTrigger>
      <DropdownMenu aria-label="Crear o subir" onAction={handleAction}>
        <DropdownItem key="folder" startContent={<IconFolder />}>Nueva carpeta</DropdownItem>
        <DropdownItem key="upload-file" startContent={<IconUpload />}>Subir archivos</DropdownItem>
        <DropdownItem key="upload-folder" startContent={<IconFolder />}>Subir carpeta</DropdownItem>
      </DropdownMenu>
    </Dropdown>
  )
}
