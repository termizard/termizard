export interface TabEntry {
  id: number
  title?: string
}

export function reorderTabsAt<T>(tabs: T[], fromIdx: number, toIdx: number): T[] {
  if (fromIdx === toIdx || fromIdx < 0 || toIdx < 0 || fromIdx >= tabs.length) return tabs
  const next = [...tabs]
  const [removed] = next.splice(fromIdx, 1)
  next.splice(toIdx, 0, removed)
  return next
}
