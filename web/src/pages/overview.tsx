import { Link } from "react-router-dom"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useNodesStream } from "@/lib/sse"
import { timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

export function OverviewPage() {
  const { nodes, connected } = useNodesStream()
  const online = nodes?.filter((n) => n.online).length ?? 0

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-lg font-semibold">Overview</h1>
        <p className="text-sm text-muted-foreground">
          {connected ? "Live" : "Connecting…"} · {online} of {nodes?.length ?? 0}{" "}
          online
        </p>
      </header>

      {nodes === null ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : nodes.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No nodes yet. Add one in Settings.
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {nodes.map((n) => (
            <Link
              key={n.node_id}
              to={`/nodes/${encodeURIComponent(n.node_id)}`}
              className="group"
            >
              <Card className="h-full transition-colors group-hover:bg-accent">
                <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    {n.hostname || n.node_id}
                  </CardTitle>
                  <span
                    className={cn(
                      "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px]",
                      n.online
                        ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                        : "border-border bg-muted text-muted-foreground",
                    )}
                  >
                    <span
                      className={cn(
                        "size-1.5 rounded-full",
                        n.online ? "bg-emerald-500" : "bg-muted-foreground",
                      )}
                    />
                    {n.online ? "online" : "offline"}
                  </span>
                </CardHeader>
                <CardContent className="text-xs text-muted-foreground">
                  <div>Last report {timeAgo(n.last_seen)}</div>
                  {n.hostname && (
                    <div className="mt-1 truncate font-mono text-[11px]">
                      {n.node_id}
                    </div>
                  )}
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
