export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

export function formatTime(value: string | null | undefined): string {
  if (!value) return 'Never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Never'
  return date.toLocaleString()
}

export function instanceBadgeClass(inst: { is_running: boolean; overall_result: string }): string {
  if (inst.is_running) return 'badge-running'
  if (inst.overall_result === 'success') return 'badge-running'
  if (inst.overall_result === 'partial') return 'badge-warn'
  if (inst.overall_result === 'failed') return 'badge-error'
  return 'badge-stopped'
}

export function dbResultBadgeClass(result: string): string {
  if (result === 'success') return 'badge-running'
  if (result === 'partial') return 'badge-warn'
  if (result === 'failed') return 'badge-error'
  return 'badge-stopped'
}
