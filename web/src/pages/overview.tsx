import { useMemo, useState } from "react"
import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import { Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { useNodesStream, type StatusEvent } from "@/lib/sse"
import { formatBytes, formatRate, lastSeenAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

const GRID =
  "grid grid-cols-[minmax(190px,1.5fr)_minmax(56px,0.6fr)_minmax(150px,1.1fr)_minmax(150px,1.1fr)_minmax(150px,1.2fr)_minmax(64px,0.6fr)] items-center gap-x-4"

export function OverviewPage() {
  const { nodes, connected } = useNodesStream()
  const [tag, setTag] = useState<string | null>(null)
  const [query, setQuery] = useState("")

  const all = nodes ?? []
  const tags = useMemo(
    () =>
      Array.from(new Set((nodes ?? []).flatMap((n) => n.tags ?? []))).sort(),
    [nodes]
  )

  // Search and tag filters combine: the query matches node ID, hostname, or an
  // exact tag, case-insensitively, and updates on every keystroke.
  const q = query.trim().toLowerCase()
  const filtered = all.filter((n) => {
    if (tag && !n.tags?.includes(tag)) return false
    if (!q) return true
    return (
      n.node_id.toLowerCase().includes(q) ||
      (n.hostname ?? "").toLowerCase().includes(q) ||
      (n.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  })
  const online = filtered.filter((n) => n.online).length
  const hasFilters = tag !== null || q !== ""

  return (
    <div className="flex flex-col gap-4">
      <header>
        <h1 className="text-lg font-semibold">Overview</h1>
        <p className="text-sm text-muted-foreground">
          {connected ? "Live" : "Connecting…"} · {online} of {filtered.length}{" "}
          online · updates every 5s
        </p>
      </header>

      {nodes !== null && all.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {tags.length > 0 && (
            <>
              <FilterChip active={tag === null} onClick={() => setTag(null)}>
                All
              </FilterChip>
              {tags.map((t) => (
                <FilterChip
                  key={t}
                  active={tag === t}
                  onClick={() => setTag(t)}
                >
                  {t}
                </FilterChip>
              ))}
            </>
          )}
          <div className="relative ml-auto w-full sm:w-56">
            <Search className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search machines…"
              aria-label="Search machines"
              className="h-7 pr-7 pl-7"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery("")}
                aria-label="Clear search"
                className="absolute top-1/2 right-1 -translate-y-1/2 p-0.5 text-muted-foreground transition-colors hover:text-foreground"
              >
                <X className="size-3" />
              </button>
            )}
          </div>
        </div>
      )}

      {nodes === null ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : filtered.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-10 text-center text-sm text-muted-foreground">
            <span>
              {all.length === 0
                ? "No nodes yet. Add one in Settings."
                : "No machines match this search."}
            </span>
            {all.length > 0 && hasFilters && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setTag(null)
                  setQuery("")
                }}
              >
                Clear filters
              </Button>
            )}
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="hidden overflow-x-auto md:block">
            <div className="min-w-205">
              <div
                className={cn(
                  GRID,
                  "border-b border-border px-3 py-2 text-xs text-muted-foreground"
                )}
              >
                <span>Machine</span>
                <span className="text-right">CPU</span>
                <span className="text-right">Memory</span>
                <span className="text-right">Disk</span>
                <span className="text-right">Network ↓ / ↑</span>
                <span className="text-right">Agent</span>
              </div>
              {filtered.map((n) => {
                const isPending = n.last_seen <= 0
                return (
                  <Link
                    key={n.node_id}
                    to={`/nodes/${encodeURIComponent(n.node_id)}`}
                    className={cn(
                      GRID,
                      "border-b border-border/50 px-3 py-2 text-sm transition-colors last:border-0 hover:bg-accent",
                      isPending && "opacity-60"
                    )}
                  >
                    <div className="flex min-w-0 items-center gap-2">
                      <StatusDot online={n.online} pending={isPending} />
                      <span className="min-w-0">
                        <span className="block truncate font-medium">
                          {n.hostname || n.node_id}
                        </span>
                        <span className="block truncate font-mono text-[11px] text-muted-foreground">
                          {secondaryLabel(n)}
                        </span>
                      </span>
                    </div>
                    <span className="text-right font-medium tabular-nums">
                      {n.live ? `${n.live.cpu_percent.toFixed(0)}%` : "—"}
                    </span>
                    <MemDiskCell
                      used={n.live?.mem_used}
                      total={n.live?.mem_total}
                    />
                    <MemDiskCell
                      used={n.live?.disk_used}
                      total={n.live?.disk_total}
                    />
                    <NetCell
                      rx={n.live?.net_rx_rate}
                      tx={n.live?.net_tx_rate}
                    />
                    <span className="text-right font-mono text-xs text-muted-foreground">
                      {n.agent_version || "—"}
                    </span>
                  </Link>
                )
              })}
            </div>
          </div>

          {/* On phones the six-column grid is unreadable; show one card per node
              instead. The table above is unchanged from md upwards. */}
          <div className="flex flex-col gap-2 md:hidden">
            {filtered.map((n) => {
              const isPending = n.last_seen <= 0
              return (
                <Link
                  key={n.node_id}
                  to={`/nodes/${encodeURIComponent(n.node_id)}`}
                  className={cn(
                    "border border-border bg-card p-3 text-sm transition-colors hover:bg-accent",
                    isPending && "opacity-60"
                  )}
                >
                  <div className="flex items-center gap-2">
                    <StatusDot online={n.online} pending={isPending} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">
                        {n.hostname || n.node_id}
                      </span>
                      <span className="block truncate font-mono text-[11px] text-muted-foreground">
                        {secondaryLabel(n)}
                      </span>
                    </span>
                    <span className="font-mono text-[11px] text-muted-foreground">
                      {n.agent_version || "—"}
                    </span>
                  </div>
                  <div className="mt-2 grid grid-cols-3 gap-2 text-[11px]">
                    <MobileStat
                      label="CPU"
                      value={n.live ? `${n.live.cpu_percent.toFixed(0)}%` : "—"}
                    />
                    <MobileStat
                      label="Memory"
                      value={usagePercent(n.live?.mem_used, n.live?.mem_total)}
                    />
                    <MobileStat
                      label="Disk"
                      value={usagePercent(
                        n.live?.disk_used,
                        n.live?.disk_total
                      )}
                    />
                  </div>
                  <div className="mt-1 text-[11px] text-muted-foreground">
                    ↓ {n.live ? formatRate(n.live.net_rx_rate) : "—"} · ↑{" "}
                    {n.live ? formatRate(n.live.net_tx_rate) : "—"}
                  </div>
                </Link>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}

function StatusDot({ online, pending }: { online: boolean; pending: boolean }) {
  return (
    <span
      className={cn(
        "size-1.5 shrink-0 rounded-full",
        online
          ? "bg-emerald-500"
          : pending
            ? "border border-muted-foreground bg-transparent"
            : "bg-muted-foreground"
      )}
    />
  )
}

// secondaryLabel is the muted line under a machine's name: pending nodes say so,
// installed nodes show their node ID, and nodes without host info fall back to
// the last report time.
function secondaryLabel(n: StatusEvent): string {
  if (n.last_seen <= 0) return "pending install"
  if (n.hostname) return n.node_id
  return `last report ${lastSeenAgo(n.last_seen)}`
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-full border px-2.5 py-0.5 text-xs transition-colors",
        active
          ? "border-primary bg-primary text-primary-foreground"
          : "border-border text-muted-foreground hover:bg-accent"
      )}
    >
      {children}
    </button>
  )
}

function usagePercent(used?: number, total?: number): string {
  if (used === undefined || total === undefined || total <= 0) return "—"
  return `${((used / total) * 100).toFixed(0)}%`
}

function MemDiskCell({ used, total }: { used?: number; total?: number }) {
  if (used === undefined || total === undefined || total <= 0) {
    return <span className="text-right text-muted-foreground">—</span>
  }
  return (
    <span className="text-right tabular-nums">
      <span className="block font-medium">{usagePercent(used, total)}</span>
      <span className="block text-[11px] text-muted-foreground">
        {formatBytes(used)} / {formatBytes(total)}
      </span>
    </span>
  )
}

function MobileStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate font-medium tabular-nums">{value}</span>
    </div>
  )
}

function NetCell({ rx, tx }: { rx?: number; tx?: number }) {
  if (rx === undefined || tx === undefined) {
    return <span className="text-right text-muted-foreground">—</span>
  }
  return (
    <span className="text-right tabular-nums">
      <span className="block text-[11px] text-muted-foreground">
        ↓ {formatRate(rx)}
      </span>
      <span className="block text-[11px] text-muted-foreground">
        ↑ {formatRate(tx)}
      </span>
    </span>
  )
}
