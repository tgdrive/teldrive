import { t } from "@/lib/localization";
export type ConfigOption = { key: string; value: string; line: number };
export type ConfigSection = { name: string; options: ConfigOption[] };
export function parseRcloneConfig(content: string): ConfigSection[] {
  if (content.length > 2 * 1024 * 1024 || content.includes("\0"))
    throw new Error(t("El archivo debe ser una configuración de hasta 2 MB."));
  if (content.includes("RCLONE_ENCRYPT_V0:"))
    throw new Error(t("Desbloquea el archivo cifrado en Teldrive Desktop antes de editarlo."));
  const result: ConfigSection[] = [];
  let section: ConfigSection | undefined;
  for (const [line, text] of content.replace(/\r\n/g, "\n").split("\n").entries()) {
    const heading = text.match(/^\s*\[([^\]]+)\]\s*(?:[#;].*)?$/);
    if (heading) {
      section = result.find((entry) => entry.name === heading[1]);
      if (!section) {
        section = { name: heading[1]!, options: [] };
        result.push(section);
      }
      continue;
    }
    if (!section || /^\s*[#;]/.test(text) || !text.trim()) continue;
    const option = text.match(/^\s*([\w.-]+)\s*=\s*(.*)$/);
    if (!option)
      throw new Error(t("Revisa la opción de la línea {0}: debe usar clave = valor.", [line + 1]));
    section.options.push({ key: option[1]!, value: option[2]!, line });
  }
  return result;
}
export function setRcloneOption(content: string, option: ConfigOption, value: string) {
  if (/[\r\n\0]/.test(value)) throw new Error(t("El valor debe ocupar una sola línea."));
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  lines[option.line] = `${option.key} = ${value}`;
  return lines.join("\n");
}
export const privateRcloneOption = (key: string) =>
  /pass|token|secret|key|credential|cookie/i.test(key);
