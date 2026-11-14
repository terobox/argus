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

interface MetricChartProps {
  points: SeriesPoint[]
  accessor: (p: SeriesPoint) => number
  name: string
  color?: string
  format?: (v: number) => string
  height?: number
}

// MetricChart draws a single filled-area series (e.g. CPU or memory used).
export function MetricChart({
  points,
  accessor,
  name,
  color = "var(--chart-1)",
  format,
  height = 180,
}: MetricChartProps) {
  const data = points.map((p) => ({ ts: p.ts, value: accessor(p) }))
  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={`grad-${name}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={color} stopOpacity={0.25} />
            <stop offset="95%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
        <XAxis
          dataKey="ts"
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

interface MultiLineChartProps {
  points: SeriesPoint[]
  series: Series[]
  format?: (v: number) => string
  height?: number
}

// MultiLineChart draws several line series sharing one time axis (e.g. load
// averages or network rx/tx).
export function MultiLineChart({
  points,
  series,
  format,
  height = 180,
}: MultiLineChartProps) {
  const data = points.map((p) => {
    const row: Record<string, number> = { ts: p.ts }
    for (const s of series) row[s.key] = s.value(p)
    return row
  })
  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
        <XAxis
          dataKey="ts"
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
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
