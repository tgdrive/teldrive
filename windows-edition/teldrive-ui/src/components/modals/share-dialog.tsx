import { CopyButton } from "@/components/copy-button"
import { $api } from "@/utils/api"
import { NetworkError } from "@/utils/fetch-throw"
import { useModalStore } from "@/utils/stores"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Button, Input, ModalBody, ModalFooter, ModalHeader, Switch } from "@tw-material/react"
import { useEffect, useState } from "react"
import IconLink from "~icons/ic/round-link"
import IconLock from "~icons/mdi/lock-outline"

function localDate(value: Date) {
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}-${String(value.getDate()).padStart(2, "0")}`
}

export function ShareFileDialog({ handleClose }: { handleClose: () => void }) {
  const file = useModalStore((state) => state.currentFile)
  const queryClient = useQueryClient()
  const options = $api.queryOptions("get", "/files/{id}/share", { params: { path: { id: file.id } } })
  const { data, isLoading, error, refetch } = useQuery({ ...options, retry: false })
  const create = $api.useMutation("post", "/files/{id}/share")
  const update = $api.useMutation("patch", "/files/{id}/share")
  const remove = $api.useMutation("delete", "/files/{id}/share")
  const [password, setPassword] = useState("")
  const [protectedLink, setProtectedLink] = useState(false)
  const [expiresAt, setExpiresAt] = useState("")
  const [message, setMessage] = useState("")
  const [saving, setSaving] = useState(false)
  const busy = saving || create.isPending || update.isPending || remove.isPending
  const loadFailed = !!error && !(error instanceof NetworkError && error.status === 404)
  const link = data ? `${window.location.origin}/share/${data.id}` : ""
  const expired = !!data?.expiresAt && new Date(data.expiresAt) < new Date()

  useEffect(() => {
    setProtectedLink(!!data?.protected)
    setExpiresAt(data?.expiresAt ? localDate(new Date(data.expiresAt)) : "")
    setPassword("")
  }, [data])

  async function save(revoke = false) {
    setMessage("")
    if (!revoke && protectedLink && !password && !data?.protected) {
      setMessage("Introduce una contraseña para proteger el enlace.")
      return
    }
    if (!revoke && expiresAt && new Date(`${expiresAt}T23:59:59`) <= new Date()) {
      setMessage("Selecciona una fecha de vencimiento futura.")
      return
    }
    setSaving(true)
    try {
      const params = { path: { id: file.id } }
      if (revoke) await remove.mutateAsync({ params })
      else {
        const body = {
          ...(protectedLink ? (password ? { password } : {}) : { password: "" }),
          ...(expiresAt ? { expiresAt: new Date(`${expiresAt}T23:59:59`).toISOString() } : data ? { expiresAt: "0001-01-01T00:00:00Z" } : {}),
        }
        if (data) await update.mutateAsync({ params, body })
        else await create.mutateAsync({ params, body })
      }
      queryClient.removeQueries({ queryKey: ["Shares_listFiles"] })
      await queryClient.invalidateQueries({ queryKey: ["Files_list"] })
      if (revoke) queryClient.setQueryData(options.queryKey, null)
      else {
        await queryClient.invalidateQueries({ queryKey: options.queryKey })
      }
      setMessage(revoke ? "Enlace eliminado. El acceso queda restringido." : "Enlace guardado.")
    } catch {
      setMessage("No se pudo guardar el cambio. Revisa la conexión e inténtalo de nuevo.")
    } finally { setSaving(false) }
  }

  return (
    <>
      <ModalHeader className="flex-col items-start gap-1">
        <h2 className="text-xl">Compartir</h2><p className="text-sm font-normal text-on-surface-variant truncate max-w-full" title={file.name}>{file.name}</p>
      </ModalHeader>
      <ModalBody className="gap-5">
        <section className="rounded-2xl border border-outline-variant/50 p-4 space-y-3">
          <h3 className="font-medium">Acceso al enlace</h3>
          <div className="flex items-center gap-3">
            {data ? <IconLink className="size-6 text-primary" /> : <IconLock className="size-6 text-on-surface-variant" />}
            <div><p className="text-sm font-medium">{isLoading ? "Comprobando acceso…" : data ? (expired ? "Enlace vencido" : "Cualquier persona con el enlace") : "Restringido"}</p>
              <p className="text-xs text-on-surface-variant">{data ? "Permite ver y descargar. No permite editar." : "Crea un enlace para compartir este archivo o carpeta."}</p></div>
          </div>
        </section>
        {loadFailed ? <div role="alert" className="text-sm text-error">No se pudo consultar el enlace.<Button variant="text" onPress={() => void refetch()}>Reintentar</Button></div> : <>
          <Switch isSelected={protectedLink} onValueChange={setProtectedLink} isDisabled={busy || isLoading}>Proteger con contraseña</Switch>
          {protectedLink && <Input label="Contraseña" labelPlacement="outside" aria-label="Contraseña del enlace" type="password" autoComplete="new-password" value={password} onValueChange={setPassword} placeholder={data?.protected ? "Vacío para mantener la actual" : "Introduce una contraseña"} isDisabled={busy} />}
          <Input label="Vencimiento (opcional)" labelPlacement="outside" type="date" value={expiresAt} onValueChange={setExpiresAt} min={localDate(new Date())} isDisabled={busy || isLoading} description="El enlace vence al finalizar ese día en tu zona horaria." />
          {expiresAt && <Button variant="text" size="sm" className="self-start" isDisabled={busy} onPress={() => setExpiresAt("")}>Quitar vencimiento</Button>}
          {data && <div className="flex gap-2 items-end"><Input label="Enlace para compartir" labelPlacement="outside" value={link} isReadOnly /><CopyButton value={link} aria-label="Copiar enlace" title="Copiar enlace" isDisabled={busy || expired} /></div>}
        </>}
        {message && <output className="text-sm text-on-surface-variant">{message}</output>}
      </ModalBody>
      <ModalFooter className="flex-wrap">
        {data && <Button variant="text" className="text-error mr-auto" isDisabled={busy} onPress={() => void save(true)}>Eliminar enlace</Button>}
        <Button variant="text" onPress={handleClose} isDisabled={busy}>Cerrar</Button>
        <Button variant="filled" isDisabled={busy || isLoading || loadFailed} isLoading={busy} onPress={() => void save()}>{data ? "Guardar cambios" : "Crear enlace"}</Button>
      </ModalFooter>
    </>
  )
}
