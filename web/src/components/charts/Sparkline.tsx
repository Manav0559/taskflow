import { Area, AreaChart, ResponsiveContainer, Tooltip, YAxis } from 'recharts'
import { useTheme } from '@/lib/theme'

export interface SparklinePoint {
  t: number
  value: number
}

export function Sparkline({
  data,
  color,
  gradientId,
}: {
  data: SparklinePoint[]
  color: string
  gradientId: string
}) {
  const { colors } = useTheme()

  if (data.length < 2) {
    return <div className="flex h-16 items-center text-xs text-ink-muted">Collecting data…</div>
  }

  return (
    <ResponsiveContainer width="100%" height={64}>
      <AreaChart data={data} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.25} />
            <stop offset="100%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <YAxis hide domain={['dataMin', 'dataMax']} />
        <Tooltip
          contentStyle={{
            background: colors.card,
            border: `1px solid ${colors.border}`,
            borderRadius: 8,
            fontSize: 12,
          }}
          labelFormatter={() => ''}
        />
        <Area type="monotone" dataKey="value" stroke={color} strokeWidth={2} fill={`url(#${gradientId})`} />
      </AreaChart>
    </ResponsiveContainer>
  )
}
