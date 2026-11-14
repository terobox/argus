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
import { MetricChart, MultiLineChart } from "@/components/chart"
import { api, type NodeDetail, type SeriesPoint } from "@/lib/api"
import { useSampleStream } from "@/lib/sse"
import { formatBytes, formatPercent, formatRate, timeAgo } from "@/lib/format"
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
            {detail ? timeAgo(detail.last_seen) : "…"} ·{" "}
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

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Stat
          label="CPU"
          value={sample ? formatPercent(sample.cpu.used_percent) : "—"}
        />
        <Stat
          label="Load (1m)"
          value={sample ? sample.load.load1.toFixed(2) : "—"}
        />
        <Stat
          label="Memory used"
          value={sample ? formatBytes(sample.mem.used_bytes) : "—"}
        />
        <Stat label="Resolution" value={resolution || "—"} />
      </div>

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
          <CardDescription>Total utilization %</CardDescription>
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

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="py-4">
        <div className="text-xs text-muted-foreground">{label}</div>
        <div className="mt-1 text-lg font-semibold tabular-nums">{value}</div>
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
