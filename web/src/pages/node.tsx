import { useEffect, useRef, useState } from "react"
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

// Gauge threshold colors — traffic-light style.
function gaugeColor(percent: number | undefined): string {
  if (percent === undefined) return "var(--muted-foreground)"
  if (percent >= 85) return "#ef4444" // red-500
  if (percent >= 70) return "#f59e0b" // amber-500
  return "#22c55e" // green-500
}

// RingGauge renders a circular SVG ring with a text value inside plus an
// optional sub-label and sparkline below.
function RingGauge({
  label,
  value,
  sub,
  percent,
  points,
  accessor,
  color,
  loading,
}: {
  label: string
  value: string
  sub?: string
  percent?: number
  points: SeriesPoint[]
  accessor: (p: SeriesPoint) => number
  color: string
  loading?: boolean
}) {
  const r = 26
  const circ = 2 * Math.PI * r
  const fill =
    percent !== undefined ? circ * Math.min(1, Math.max(0, percent / 100)) : 0
  const ring = gaugeColor(percent)

  if (loading) {
    return (
      <Card>
        <CardContent className="py-3 px-4">
          <div className="flex items-center gap-3">
            <div className="size-16 shrink-0 animate-pulse rounded-full bg-muted" />
            <div className="flex-1 space-y-2">
              <div className="h-3 w-12 animate-pulse rounded bg-muted" />
              <div className="h-5 w-16 animate-pulse rounded bg-muted" />
              <div className="h-3 w-20 animate-pulse rounded bg-muted" />
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardContent className="py-3 px-4">
        <div className="flex items-center gap-3">
          {/* Circular ring */}
          <div className="relative shrink-0">
            <svg
              width={64}
              height={64}
              viewBox="0 0 64 64"
              aria-hidden="true"
            >
              {/* Track */}
              <circle
                cx={32}
                cy={32}
                r={r}
                fill="none"
                stroke="var(--muted)"
                strokeWidth={5}
              />
              {/* Progress arc */}
              {percent !== undefined && (
                <circle
                  cx={32}
                  cy={32}
                  r={r}
                  fill="none"
                  stroke={ring}
                  strokeWidth={5}
                  strokeDasharray={`${fill} ${circ}`}
                  strokeLinecap="round"
                  style={{
                    transform: "rotate(-90deg)",
                    transformOrigin: "32px 32px",
                    transition: "stroke-dasharray 0.4s ease",
                  }}
                />
              )}
            </svg>
            <div className="absolute inset-0 flex items-center justify-center">
              <span
                className="text-xs font-bold tabular-nums"
                style={{ color: percent !== undefined ? ring : "var(--muted-foreground)" }}
              >
                {percent !== undefined ? `${Math.round(percent)}%` : "—"}
              </span>
            </div>
          </div>
          {/* Text */}
          <div className="min-w-0 flex-1">
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className="mt-0.5 truncate text-xl font-bold tabular-nums">
              {value}
            </p>
            {sub && (
              <p className="truncate text-[11px] tabular-nums text-muted-foreground">
                {sub}
              </p>
            )}
          </div>
        </div>
        {points.length > 0 && (
          <div className="mt-2 h-7 w-full">
            <Sparkline points={points} accessor={accessor} color={color} height={28} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// LastUpdated shows how long ago the sample was taken and auto-refreshes every
// second so the counter stays accurate without a full re-render cycle.
function LastUpdated({ ts }: { ts: number }) {
  const [, forceRender] = useState(0)
  const ref = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    ref.current = setInterval(() => forceRender((n) => n + 1), 1000)
    return () => {
      if (ref.current) clearInterval(ref.current)
    }
  }, [])

  return (
    <p className="text-xs text-muted-foreground">
      Updated {timeAgo(ts)} · refreshes every ~15s
    </p>
  )
}

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

  const online =
    detail ? Date.now() / 1000 - detail.last_seen <= ONLINE_AFTER : false
  const hi = detail?.host_info
  // Prefer the live SSE sample; fall back to the stored latest so top stats
  // are never blank after the detail API call returns.
  const live = sample ?? detail?.latest_sample ?? null
  const detailLoading = detail === null && !notFound

  // Derive a rough "load saturation" percentage from load1 / cpu_cores.
  const loadPercent =
    live && hi?.cpu_cores
      ? Math.min(100, (live.load.load1 / hi.cpu_cores) * 100)
      : undefined

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
        <RingGauge
          label="CPU"
          value={live ? formatPercent(live.cpu.used_percent) : "—"}
          sub={hi ? `${hi.cpu_cores} cores` : undefined}
          percent={live?.cpu.used_percent}
          points={points}
          accessor={(p) => p.cpu}
          color="var(--chart-1)"
          loading={detailLoading}
        />
        <RingGauge
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
          loading={detailLoading}
        />
        <RingGauge
          label="Disk"
          value={live?.disk_cap ? formatBytes(live.disk_cap.used_bytes) : "—"}
          sub={
            live?.disk_cap
              ? `of ${formatBytes(live.disk_cap.total_bytes)}`
              : undefined
          }
          percent={
            live?.disk_cap && live.disk_cap.total_bytes > 0
              ? (live.disk_cap.used_bytes / live.disk_cap.total_bytes) * 100
              : undefined
          }
          points={points}
          accessor={(p) => p.disk_used ?? 0}
          color="var(--chart-3)"
          loading={detailLoading}
        />
        <RingGauge
          label="Load"
          value={live ? live.load.load1.toFixed(2) : "—"}
          sub={
            live
              ? `5m ${live.load.load5.toFixed(2)} · 15m ${live.load.load15.toFixed(2)}`
              : undefined
          }
          percent={loadPercent}
          points={points}
          accessor={(p) => p.load1}
          color="var(--chart-4)"
          loading={detailLoading}
        />
      </div>

      {live ? (
        <LastUpdated ts={live.ts} />
      ) : (
        <p className="text-xs text-muted-foreground">
          {detail
            ? "Waiting for the agent's first report (arrives within ~15s)"
            : "Loading…"}
        </p>
      )}

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
          <CardDescription>
            Total utilization % · {resolution || "…"}
          </CardDescription>
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
              {
                key: "l1",
                name: "1m",
                color: "var(--chart-1)",
                value: (p) => p.load1,
              },
              {
                key: "l5",
                name: "5m",
                color: "var(--chart-2)",
                value: (p) => p.load5,
              },
              {
                key: "l15",
                name: "15m",
                color: "var(--chart-3)",
                value: (p) => p.load15,
              },
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
              {
                key: "rx",
                name: "rx",
                color: "var(--chart-2)",
                value: (p) => p.net_rx_rate,
              },
              {
                key: "tx",
                name: "tx",
                color: "var(--chart-3)",
                value: (p) => p.net_tx_rate,
              },
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
              {
                key: "dr",
                name: "read",
                color: "var(--chart-2)",
                value: (p) => p.disk_read_rate,
              },
              {
                key: "dw",
                name: "write",
                color: "var(--chart-3)",
                value: (p) => p.disk_write_rate,
              },
            ]}
          />
        </CardContent>
      </Card>
    </div>
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
