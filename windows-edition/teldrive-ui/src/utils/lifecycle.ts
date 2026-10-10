export type LifecycleItem = { id: string; name: string; type: string; mimeType?: string; size: number | null; movedAt: string; updatedAt?: string }

export async function lifecycleList(state: "trash" | "spam", signal?: AbortSignal): Promise<LifecycleItem[]> {
  const response = await fetch(`/api/files/lifecycle?state=${state}`, { signal })
  if (!response.ok) throw new Error("No se pudo consultar esta carpeta. Usa el binario personalizado actualizado.")
  return response.json()
}

export async function lifecycleChange(ids: string[], action: "trash" | "spam" | "restore" | "delete", state?: "trash" | "spam") {
  const response = await fetch("/api/files/lifecycle", { method: "POST", headers: { "Content-Type": "application/json", "X-Teldrive-Intent": "file-lifecycle" }, body: JSON.stringify({ ids, action, ...(state ? { state } : {}) }) })
  if (!response.ok) throw new Error("No se pudo completar el cambio. Actualiza la lista e inténtalo de nuevo.")
}
