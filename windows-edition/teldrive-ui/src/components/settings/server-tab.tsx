import { cliGroups, cliLabels } from "@/config/cli-labels"
import schema from "@/config/cli-schema.json"
import { psQuote, saveText, serverConfig } from "@/utils/integrations"
import { Button } from "@tw-material/react"
import { useState } from "react"
import toast from "react-hot-toast"

export function ServerTab() {
  const [values, setValues] = useState<Record<string, string>>({})
  const [filter, setFilter] = useState("")
  const [user, setUser] = useState("")
  const [config, setConfig] = useState("config.toml")
  const [concurrent, setConcurrent] = useState("4")
  const [dryRun, setDryRun] = useState(true)
  const [pending, setPending] = useState(false)
  const [uploads, setUploads] = useState(false)
  const command = `& .\\teldrive.exe check --config ${psQuote(config)}${user ? ` --user ${psQuote(user)}` : ""} --concurrent ${concurrent} --export-file 'results.json'${dryRun ? " --dry-run" : ""}${pending ? " --clean-pending" : ""}${uploads ? " --clean-uploads" : ""}`
  const control = "w-full rounded-xl bg-surface border border-outline-variant p-3 text-sm"
  return <div className="h-full overflow-y-auto p-4 space-y-6">
    <section className="rounded-3xl bg-surface-container-low p-6 space-y-4">
      <h2 className="text-xl font-semibold">Opciones del servidor</h2>
      <p className="text-sm text-on-surface-variant">Prepara las {schema.length} opciones de «teldrive run», incluyendo la conversión multimedia. Activa las que quieras exportar. Este formulario no lee ni modifica la configuración del servidor en ejecución: aplica el archivo TOML en el equipo servidor y reinicia para usarlo.</p>
      <label className="block text-sm">Buscar opción<input className={`${control} mt-2`} value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Puerto, reproducción, caché…" /></label>
      {Object.entries(cliGroups).map(([group, label]) => {
        const fields = schema.map((field, i) => ({ ...field, label: cliLabels[i] })).filter((field) => field.path.startsWith(`${group}.`) && `${field.label} ${field.path}`.toLocaleLowerCase("es").includes(filter.toLocaleLowerCase("es")))
        if (!fields.length) return null
        return <details key={group} open={filter ? true : undefined} className="border border-outline-variant/40 rounded-2xl p-4">
          <summary className="cursor-pointer font-medium">{label} · {fields.length}</summary>
          <div className="mt-4 space-y-4">{fields.map((field) => {
            const enabled = values[field.path] !== undefined
            const secret = /secret|password|encryption-key|data-source|app-hash/.test(field.path)
            return <div key={field.path} className="space-y-2">
              <label className="flex gap-2 text-sm items-start"><input type="checkbox" className="mt-1 accent-primary" checked={enabled} onChange={(e) => setValues((previous) => { const next = { ...previous }; if (e.target.checked) next[field.path] = field.default; else delete next[field.path]; return next })} />{field.label}</label>
              <code className="block text-xs text-on-surface-variant break-all">--{field.path.replaceAll(".", "-")}</code>
              {enabled && (field.type === "bool" ? <select aria-label={field.label} className={control} value={values[field.path]} onChange={(e) => setValues({ ...values, [field.path]: e.target.value })}><option value="true">Activado</option><option value="false">Desactivado</option></select> : <input aria-label={field.label} className={control} type={secret ? "password" : ["int", "int64"].includes(field.type) ? "number" : "text"} autoComplete="off" value={values[field.path]} onChange={(e) => setValues({ ...values, [field.path]: e.target.value })} placeholder={field.type === "slice" ? "Valores separados por comas" : field.type === "duration" ? "Ejemplo: 30s, 5m, 1h, 7d" : "Valor"} />)}
            </div>
          })}</div>
        </details>
      })}
      <div className="flex flex-wrap gap-3"><Button variant="filled" isDisabled={!Object.keys(values).length} onPress={() => { try { saveText("config-preparada.toml", serverConfig(values)); toast.success("Configuración exportada") } catch (error) { toast.error((error as Error).message) } }}>Exportar TOML ({Object.keys(values).length})</Button><Button variant="text" onPress={() => setValues({})}>Vaciar formulario</Button></div>
      <p className="text-xs text-on-surface-variant">El archivo contiene únicamente las opciones seleccionadas. Combínalo con tu configuración existente antes de iniciar: conexión PostgreSQL y secreto JWT son obligatorios. Los secretos escritos aquí solo permanecen en este formulario hasta salir.</p>
    </section>
    <section className="rounded-3xl bg-surface-container-low p-6 space-y-4">
      <h2 className="text-xl font-semibold">Comprobar integridad</h2>
      <p className="text-sm text-on-surface-variant">Prepara «teldrive check» para comparar los archivos con Telegram y exportar un informe. Ejecuta el comando en una terminal del servidor.</p>
      <label className="block text-sm">Archivo de configuración<input className={`${control} mt-2`} value={config} onChange={(e) => setConfig(e.target.value)} /></label>
      <label className="block text-sm">Usuario de Telegram<input className={`${control} mt-2`} value={user} onChange={(e) => setUser(e.target.value)} placeholder="Sin @; vacío para elegir en terminal" /></label>
      <label className="block text-sm">Canales simultáneos<select className={`${control} mt-2`} value={concurrent} onChange={(e) => setConcurrent(e.target.value)}>{[1, 2, 4, 8].map((n) => <option key={n} value={n}>{n}</option>)}</select></label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={dryRun} onChange={(e) => setDryRun(e.target.checked)} />Simular sin borrar datos (recomendado)</label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={pending} onChange={(e) => setPending(e.target.checked)} />Incluir archivos pendientes de eliminación</label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={uploads} onChange={(e) => setUploads(e.target.checked)} />Incluir subidas incompletas</label>
      {!dryRun && <p className="text-sm text-error">Al ejecutar este comando se eliminarán partes ausentes y mensajes huérfanos; las opciones de limpieza pueden borrar más datos.</p>}
      <pre className="text-xs whitespace-pre-wrap break-all rounded-xl bg-surface p-4">{command}</pre>
      <Button variant="filledTonal" onPress={() => void navigator.clipboard.writeText(command).then(() => toast.success("Comando copiado"), () => toast.error("No se pudo copiar el comando"))}>Copiar comando de PowerShell</Button>
    </section>
    <section className="rounded-3xl bg-surface-container-low p-6 space-y-3"><h2 className="text-xl font-semibold">Otros comandos revisados</h2><p className="text-sm text-on-surface-variant">«version» está disponible en Información. «completion» genera autocompletado para bash, zsh, fish y PowerShell. «help» muestra la ayuda del CLI. «upgrade» instala la versión oficial y sustituye esta edición personalizada; para conservar la interfaz, usa el script de compilación incluido.</p></section>
  </div>
}
