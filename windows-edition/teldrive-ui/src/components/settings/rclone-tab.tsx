import { psQuote, rcloneConfig, saveText } from "@/utils/integrations"
import { Button } from "@tw-material/react"
import { useEffect, useState } from "react"
import toast from "react-hot-toast"

export function RcloneTab() {
  const [name, setName] = useState("teldrive")
  const [host, setHost] = useState(window.location.origin)
  const [token, setToken] = useState("")
  const [chunk, setChunk] = useState("512Mi")
  const [concurrency, setConcurrency] = useState("2")
  const [channel, setChannel] = useState("")
  const [encrypt, setEncrypt] = useState(false)
  const [busy, setBusy] = useState(false)
  const [source, setSource] = useState("D:\\Archivos")
  const [destination, setDestination] = useState("Respaldo")
  const [desktop, setDesktop] = useState(false)
  const [running, setRunning] = useState(false)
  const [jobLog, setJobLog] = useState("")
  const [drive, setDrive] = useState("T:")
  const [dryRun, setDryRun] = useState(true)
  const [winfsp, setWinfsp] = useState(false)
  useEffect(() => {
    let active = true
    async function refresh() {
      try {
        const response = await fetch("/desktop/api/status", { cache: "no-store" })
        if (!response.ok) return
        const data = await response.json() as { desktop?: boolean; winfsp: boolean; rclone: { running: boolean; log?: string; exit?: string } }
        if (active && data.desktop) { setDesktop(true); setWinfsp(data.winfsp); setRunning(data.rclone.running); setJobLog(data.rclone.log || data.rclone.exit || "Sin actividad.") }
      } catch { /* The web/CLI edition does not expose a desktop manager. */ }
    }
    void refresh()
    const timer = setInterval(() => void refresh(), 2500)
    return () => { active = false; clearInterval(timer) }
  }, [])
  async function desktopAction(action: string) {
    setBusy(true)
    try {
      const path = action === "stop" ? "rclone/stop" : action === "install" ? "winfsp/install" : "rclone/start"
      const body = action === "stop" || action === "install" ? {} : { action, config: rcloneConfig({ name, host, token, chunk, concurrency, channel, encrypt }), remote: name, local: source, destination, drive, dryRun }
      const response = await fetch(`/desktop/api/${path}`, { method: "POST", headers: { "Content-Type": "application/json", "X-Teldrive-Desktop": "1" }, body: JSON.stringify(body) })
      if (!response.ok) throw new Error(await response.text())
      toast.success(action === "install" ? "Completa el instalador oficial de WinFsp." : action === "stop" ? "Tarea detenida" : "Tarea iniciada; el progreso aparece debajo.")
    } catch (error) { toast.error((error as Error).message) } finally { setBusy(false) }
  }
  const control = "w-full rounded-xl bg-surface border border-outline-variant p-3 text-sm mt-2"
  const prefix = "& .\\rclone.exe --config '.\\rclone-teldrive.conf'"
  const remote = `${name}:${destination}`
  async function connect() {
    setBusy(true)
    try {
      const response = await fetch("/api/integrations/rclone/token", { method: "POST", credentials: "same-origin", headers: { "X-Teldrive-Intent": "export-rclone" } })
      if (!response.ok) throw new Error(response.status === 404 ? "Esta función requiere el nuevo binario personalizado." : "No se pudo exportar la sesión. Vuelve a iniciar sesión.")
      const data = await response.json() as { token: string }
      setToken(data.token)
      toast.success("Sesión conectada a rclone")
    } catch (error) { toast.error((error as Error).message) } finally { setBusy(false) }
  }
  function exportConfig() { try { saveText("rclone-teldrive.conf", rcloneConfig({ name, host, token, chunk, concurrency, channel, encrypt })); toast.success("Configuración de rclone exportada") } catch (error) { toast.error((error as Error).message) } }
  return <div className="h-full overflow-y-auto p-4 space-y-6">
    <section className="rounded-3xl bg-surface-container-low p-6 space-y-4">
      <h2 className="text-xl font-semibold">Conectar rclone con Teldrive</h2>
      <p className="text-sm text-on-surface-variant">Usa el fork tgdrive/rclone v1.73.1 incluido en el paquete de Windows. Puedes listar, copiar, descargar y montar tu unidad desde el equipo donde ejecutes rclone.</p>
      <a className="text-sm text-primary underline" href="https://github.com/tgdrive/rclone/releases/tag/v1.73.1" target="_blank" rel="noreferrer">Ver versión compatible</a>
      <label className="block text-sm">Nombre del remoto<input className={control} value={name} onChange={(e) => setName(e.target.value)} /></label>
      <label className="block text-sm">Dirección del servidor<input className={control} type="url" value={host} onChange={(e) => setHost(e.target.value)} placeholder="http://localhost:8080" /></label>
      <p className="text-xs text-on-surface-variant">Debe ser accesible desde el equipo de rclone. Usa la raíz del servidor, sin añadir /api.</p>
      <div className="flex flex-wrap gap-2"><Button variant="filledTonal" isLoading={busy} onPress={() => void connect()}>Usar mi sesión actual</Button>{token && <Button variant="text" onPress={() => setToken("")}>Quitar token del formulario</Button>}</div>
      <label className="block text-sm">Token de acceso<input className={control} type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} placeholder="Conecta tu sesión o pega un token existente" /></label>
      <p className="text-xs text-on-surface-variant">El token da acceso a tu cuenta y caduca con la sesión. No se guarda en el navegador. Revocar la sesión en Cuenta revoca también rclone; protege el archivo exportado y vuelve a generarlo al caducar.</p>
      <div className="grid sm:grid-cols-2 gap-4"><label className="block text-sm">Tamaño de fragmento<select className={control} value={chunk} onChange={(e) => setChunk(e.target.value)}>{["64Mi", "128Mi", "256Mi", "512Mi", "1024Mi"].map((size) => <option key={size}>{size}</option>)}</select></label><label className="block text-sm">Subidas simultáneas<select className={control} value={concurrency} onChange={(e) => setConcurrency(e.target.value)}>{[1, 2, 3, 4].map((n) => <option key={n} value={n}>{n}</option>)}</select></label></div>
      <label className="block text-sm">ID de canal (opcional)<input className={control} value={channel} onChange={(e) => setChannel(e.target.value)} placeholder="Vacío: canal predeterminado; sin prefijo -100" /></label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={encrypt} onChange={(e) => setEncrypt(e.target.checked)} />Cifrar archivos con Teldrive</label>
      {encrypt && <p className="text-xs text-on-surface-variant">Configura y conserva tg.uploads.encryption-key en el servidor antes de subir archivos cifrados.</p>}
      <Button variant="filled" isDisabled={!token} onPress={exportConfig}>Descargar configuración de rclone</Button>
      <p className="text-xs text-on-surface-variant">La configuración activa la verificación BLAKE3 del fork de Teldrive. Los archivos antiguos sin hash seguirán disponibles.</p>
    </section>
    <section className="rounded-3xl bg-surface-container-low p-6 space-y-4">
      <h2 className="text-xl font-semibold">Copiar y montar archivos</h2>
      <label className="block text-sm">Carpeta local<input className={control} value={source} onChange={(e) => setSource(e.target.value)} /></label>
      <label className="block text-sm">Carpeta de la unidad<input className={control} value={destination} onChange={(e) => setDestination(e.target.value)} /></label>
      {desktop && <div className="rounded-2xl border border-primary/40 p-4 space-y-4">
        <h3 className="font-semibold">Ejecutar desde Teldrive Desktop</h3>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={dryRun} onChange={(e) => setDryRun(e.target.checked)} />Simular las copias antes de transferir archivos</label>
        <div className="flex flex-wrap gap-2">{[{ action: "list", title: "Comprobar conexión" }, { action: "upload", title: "Copiar a Teldrive" }, { action: "download", title: "Descargar al equipo" }].map(({ action, title }) => <Button key={action} variant="filledTonal" isDisabled={!token || busy || running} onPress={() => void desktopAction(action)}>{title}</Button>)}</div>
        <label className="block text-sm">Letra de unidad<select className={control} value={drive} onChange={(e) => setDrive(e.target.value)}>{"DEFGHIJKLMNOPQRSTUVWXYZ".split("").map((letter) => <option key={letter}>{letter}:</option>)}</select></label>
        <div className="flex flex-wrap gap-2"><Button variant="filled" isDisabled={!token || busy || running || !winfsp} onPress={() => void desktopAction("mount")}>Montar unidad</Button><Button variant="text" isDisabled={!running || busy} onPress={() => void desktopAction("stop")}>Detener tarea / desmontar</Button>{!winfsp && <Button variant="filledTonal" isDisabled={busy} onPress={() => void desktopAction("install")}>Instalar WinFsp incluido</Button>}</div>
        <p className="text-sm text-on-surface-variant">{running ? "Tarea en ejecución. Puedes navegar por tu unidad mientras continúa." : "Sin tareas en ejecución."} {!winfsp && "Windows solicitará permiso de administrador para instalar el controlador de montaje."}</p>
        <pre className="max-h-64 overflow-auto rounded-xl bg-surface p-3 text-xs whitespace-pre-wrap break-all" aria-label="Progreso de rclone">{jobLog}</pre>
        <a className="text-sm text-primary underline" href="/desktop">Abrir controles del programa</a>
        <p className="text-xs text-on-surface-variant">WinFsp - Windows File System Proxy, Copyright (C) Bill Zissimopoulos · <a href="https://github.com/winfsp/winfsp" target="_blank" rel="noreferrer" className="underline">Proyecto y licencia</a></p>
      </div>}
      {[{ title: "Comprobar conexión", command: `${prefix} lsd ${psQuote(`${name}:`)}` }, { title: "Simular copia hacia Teldrive", command: `${prefix} copy ${psQuote(source)} ${psQuote(remote)} --transfers ${concurrency} --progress --dry-run` }, { title: "Copiar hacia Teldrive", command: `${prefix} copy ${psQuote(source)} ${psQuote(remote)} --transfers ${concurrency} --progress` }, { title: "Descargar al equipo", command: `${prefix} copy ${psQuote(remote)} ${psQuote(source)} --transfers ${concurrency} --progress` }, { title: "Montar como unidad T:", command: `${prefix} mount ${psQuote(`${name}:`)} 'T:' --vfs-cache-mode writes` }].map(({ title, command }) => <div key={title} className="rounded-2xl border border-outline-variant/40 p-4 space-y-3"><h3 className="font-medium text-sm">{title}</h3><pre className="text-xs whitespace-pre-wrap break-all">{command}</pre><Button size="sm" variant="text" onPress={() => void navigator.clipboard.writeText(command).then(() => toast.success("Comando copiado"), () => toast.error("No se pudo copiar"))}>Copiar comando</Button></div>)}
      <p className="text-sm text-on-surface-variant">{desktop ? "También puedes usar estos comandos manualmente con el archivo de configuración exportado. Los controles de arriba ejecutan el rclone incluido en el programa." : "Ejecuta los comandos de PowerShell junto a rclone.exe y el archivo descargado. El montaje en Windows requiere WinFsp instalado y una letra de unidad libre. Las copias se ejecutan en la terminal; su progreso aparece allí."}</p>
      <a className="text-sm text-primary underline" href="https://winfsp.dev/rel/" target="_blank" rel="noreferrer">Descargar WinFsp para montar la unidad</a>
    </section>
  </div>
}
