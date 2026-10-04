const W = 520
const H = 210
const M = { top: 10, right: 12, bottom: 30, left: 52 }

export interface LineSeries {
  label: string
  color: string
  points: { x: number; y: number }[]
}

export interface BarSeries {
  label: string
  color: string
  values: number[]
}

function niceMax(v: number): number {
  if (v <= 0) return 1
  const step = 10 ** Math.floor(Math.log10(v))
  for (const m of [1, 2, 2.5, 5, 10]) if (m * step >= v) return m * step
  return 10 * step
}

const ticks = (max: number, n = 4) => Array.from({ length: n + 1 }, (_, i) => (max * i) / n)

function Axes({ yMax, yFormat, xTicks, xLabel }: { yMax: number; yFormat: (v: number) => string; xTicks: { at: number; label: string }[]; xLabel: string }) {
  const h = H - M.top - M.bottom
  return (
    <g fontFamily="IBM Plex Mono, ui-monospace, monospace" fontSize="10" fill="var(--muted-foreground)">
      {ticks(yMax).map(t => {
        const y = M.top + h - (t / yMax) * h
        return (
          <g key={t}>
            <line x1={M.left} x2={W - M.right} y1={y} y2={y} stroke="var(--border)" />
            <text x={M.left - 6} y={y + 3} textAnchor="end">{yFormat(t)}</text>
          </g>
        )
      })}
      {xTicks.map(t => <text key={t.label + t.at} x={t.at} y={H - M.bottom + 14} textAnchor="middle">{t.label}</text>)}
      <text x={(M.left + W - M.right) / 2} y={H - 2} textAnchor="middle" fill="var(--dim)">{xLabel}</text>
    </g>
  )
}

function Legend({ items }: { items: { label: string; color: string }[] }) {
  return (
    <div className="mt-1 flex flex-wrap gap-3 text-[11.5px] text-muted-foreground">
      {items.map(i => <span key={i.label} className="flex items-center gap-1.5"><span className="size-2.5" style={{ background: i.color }} />{i.label}</span>)}
    </div>
  )
}

export function LineChart({ series, xLabel, yFormat, label }: { series: LineSeries[]; xLabel: string; yFormat: (v: number) => string; label: string }) {
  const xMax = niceMax(Math.max(1, ...series.flatMap(s => s.points.map(p => p.x))))
  const yMax = niceMax(Math.max(0, ...series.flatMap(s => s.points.map(p => p.y))))
  const w = W - M.left - M.right
  const h = H - M.top - M.bottom
  const sx = (x: number) => M.left + (x / xMax) * w
  const sy = (y: number) => M.top + h - (y / yMax) * h
  return (
    <figure className="m-0">
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" role="img" aria-label={label}>
        <Axes yMax={yMax} yFormat={yFormat} xLabel={xLabel} xTicks={ticks(xMax).map(t => ({ at: sx(t), label: String(Math.round(t)) }))} />
        {series.map(s => (
          <g key={s.label}>
            <polyline fill="none" stroke={s.color} strokeWidth="1.75" points={s.points.map(p => `${sx(p.x)},${sy(p.y)}`).join(' ')} />
            {s.points.map((p, i) => <circle key={i} cx={sx(p.x)} cy={sy(p.y)} r="2.5" fill={s.color}><title>{`${s.label}: ${yFormat(p.y)} at ${Math.round(p.x)} ${xLabel}`}</title></circle>)}
          </g>
        ))}
      </svg>
      <Legend items={series} />
    </figure>
  )
}

export function BarChart({ series, xLabel, yFormat, label }: { series: BarSeries[]; xLabel: string; yFormat: (v: number) => string; label: string }) {
  const count = Math.max(1, ...series.map(s => s.values.length))
  const yMax = niceMax(Math.max(0, ...series.flatMap(s => s.values)))
  const w = W - M.left - M.right
  const h = H - M.top - M.bottom
  const slot = w / count
  const bar = Math.max(1, (slot * 0.8) / series.length)
  const every = Math.ceil(count / 14)
  return (
    <figure className="m-0">
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" role="img" aria-label={label}>
        <Axes
          yMax={yMax}
          yFormat={yFormat}
          xLabel={xLabel}
          xTicks={Array.from({ length: count }, (_, i) => i).filter(i => i % every === 0).map(i => ({ at: M.left + slot * i + slot / 2, label: String(i + 1) }))}
        />
        {series.map((s, si) => s.values.map((v, i) => {
          const bh = (v / yMax) * h
          return (
            <rect key={`${s.label}-${i}`} x={M.left + slot * i + slot * 0.1 + bar * si} y={M.top + h - bh} width={bar} height={Math.max(0, bh)} fill={s.color}>
              <title>{`${s.label} · ${i + 1}: ${yFormat(v)}`}</title>
            </rect>
          )
        }))}
      </svg>
      <Legend items={series} />
    </figure>
  )
}
