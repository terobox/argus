export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B"
  const units = ["B", "KiB", "MiB", "GiB", "TiB"]
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function formatRate(n: number): string {
  return `${formatBytes(n)}/s`
}

export function formatPercent(n: number): string {
  return `${n.toFixed(0)}%`
}

function pad(n: number): string {
  return n.toString().padStart(2, "0")
}

export function formatClock(ts: number): string {
  const d = new Date(ts * 1000)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function formatDateTime(ts: number): string {
  const d = new Date(ts * 1000)
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function timeAgo(ts: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - ts))
  if (s < 5) return "just now"
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

// lastSeenAgo renders a node's last report time; a zero timestamp means the
// agent has never reported yet (last_seen is 0 until the first ingest).
export function lastSeenAgo(ts: number): string {
  if (ts <= 0) return "never"
  return timeAgo(ts)
}
