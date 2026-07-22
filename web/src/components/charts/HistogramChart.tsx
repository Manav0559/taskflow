import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { formatSeconds, toNonCumulative, type Bucket } from '@/lib/metrics'
import { useTheme } from '@/lib/theme'

export function HistogramChart({
  buckets,
  color,
  unit = 'seconds',
}: {
  buckets: Bucket[]
  color: string
  unit?: 'seconds'
}) {
  const { colors } = useTheme()
  const data = toNonCumulative(buckets)
    .filter((b) => Number.isFinite(b.le))
    .map((b) => ({ bucket: unit === 'seconds' ? formatSeconds(b.le) : String(b.le), count: b.count }))

  if (data.length === 0) {
    return <div className="flex h-40 items-center justify-center text-sm text-ink-muted">No data yet</div>
  }

  return (
    <ResponsiveContainer width="100%" height={160}>
      <BarChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -20 }}>
        <CartesianGrid strokeDasharray="3 3" stroke={colors.border} vertical={false} />
        <XAxis
          dataKey="bucket"
          tick={{ fontSize: 11, fill: colors.inkMuted }}
          axisLine={{ stroke: colors.hairline }}
          tickLine={false}
        />
        <YAxis
          tick={{ fontSize: 11, fill: colors.inkMuted }}
          axisLine={false}
          tickLine={false}
          width={36}
          allowDecimals={false}
        />
        <Tooltip
          contentStyle={{
            background: colors.card,
            border: `1px solid ${colors.border}`,
            borderRadius: 8,
            fontSize: 12,
          }}
          labelStyle={{ color: colors.ink }}
          cursor={{ fill: colors.border, opacity: 0.3 }}
        />
        <Bar dataKey="count" fill={color} radius={[4, 4, 0, 0]} maxBarSize={36} />
      </BarChart>
    </ResponsiveContainer>
  )
}
