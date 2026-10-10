export const categoryLabels: Record<string, string> = { archive: "Comprimidos", audio: "Audio", image: "Imágenes", video: "Vídeos", document: "Documentos", folder: "Carpetas", other: "Otros" }

export function spanishError(message?: string) {
  const text = message?.toLowerCase() || ""
  if (text.includes("password")) return "Contraseña incorrecta."
  if (text.includes("phone_code") || text.includes("phone code")) return "Código de verificación incorrecto o vencido."
  if (text.includes("phone_number")) return "Número de teléfono no válido."
  if (text.includes("flood") || text.includes("too many")) return "Demasiados intentos. Espera antes de volver a intentarlo."
  if (text.includes("exists")) return "El archivo o registro ya existe."
  if (text.includes("not found")) return "No se encontró el recurso."
  if (text.includes("expired") || text.includes("unauthorized")) return "La sesión ha vencido. Vuelve a iniciar sesión."
  if (text.includes("network") || text.includes("fetch")) return "No se pudo conectar con el servidor."
  if (text.includes("abort")) return "Operación cancelada."
  return "No se pudo completar la operación. Inténtalo de nuevo."
}
