import Loader from "@/components/loader"
import AudioPreview from "@/components/previews/audio/audio-preview"
import CodePreview from "@/components/previews/code-preview"
import DocPreview from "@/components/previews/doc-preview"
import ImagePreview from "@/components/previews/image-preview"
import { MediaCompatibility } from "@/components/previews/media-compatibility"
import PDFPreview from "@/components/previews/pdf-preview"
import { CustomActions } from "@/hooks/use-file-action"
import type { BrowseView, Session } from "@/types"
import { filesize, mediaUrl, sharedMediaUrl } from "@/utils/common"
import { useModalStore } from "@/utils/stores"
import type { FileData } from "@tw-material/file-browser"
import { Button, Modal, ModalContent } from "@tw-material/react"
import { Suspense, lazy, memo, useCallback, useEffect, useMemo, useState } from "react"
import { ErrorBoundary } from "react-error-boundary"
import { useShallow } from "zustand/react/shallow"
import IconShare from "~icons/fluent/share-24-regular"
import IconDownload from "~icons/ic/outline-file-download"
import IconClose from "~icons/ic/round-close"
import IconPrev from "~icons/ic/round-navigate-before"
import IconNext from "~icons/ic/round-navigate-next"
import IconInfo from "~icons/mdi/information-outline"

const VideoPreview = lazy(() => import("@/components/previews/video/video-preview"))
const EpubPreview = lazy(() => import("@/components/previews/epub-preview"))

