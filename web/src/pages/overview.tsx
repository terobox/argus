import { useMemo, useState } from "react"
import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import { Card, CardContent } from "@/components/ui/card"
import { useNodesStream } from "@/lib/sse"
import { formatBytes, formatRate, lastSeenAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

const GRID =
  "grid grid-cols-[minmax(190px,1.5fr)_minmax(56px,0.6fr)_minmax(150px,1.1fr)_minmax(150px,1.1fr)_minmax(150px,1.2fr)_minmax(64px,0.6fr)] items-center gap-x-4"

export function OverviewPage() {
  const { nodes, connected } = useNodesStream()
  const [tag, setTag] = useState<string | null>(null)

  const all = nodes ?? []
  const tags = useMemo(
    () => Array.from(new Set(all.flatMap((n) => n.tags ?? []))).sort(),
    [all],
  )
  const filtered = tag ? all.filter((n) => n.tags?.includes(tag)) : all
  const online = filtered.filter((n) => n.online).length

  return (
    <div className="flex flex-col gap-4">
      <header>
        <h1 className="text-lg font-semibold">Overview</h1>
        <p className="text-sm text-muted-foreground">
          {connected ? "Live" : "Connecting…"} · {online} of {filtered.length}{" "}
          online · updates every 5s
        </p>
      </header>

      {tags.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <FilterChip active={tag === null} onClick={() => setTag(null)}>
            All
          </FilterChip>
          {tags.map((t) => (
            <FilterChip key={t} active={tag === t} onClick={() => setTag(t)}>
              {t}
            </FilterChip>
          ))}
        </div>
      )}

      {nodes === null ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : filtered.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No nodes yet. Add one in Settings.
          </CardContent>
        </Card>
      ) : (
        <div className="overflow-x-auto">
          <div className="min-w-205">
            <div
              className={cn(
                GRID,
                "border-b border-border px-3 py-2 text-xs text-muted-foreground",
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
                    isPending && "opacity-60",
                  )}
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <span
                      className={cn(
                        "size-1.5 shrink-0 rounded-full",
                        n.online
                          ? "bg-emerald-500"
                          : isPending
                            ? "border border-muted-foreground bg-transparent"
                            : "bg-muted-foreground",
                      )}
                    />
                    <span className="min-w-0">
                      <span className="block truncate font-medium">
                        {n.hostname || n.node_id}
                      </span>
                      <span className="block truncate font-mono text-[11px] text-muted-foreground">
                        {isPending
                          ? "pending install"
                          : n.hostname
                            ? n.node_id
                            : `last report ${lastSeenAgo(n.last_seen)}`}
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
                  <NetCell rx={n.live?.net_rx_rate} tx={n.live?.net_tx_rate} />
                  <span className="text-right font-mono text-xs text-muted-foreground">
                    {n.agent_version || "—"}
                  </span>
                </Link>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
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
          : "border-border text-muted-foreground hover:bg-accent",
      )}
    >
      {children}
    </button>
  )
}

function MemDiskCell({
  used,
  total,
}: {
  used?: number
  total?: number
}) {
  if (used === undefined || total === undefined || total <= 0) {
    return <span className="text-right text-muted-foreground">—</span>
  }
  return (
    <span className="text-right tabular-nums">
      <span className="block font-medium">
        {((used / total) * 100).toFixed(0)}%
      </span>
      <span className="block text-[11px] text-muted-foreground">
        {formatBytes(used)} / {formatBytes(total)}
      </span>
    </span>
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
