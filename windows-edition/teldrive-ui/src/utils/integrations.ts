import schema from "@/config/cli-schema.json"

export function serverConfig(values: Record<string, string>): string {
  const groups: Record<string, string[]> = {}
  for (const field of schema) {
    const value = values[field.path]
    if (value === undefined) continue
    const index = field.path.lastIndexOf(".")
    const section = field.path.slice(0, index)
    const key = field.path.slice(index + 1)
    let encoded: string
    if (field.type === "bool") {
      if (!["true", "false"].includes(value)) throw new Error(`Valor inválido: ${field.path}`)
      encoded = value
    } else if (["int", "int64"].includes(field.type)) {
      if (!/^-?\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new Error(`Número inválido: ${field.path}`)
      encoded = value
    } else if (field.type === "slice") encoded = JSON.stringify(value.split(",").map((item) => item.trim()).filter(Boolean))
    else encoded = JSON.stringify(value)
    groups[section] ??= []
    groups[section].push(`${key} = ${encoded}`)
  }
  return `# Configuración preparada en Teldrive. Revisar y reiniciar para aplicarla.\n${Object.entries(groups).map(([section, entries]) => `\n[${section}]\n${entries.join("\n")}\n`).join("")}`
}

export function psQuote(value: string) { return `'${value.replaceAll("'", "''")}'` }

export function rcloneConfig({ name, host, token, chunk, concurrency, channel, encrypt }: { name: string; host: string; token: string; chunk: string; concurrency: string; channel: string; encrypt: boolean }) {
  if (!/^[a-zA-Z][a-zA-Z0-9_-]{1,40}$/.test(name)) throw new Error("Usa un nombre de remoto de 2 a 41 letras, números, guiones o guiones bajos.")
  const url = new URL(host)
  if (!["http:", "https:"].includes(url.protocol) || url.search || url.hash || url.username || url.password || !["", "/"].includes(url.pathname)) throw new Error("Introduce la dirección raíz del servidor, sin /api ni credenciales.")
  if (!token || /[\r\n]/.test(token)) throw new Error("Falta un token válido de sesión.")
  if (!/^[1-4]$/.test(concurrency)) throw new Error("Selecciona entre 1 y 4 subidas simultáneas.")
  if (!["64Mi", "128Mi", "256Mi", "512Mi", "1024Mi"].includes(chunk)) throw new Error("Tamaño de fragmento inválido.")
  if (channel && !/^\d+$/.test(channel)) throw new Error("El ID del canal debe contener solo números, sin el prefijo -100.")
  return `[${name}]\ntype = teldrive\napi_host = ${url.origin}\naccess_token = ${token.trim()}\nchunk_size = ${chunk}\nupload_concurrency = ${concurrency}\npage_size = 500\nhash_enabled = true\nencrypt_files = ${encrypt}\n${channel ? `channel_id = ${channel}\n` : ""}`
}

export function saveText(name: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: "text/plain;charset=utf-8" }))
  const anchor = document.createElement("a")
  anchor.href = url; anchor.download = name
  document.body.append(anchor); anchor.click(); anchor.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