export default memo(function PreviewModal({ files: fileProp, session, shareId, path, view }: {
  files: FileData[]; path: string; session?: Session; shareId?: string; view: BrowseView
}) {
  const files = useMemo(() => fileProp.filter((file) => !file.isDir), [fileProp])
  const { actions, open, currentFile } = useModalStore(useShallow((state) => ({ actions: state.actions, open: state.open, currentFile: state.currentFile })))
  const [showDetails, setShowDetails] = useState(false)
  const { id, name, previewType } = currentFile
  const index = files.findIndex((file) => file.id === id)
  const assetUrl = shareId ? sharedMediaUrl(shareId, id, name) : mediaUrl(id, name, view === "my-drive" ? path || "/" : "", session?.hash || "")
  const downloadUrl = shareId ? sharedMediaUrl(shareId, id, name, true) : mediaUrl(id, name, view === "my-drive" ? path || "/" : "", session?.hash || "", true)
  const navigateFile = useCallback((direction: number, type = "all") => {
    const candidates = type === "all" ? files : files.filter((file) => file.previewType === type)
    if (candidates.length < 2) return
    const position = candidates.findIndex((file) => file.id === id)
    actions.setCurrentFile(candidates[(position + direction + candidates.length) % candidates.length])
  }, [files, id, actions])
  const nextItem = useCallback((type = "all") => navigateFile(1, type), [navigateFile])
  const prevItem = useCallback((type = "all") => navigateFile(-1, type), [navigateFile])

  useEffect(() => {
    function handleKey(event: KeyboardEvent) {
      const target = event.target as HTMLElement
      if (target.closest("input, textarea, select, video, audio, [contenteditable=true], [role=slider]")) return
      if (previewType === "video" || previewType === "audio" || previewType === "code") return
      if (event.key === "ArrowRight") { event.preventDefault(); nextItem() }
      if (event.key === "ArrowLeft") { event.preventDefault(); prevItem() }
    }
    window.addEventListener("keydown", handleKey)
    return () => window.removeEventListener("keydown", handleKey)
  }, [nextItem, prevItem, previewType])

  const unavailable = <div className="flex flex-col items-center justify-center gap-4 p-6 h-full text-center"><IconInfo className="size-12 opacity-70" /><h2 className="text-xl">No hay vista previa disponible</h2><p className="text-sm text-white/70 max-w-md">Descarga el archivo para abrirlo con una aplicación compatible.</p><Button as="a" href={downloadUrl} variant="filledTonal" startContent={<IconDownload />}>Descargar archivo</Button></div>
  function renderPreview() {
    switch (previewType) {
      case "video": return <div className="w-full max-w-6xl m-auto"><MediaCompatibility key={id} id={id} mode="video" shareId={shareId}>{(url) => <VideoPreview url={url || assetUrl} />}</MediaCompatibility></div>
      case "image": return <ImagePreview name={name} assetUrl={assetUrl} />
      case "pdf": return <PDFPreview assetUrl={assetUrl} />
      case "office": return <DocPreview assetUrl={assetUrl} />
      case "code": case "text": case "markdown": return <CodePreview name={name} assetUrl={assetUrl} />
      case "epub": return <div className="h-full bg-white text-black"><EpubPreview assetUrl={assetUrl} /></div>
      case "audio": return <div className="max-w-4xl w-full m-auto"><MediaCompatibility key={id} id={id} mode="audio" shareId={shareId}>{(url) => <div className="rounded-2xl bg-surface text-on-surface p-4 sm:p-8"><AudioPreview nextItem={nextItem} prevItem={prevItem} name={name} assetUrl={url || assetUrl} /></div>}</MediaCompatibility></div>
      default: return unavailable
    }
  }

  return (
    <Modal aria-labelledby="preview-file-name" isOpen={open} size="full" placement="center" hideCloseButton onClose={() => actions.setOpen(false)} classNames={{ wrapper: "overflow-hidden", base: "!m-0 rounded-none bg-[#16181c] text-white size-full max-w-none shadow-none" }}>
      <ModalContent>
        <div className="grid grid-rows-[auto_1fr_auto] size-full min-h-0">
          <header className="flex items-center gap-2 border-b border-white/10 p-3">
            <Button isIconOnly variant="text" className="text-white shrink-0" aria-label="Cerrar vista previa" title="Cerrar (Esc)" onPress={() => actions.setOpen(false)}><IconClose /></Button>
            <div className="min-w-0 flex-1"><h2 id="preview-file-name" className="text-sm sm:text-base truncate" title={name}>{name}</h2><p className="text-xs text-white/60">{files.length > 0 ? `${index + 1} de ${files.length}` : "Vista previa"}</p></div>
            {session && !shareId && <Button isIconOnly variant="text" className="text-white" aria-label="Compartir archivo" title="Compartir" onPress={() => actions.set({ operation: CustomActions.ShareFiles.id, open: true })}><IconShare /></Button>}
            <Button as="a" href={downloadUrl} isIconOnly variant="text" className="text-white" aria-label="Descargar archivo" title="Descargar"><IconDownload /></Button>
            <Button isIconOnly variant="text" className="text-white" aria-label="Detalles del archivo" aria-pressed={showDetails} title="Detalles" onPress={() => setShowDetails((value) => !value)}><IconInfo /></Button>
          </header>
          <div className="flex min-h-0 overflow-hidden">
            <div className="flex-1 min-w-0 min-h-0 p-2 sm:p-5 flex flex-col overflow-auto">
              <ErrorBoundary key={id} fallback={unavailable}><Suspense fallback={<Loader />}><div key={id} className="h-full min-h-0 flex flex-col justify-center">{renderPreview()}</div></Suspense></ErrorBoundary>
            </div>
            {showDetails && <aside className="w-44 sm:w-64 shrink-0 border-l border-white/10 p-4 overflow-y-auto text-sm space-y-5"><h3 className="font-semibold">Detalles</h3><dl className="space-y-4"><div><dt className="text-white/60">Nombre</dt><dd className="break-all mt-1">{name}</dd></div><div><dt className="text-white/60">Tipo</dt><dd className="break-all mt-1">{currentFile.mimeType || "Archivo"}</dd></div><div><dt className="text-white/60">Tamaño</dt><dd className="mt-1">{filesize(currentFile.size || 0)}</dd></div>{currentFile.modDate && <div><dt className="text-white/60">Modificado</dt><dd className="mt-1">{new Date(currentFile.modDate).toLocaleString()}</dd></div>}</dl></aside>}
          </div>
          <footer className="flex justify-center items-center gap-5 p-2 border-t border-white/10">
            <Button isIconOnly variant="text" className="text-white" isDisabled={files.length < 2} onPress={() => prevItem()} aria-label="Archivo anterior"><IconPrev /></Button>
            <span className="text-xs text-white/60">{index + 1} / {files.length}</span>
            <Button isIconOnly variant="text" className="text-white" isDisabled={files.length < 2} onPress={() => nextItem()} aria-label="Archivo siguiente"><IconNext /></Button>
          </footer>
        </div>
      </ModalContent>
    </Modal>
  )
})
