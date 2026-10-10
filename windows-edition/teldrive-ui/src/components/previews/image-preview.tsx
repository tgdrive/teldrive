import { Button } from "@tw-material/react"
import { memo, useState } from "react"
import IconSpinner from "~icons/svg-spinners/tadpole"

export default memo(function ImagePreview({ name, assetUrl }: { name: string; assetUrl: string }) {
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)
  const [zoom, setZoom] = useState(1)
  const [rotation, setRotation] = useState(0)
  return (
    <div className="flex flex-col h-full min-h-0 gap-3">
      <div className="relative flex-1 min-h-0 overflow-auto flex items-center justify-center">
        {!loaded && !failed && <IconSpinner className="size-8 absolute" />}
        {failed ? <p role="alert" className="text-sm text-white/80 text-center p-6">No se pudo mostrar la imagen. Descárgala para abrirla con otra aplicación.</p> : <img src={assetUrl} alt={name} onLoad={() => setLoaded(true)} onError={() => setFailed(true)} className="max-w-full max-h-full object-contain transition-opacity" style={{ opacity: loaded ? 1 : 0, transform: `scale(${zoom}) rotate(${rotation}deg)`, transformOrigin: "center" }} />}
      </div>
      <div className="flex justify-center items-center gap-2">
        <Button variant="text" size="sm" className="text-white min-w-9" aria-label="Alejar imagen" isDisabled={zoom <= 0.5 || failed} onPress={() => setZoom((value) => Math.max(0.5, value - 0.25))}>−</Button>
        <Button variant="text" size="sm" className="text-white" aria-label="Restablecer imagen" onPress={() => { setZoom(1); setRotation(0) }}>{Math.round(zoom * 100)}%</Button>
        <Button variant="text" size="sm" className="text-white min-w-9" aria-label="Acercar imagen" isDisabled={zoom >= 3 || failed} onPress={() => setZoom((value) => Math.min(3, value + 0.25))}>+</Button>
        <Button variant="text" size="sm" className="text-white" aria-label="Girar imagen" isDisabled={failed} onPress={() => setRotation((value) => value + 90)}>Girar</Button>
      </div>
    </div>
  )
})
