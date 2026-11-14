import { useEffect, useState } from "react"
import { useParams } from "react-router-dom"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { MetricChart, MultiLineChart, Sparkline } from "@/components/chart"
import { api, type NodeDetail, type SeriesPoint } from "@/lib/api"
import { useSampleStream } from "@/lib/sse"
import { formatBytes, formatPercent, formatRate, lastSeenAgo, timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

const RANGES = [
  { label: "1h", seconds: 3600 },
  { label: "6h", seconds: 6 * 3600 },
  { label: "24h", seconds: 24 * 3600 },
  { label: "7d", seconds: 7 * 24 * 3600 },
  { label: "30d", seconds: 30 * 24 * 3600 },
]

const ONLINE_AFTER = 90 // seconds

export function NodePage() {
  const { id = "" } = useParams()
  const [detail, setDetail] = useState<NodeDetail | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [range, setRange] = useState(RANGES[0].seconds)
  const [points, setPoints] = useState<SeriesPoint[]>([])
  const [resolution, setResolution] = useState("")
  const { sample } = useSampleStream(id)

  useEffect(() => {
    let cancelled = false
    api
      .getNode(id)
      .then((d) => {
        if (!cancelled) setDetail(d)
      })
      .catch(() => {
        if (!cancelled) setNotFound(true)
      })
    return () => {
      cancelled = true
    }
  }, [id])

  useEffect(() => {
    let cancelled = false
    const to = Math.floor(Date.now() / 1000)
    const from = to - range
    api
      .series(id, from, to)
      .then((r) => {
        if (cancelled) return
        setPoints(r.points)
        setResolution(r.resolution)
      })
      .catch(() => {
        if (!cancelled) setPoints([])
      })
    return () => {
      cancelled = true
    }
  }, [id, range])

  if (notFound) {
    return <p className="text-sm text-muted-foreground">Node not found.</p>
  }

  const online = detail ? Date.now() / 1000 - detail.last_seen <= ONLINE_AFTER : false
  const hi = detail?.host_info
  // Prefer the live SSE sample once it arrives; fall back to the latest sample
  // from the detail response so the top stats are never blank on first paint.
  const live = sample ?? detail?.latest_sample ?? null

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold">{hi?.hostname || id}</h1>
          <p className="text-sm text-muted-foreground">
            <span
              className={cn(
                "mr-1 inline-block size-1.5 rounded-full align-middle",
                online ? "bg-emerald-500" : "bg-muted-foreground",
              )}
            />
            {online ? "online" : "offline"} · last report{" "}
            {detail ? lastSeenAgo(detail.last_seen) : "…"} ·{" "}
            <span className="font-mono text-xs">{id}</span>
          </p>
        </div>
        <div className="flex gap-1">
          {RANGES.map((r) => (
            <Button
              key={r.label}
              variant={range === r.seconds ? "default" : "outline"}
              size="xs"
              onClick={() => setRange(r.seconds)}
            >
              {r.label}
            </Button>
          ))}
        </div>
      </header>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <UsageStat
          label="CPU"
          value={live ? formatPercent(live.cpu.used_percent) : "—"}
          sub="usage"
          percent={live?.cpu.used_percent}
          points={points}
          accessor={(p) => p.cpu}
          color="var(--chart-1)"
        />
        <UsageStat
          label="Memory"
          value={live ? formatBytes(live.mem.used_bytes) : "—"}
          sub={live ? `of ${formatBytes(live.mem.total_bytes)}` : undefined}
          percent={
            live && live.mem.total_bytes > 0
              ? (live.mem.used_bytes / live.mem.total_bytes) * 100
              : undefined
          }
          points={points}
          accessor={(p) => p.mem_used}
          color="var(--chart-2)"
        />
        <UsageStat
          label="Disk"
          value={live?.disk_cap ? formatBytes(live.disk_cap.used_bytes) : "—"}
          sub={live?.disk_cap ? `of ${formatBytes(live.disk_cap.total_bytes)}` : undefined}
          percent={
            live?.disk_cap && live.disk_cap.total_bytes > 0
              ? (live.disk_cap.used_bytes / live.disk_cap.total_bytes) * 100
              : undefined
          }
          points={points}
          accessor={(p) => p.disk_used ?? 0}
          color="var(--chart-3)"
        />
        <UsageStat
          label="Load"
          value={live ? live.load.load1.toFixed(2) : "—"}
          sub={
            live
              ? `5m ${live.load.load5.toFixed(2)} · 15m ${live.load.load15.toFixed(2)}`
              : undefined
          }
          points={points}
          accessor={(p) => p.load1}
          color="var(--chart-4)"
        />
      </div>

      <p className="text-xs text-muted-foreground">
        {live
          ? `Updated ${timeAgo(live.ts)} · refreshes about every 15s`
          : "Waiting for the agent's first report (arrives within ~15s)"}
      </p>

      {hi && (
        <Card>
          <CardHeader>
            <CardTitle>Host</CardTitle>
            <CardDescription>Reported by the agent</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <Row k="Hostname" v={hi.hostname} />
            <Row k="OS" v={hi.os} />
            <Row k="Kernel" v={hi.kernel} />
            <Row k="Arch" v={hi.arch} />
            <Row k="CPU" v={hi.cpu_model} />
            <Row k="Cores" v={String(hi.cpu_cores)} />
            <Row k="Memory" v={formatBytes(hi.mem_total)} />
            <Row k="Agent" v={hi.agent_version} />
            {hi.public_ip && <Row k="Public IP" v={hi.public_ip} />}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>CPU</CardTitle>
          <CardDescription>Total utilization % · {resolution || "…"}</CardDescription>
        </CardHeader>
        <CardContent>
          <MetricChart
            points={points}
            accessor={(p) => p.cpu}
            name="cpu"
            color="var(--chart-1)"
            format={formatPercent}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Load average</CardTitle>
        </CardHeader>
        <CardContent>
          <MultiLineChart
            points={points}
            series={[
              { key: "l1", name: "1m", color: "var(--chart-1)", value: (p) => p.load1 },
              { key: "l5", name: "5m", color: "var(--chart-2)", value: (p) => p.load5 },
              { key: "l15", name: "15m", color: "var(--chart-3)", value: (p) => p.load15 },
            ]}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Memory</CardTitle>
          <CardDescription>Used</CardDescription>
        </CardHeader>
        <CardContent>
          <MetricChart
            points={points}
            accessor={(p) => p.mem_used}
            name="mem"
            color="var(--chart-2)"
            format={formatBytes}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Network</CardTitle>
          <CardDescription>Bytes per second</CardDescription>
        </CardHeader>
        <CardContent>
          <MultiLineChart
            points={points}
            format={formatRate}
            series={[
              { key: "rx", name: "rx", color: "var(--chart-2)", value: (p) => p.net_rx_rate },
              { key: "tx", name: "tx", color: "var(--chart-3)", value: (p) => p.net_tx_rate },
            ]}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Disk I/O</CardTitle>
          <CardDescription>Bytes per second</CardDescription>
        </CardHeader>
        <CardContent>
          <MultiLineChart
            points={points}
            format={formatRate}
            series={[
              { key: "dr", name: "read", color: "var(--chart-2)", value: (p) => p.disk_read_rate },
              { key: "dw", name: "write", color: "var(--chart-3)", value: (p) => p.disk_write_rate },
            ]}
          />
        </CardContent>
      </Card>
    </div>
  )
}

function UsageStat({
  label,
  value,
  sub,
  percent,
  points,
  accessor,
  color,
}: {
  label: string
  value: string
  sub?: string
  percent?: number
  points: SeriesPoint[]
  accessor: (p: SeriesPoint) => number
  color: string
}) {
  return (
    <Card>
      <CardContent className="py-4">
        <div className="flex items-baseline justify-between gap-2">
          <span className="text-xs text-muted-foreground">{label}</span>
          {sub && (
            <span className="truncate text-[11px] tabular-nums text-muted-foreground">
              {sub}
            </span>
          )}
        </div>
        <div className="mt-1 flex items-end justify-between gap-3">
          <div className="text-2xl font-semibold tabular-nums">{value}</div>
          {points.length > 0 && (
            <div className="h-8 w-24 shrink-0">
              <Sparkline points={points} accessor={accessor} color={color} />
            </div>
          )}
        </div>
        {percent !== undefined && (
          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full"
              style={{
                width: `${Math.max(0, Math.min(100, percent))}%`,
                background: color,
              }}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <div className="flex justify-between gap-4 border-b border-border/50 py-1 last:border-0">
      <span className="text-muted-foreground">{k}</span>
      <span className="font-mono text-xs">{v}</span>
    </div>
  )
}
