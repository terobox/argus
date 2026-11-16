import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"
import type { SeriesPoint } from "@/lib/api"
import { formatDateTime } from "@/lib/format"

// ChartRow is one plotted row: the timestamp plus one numeric value per series.
// A row whose values are null marks a break: Recharts stops the line there
// instead of connecting across it.
type ChartRow = { ts: number } & Record<string, number | null>

// buildRows converts stored points to chart rows. When two consecutive samples
// are farther apart than gapSeconds (the agent was stopped or the server was
// unreachable), a null row is inserted at the gap's start so the line breaks
// instead of drawing a straight segment across the outage.
function buildRows(
  points: SeriesPoint[],
  gapSeconds: number | undefined,
  build: (p: SeriesPoint) => ChartRow
): ChartRow[] {
  const rows: ChartRow[] = []
  let prevTs: number | null = null
  for (const p of points) {
    const row = build(p)
    if (gapSeconds && prevTs !== null && p.ts - prevTs > gapSeconds) {
      const blank: ChartRow = { ts: prevTs + gapSeconds }
      for (const key of Object.keys(row)) {
        if (key !== "ts") blank[key] = null
      }
      rows.push(blank)
    }
    rows.push(row)
    prevTs = p.ts
  }
  return rows
}

interface MetricChartProps {
  points: SeriesPoint[]
  accessor: (p: SeriesPoint) => number
  name: string
  color?: string
  format?: (v: number) => string
  height?: number
  // domain pins the x-axis to an absolute window (unix seconds). Without it the
  // axis spans just the returned data, which hides how stale the chart is.
  domain?: [number, number]
  // gapSeconds is the smallest sample spacing that counts as an outage.
  gapSeconds?: number
}

// MetricChart draws a single filled-area series (e.g. CPU or memory used).
export function MetricChart({
  points,
  accessor,
  name,
  color = "var(--chart-1)",
  format,
  height = 180,
  domain,
  gapSeconds,
}: MetricChartProps) {
  const data = buildRows(points, gapSeconds, (p) => ({
    ts: p.ts,
    value: accessor(p),
  }))
  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={`grad-${name}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={color} stopOpacity={0.25} />
            <stop offset="95%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid
          strokeDasharray="3 3"
          stroke="var(--border)"
          vertical={false}
        />
        <XAxis
          dataKey="ts"
          type="number"
          domain={domain}
          allowDataOverflow
          tickFormatter={(ts) => formatDateTime(Number(ts))}
          stroke="var(--muted-foreground)"
          fontSize={11}
          minTickGap={48}
        />
        <YAxis
          stroke="var(--muted-foreground)"
          fontSize={11}
          width={48}
          tickFormatter={(v) => (format ? format(Number(v)) : String(v))}
        />
        <Tooltip
          contentStyle={{
            background: "var(--popover)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            fontSize: 12,
          }}
          labelFormatter={(ts) => formatDateTime(Number(ts))}
          formatter={(value) => [
            format ? format(Number(value)) : String(value),
            name,
          ]}
        />
        <Area
          type="monotone"
          dataKey="value"
          stroke={color}
          strokeWidth={1.5}
          fill={`url(#grad-${name})`}
          connectNulls={false}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}

export interface Series {
  key: string
  name: string
  color: string
  value: (p: SeriesPoint) => number
}

interface SparklineProps {
  points: SeriesPoint[]
  accessor: (p: SeriesPoint) => number
  color?: string
  height?: number
  gapSeconds?: number
}

// Sparkline is a tiny, axis-free area chart used inside compact stat panels to
// show a metric's recent trend without taking over the layout.
export function Sparkline({
  points,
  accessor,
  color = "var(--chart-1)",
  height = 36,
  gapSeconds,
}: SparklineProps) {
  const data = buildRows(points, gapSeconds, (p) => ({
    ts: p.ts,
    v: accessor(p),
  }))
  const id = `spark-${color.replace(/[^a-zA-Z0-9]/g, "")}`
  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={data} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={color} stopOpacity={0.25} />
            <stop offset="95%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <Area
          type="monotone"
          dataKey="v"
          stroke={color}
          strokeWidth={1.25}
          fill={`url(#${id})`}
          connectNulls={false}
          dot={false}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}

interface MultiLineChartProps {
  points: SeriesPoint[]
  series: Series[]
  format?: (v: number) => string
  height?: number
  domain?: [number, number]
  gapSeconds?: number
}

// MultiLineChart draws several line series sharing one time axis (e.g. load
// averages or network rx/tx).
export function MultiLineChart({
  points,
  series,
  format,
  height = 180,
  domain,
  gapSeconds,
}: MultiLineChartProps) {
  const keys = series.map((s) => s.key)
  const data = buildRows(points, gapSeconds, (p) => {
    const row: ChartRow = { ts: p.ts }
    for (let i = 0; i < series.length; i++) row[keys[i]] = series[i].value(p)
    return row
  })
  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid
          strokeDasharray="3 3"
          stroke="var(--border)"
          vertical={false}
        />
        <XAxis
          dataKey="ts"
          type="number"
          domain={domain}
          allowDataOverflow
          tickFormatter={(ts) => formatDateTime(Number(ts))}
          stroke="var(--muted-foreground)"
          fontSize={11}
          minTickGap={48}
        />
        <YAxis
          stroke="var(--muted-foreground)"
          fontSize={11}
          width={48}
          tickFormatter={(v) => (format ? format(Number(v)) : String(v))}
        />
        <Tooltip
          contentStyle={{
            background: "var(--popover)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            fontSize: 12,
          }}
          labelFormatter={(ts) => formatDateTime(Number(ts))}
          formatter={(value, name) => [
            format ? format(Number(value)) : String(value),
            String(name),
          ]}
        />
        {series.map((s) => (
          <Line
            key={s.key}
            type="monotone"
            dataKey={s.key}
            name={s.name}
            stroke={s.color}
            strokeWidth={1.5}
            dot={false}
            connectNulls={false}
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
