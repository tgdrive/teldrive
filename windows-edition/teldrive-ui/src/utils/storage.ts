export function storageUsage(categories: { category: string; totalSize: number }[], limitGB: number) {
  const used = categories.filter((item) => item.category !== "folder").reduce((sum, item) => sum + Math.max(0, Number(item.totalSize) || 0), 0)
  const total = Number.isFinite(limitGB) && limitGB > 0 ? limitGB * 1_000_000_000 : 0
  return { used, total, available: Math.max(0, total - used), percent: total ? Math.min(100, used / total * 100) : 0, exceeded: total > 0 && used > total }
}
