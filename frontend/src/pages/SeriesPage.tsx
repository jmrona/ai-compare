// A series: the same comparison run several times (repetitions). One run per side varies a lot,
// so this page aggregates the attempts per side and plots cost against quality.

import { useMemo } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { Square } from 'lucide-react'
import { clients } from '@/api/transport'
import { isLive, useSeries } from '@/api/queries'
import type { Comparison, SideKey, SideRun } from '@/api/types'
import { STATUS_LABEL, formatDuration, formatTokens, formatUsd, harnessLabel, modeLabel } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, LoadingRows, Panel, SideTag } from '@/components/common/primitives'

/** Share of test runs that passed for one side of one attempt (visible and hidden); null without tests. */
export function quality(run: SideRun): number | null {
  const runs = [run.tests.visible, run.tests.hidden].filter(r => r != null)
  if (runs.length === 0) return null
  return runs.filter(r => r.status === 'passed').length / runs.length
}

const SIDES = ['A', 'B'] as const

const tokens = (r: SideRun) => r.metrics.usage.input + r.metrics.usage.cacheRead + (r.metrics.usage.cacheWrite ?? 0) + r.metrics.usage.output

interface Stat { n: number; mean: number; sd: number; min: number; max: number }

function stat(values: (number | null)[]): Stat | null {
  const v = values.filter((x): x is number => x != null)
  if (v.length === 0) return null
  const mean = v.reduce((a, b) => a + b, 0) / v.length
  const sd = Math.sqrt(v.reduce((a, b) => a + (b - mean) ** 2, 0) / v.length)
  return { n: v.length, mean, sd, min: Math.min(...v), max: Math.max(...v) }
}

export function SeriesPage() {
  const { id } = useParams({ from: '/series/$id' })
  const { data: attempts, error, isLoading } = useSeries(id)
  const stop = useMutation({ mutationFn: () => clients.comparisons.stopSeries({ seriesId: id }) })

  if (isLoading) return <div className="p-4"><LoadingRows rows={6} /></div>
  if (error || !attempts || attempts.length === 0) {
    return (
      <>
        <TopBar crumbs={[{ label: 'History', to: '/history' }, { label: `Series ${id}` }]} />
        <div className="p-4">{error ? <ErrorNote error={error} /> : <p className="text-muted-foreground">No comparisons in this series.</p>}</div>
      </>
    )
  }
  return <Series id={id} attempts={attempts} onStop={() => stop.mutate()} stopping={stop.isPending} />
}

function Series({ id, attempts, onStop, stopping }: { id: string; attempts: Comparison[]; onStop: () => void; stopping: boolean }) {
  const first = attempts[0]
  const size = first.seriesSize
  const ended = attempts.filter(c => !isLive(c))
  const stopped = attempts.some(c => c.seriesStopped)
  const pending = !stopped && attempts.length < size
  const sides = (['A', 'B'] as const)

  const stats = useMemo(() => Object.fromEntries(SIDES.map(k => {
    const runs = ended.map(c => c.sides[k])
    return [k, {
      cost: stat(runs.map(r => r.metrics.costUsd)),
      agent: stat(runs.map(r => r.metrics.agentSec)),
      tokens: stat(runs.map(tokens)),
      files: stat(runs.map(r => r.files.length)),
      quality: stat(runs.map(quality)),
      finished: runs.filter(r => r.status === 'finished').length,
    }]
  })) as Record<SideKey, Record<'cost' | 'agent' | 'tokens' | 'files' | 'quality', Stat | null> & { finished: number }>, [ended])

  return (
    <>
      <TopBar crumbs={[{ label: 'History', to: '/history' }, { label: `Series ${id}` }]}>
        <Chip tone={pending ? 'warn' : 'ok'}>{attempts.length} of {size} attempts{stopped ? ' · stopped' : ''}</Chip>
        {pending && <Button size="sm" variant="outline" disabled={stopping} onClick={onStop}><Square className="size-3" />Stop the series</Button>}
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1160px] gap-6 px-4 py-6">
        <div>
          <div className="font-mono text-xs text-dim">{first.projectName} · {size} repetitions</div>
          <h1 className="mt-2 max-w-[70ch] text-[20px] leading-snug font-semibold">{first.prompt}</h1>
          <div className="mt-3 flex flex-wrap gap-4 text-[12.5px]">
            {sides.map(k => {
              const cfg = first.sides[k].config
              return (
                <span key={k} className="flex flex-wrap items-center gap-1.5">
                  <SideTag side={k} small /><span className="font-mono">{cfg.model}</span>
                  <Chip>{cfg.effort || 'default effort'}</Chip><Chip>{modeLabel(cfg.mode)}</Chip><Chip>{harnessLabel(cfg.harness)}</Chip>
                </span>
              )
            })}
          </div>
        </div>

        <Panel title={`Aggregates · ${ended.length} ended ${ended.length === 1 ? 'attempt' : 'attempts'}`} bodyClassName="p-0">
          <table className="w-full font-mono text-[12.5px]">
            <thead>
              <tr className="text-left text-[10.5px] tracking-[0.07em] text-dim uppercase">
                <th className="px-3 py-2 font-normal">measure</th>
                {sides.map(k => <th key={k} className="px-3 py-2 text-right font-normal"><SideTag side={k} small /></th>)}
              </tr>
            </thead>
            <tbody className="tnum">
              <StatRow label="cost" a={stats.A.cost} b={stats.B.cost} format={formatUsd} lowerWins />
              <StatRow label="agent time" a={stats.A.agent} b={stats.B.agent} format={formatDuration} lowerWins />
              <StatRow label="tokens" a={stats.A.tokens} b={stats.B.tokens} format={formatTokens} lowerWins />
              <StatRow label="tests passed" a={stats.A.quality} b={stats.B.quality} format={v => `${Math.round(v * 100)}%`} />
              <StatRow label="files changed" a={stats.A.files} b={stats.B.files} format={v => v.toFixed(1)} />
              <tr className="border-t border-border/60">
                <td className="px-3 py-1.5">finished</td>
                {sides.map(k => <td key={k} className="px-3 py-1.5 text-right">{stats[k].finished} / {ended.length}</td>)}
              </tr>
            </tbody>
          </table>
          <p className="border-t px-3 py-2 text-xs text-dim">Mean ± standard deviation, with the range below. "Tests passed" is the share of test runs (visible and hidden) that passed.</p>
        </Panel>

        <Panel title="Cost versus quality">
          <CostQualityChart attempts={ended} />
        </Panel>

        <Panel title="Attempts" bodyClassName="p-0">
          <table className="w-full text-[12.5px]">
            <thead>
              <tr className="text-left text-[10.5px] tracking-[0.07em] text-dim uppercase">
                <th className="px-3 py-2 font-normal">#</th>
                {sides.map(k => <th key={k} className="px-3 py-2 font-normal"><SideTag side={k} small /> status · cost · time · tests</th>)}
                <th className="px-3 py-2 font-normal" />
              </tr>
            </thead>
            <tbody>
              {attempts.map(c => (
                <tr key={c.id} className="border-t border-border/60">
                  <td className="px-3 py-1.5 font-mono">{c.attempt}</td>
                  {sides.map(k => {
                    const r = c.sides[k]
                    const q = quality(r)
                    return (
                      <td key={k} className="px-3 py-1.5 font-mono">
                        <span className={cn(r.status === 'error' && 'text-danger')}>{STATUS_LABEL[r.status]}</span>
                        <span className="text-dim"> · </span>{formatUsd(r.metrics.costUsd)}
                        <span className="text-dim"> · </span>{formatDuration(r.metrics.agentSec)}
                        <span className="text-dim"> · </span>{q == null ? '—' : `${Math.round(q * 100)}%`}
                      </td>
                    )
                  })}
                  <td className="px-3 py-1.5 text-right">
                    <Link to={isLive(c) ? '/comparisons/$id' : '/history/$id'} params={{ id: c.id }} className="text-xs text-side-a hover:underline">#{c.id}</Link>
                  </td>
                </tr>
              ))}
              {Array.from({ length: Math.max(0, size - attempts.length) }, (_, i) => (
                <tr key={`pending-${i}`} className="border-t border-border/60 text-dim">
                  <td className="px-3 py-1.5 font-mono">{attempts.length + i + 1}</td>
                  <td className="px-3 py-1.5" colSpan={3}>{stopped ? 'will not run: the series was stopped' : 'waiting for the previous attempt'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
      </div>
    </>
  )
}

function StatRow({ label, a, b, format, lowerWins }: { label: string; a: Stat | null; b: Stat | null; format: (v: number) => string; lowerWins?: boolean }) {
  const best = a && b && a.mean !== b.mean && lowerWins != null ? ((a.mean < b.mean) === !!lowerWins ? 'A' : 'B') : null
  const cell = (s: Stat | null, k: SideKey) => (
    <td className="px-3 py-1.5 text-right align-top">
      {s ? (
        <>
          <span className={cn(best === k && 'text-ok')}>{format(s.mean)}</span>
          {s.n > 1 && <span className="text-dim"> ± {format(s.sd)}</span>}
          {s.n > 1 && <div className="text-[11px] text-dim">{format(s.min)} – {format(s.max)}</div>}
        </>
      ) : '—'}
    </td>
  )
  return (
    <tr className="border-t border-border/60">
      <td className="px-3 py-1.5 align-top">{label}</td>
      {cell(a, 'A')}
      {cell(b, 'B')}
    </tr>
  )
}

/** One point per side and attempt: x = cost, y = share of test runs passed. */
function CostQualityChart({ attempts }: { attempts: Comparison[] }) {
  const points = attempts.flatMap(c => (['A', 'B'] as const).map(k => ({ side: k, attempt: c.attempt, cost: c.sides[k].metrics.costUsd, q: quality(c.sides[k]) })))
    .filter(p => p.cost != null && p.q != null) as { side: SideKey; attempt: number; cost: number; q: number }[]
  if (points.length === 0) {
    return <p className="text-[13px] text-muted-foreground">No point to plot yet: it needs ended attempts with a cost and test results (set a test command in the project profile).</p>
  }
  const W = 640, H = 260, P = 40
  const maxCost = Math.max(...points.map(p => p.cost)) * 1.15 || 1
  const x = (v: number) => P + (v / maxCost) * (W - 2 * P)
  const y = (v: number) => H - P - v * (H - 2 * P)
  // Several attempts can land on the same spot; shift them slightly so each stays visible.
  const seen = new Map<string, number>()
  return (
    <div className="overflow-x-auto">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full max-w-[720px]" role="img" aria-label="Cost versus share of tests passed, per attempt">
        <line x1={P} y1={H - P} x2={W - P} y2={H - P} className="stroke-border" />
        <line x1={P} y1={P} x2={P} y2={H - P} className="stroke-border" />
        {[0, 0.5, 1].map(v => (
          <g key={v}>
            <line x1={P} x2={W - P} y1={y(v)} y2={y(v)} className="stroke-border/40" strokeDasharray="2 4" />
            <text x={P - 6} y={y(v) + 4} textAnchor="end" className="fill-dim text-[10px]">{Math.round(v * 100)}%</text>
          </g>
        ))}
        {[0, 0.5, 1].map(f => (
          <text key={f} x={x(maxCost * f)} y={H - P + 16} textAnchor="middle" className="fill-dim text-[10px]">{formatUsd(maxCost * f)}</text>
        ))}
        <text x={W / 2} y={H - 6} textAnchor="middle" className="fill-dim text-[10px]">cost per attempt</text>
        {points.map(p => {
          const k = `${Math.round(x(p.cost))}:${Math.round(y(p.q))}`
          const n = seen.get(k) ?? 0
          seen.set(k, n + 1)
          return (
            <circle key={`${p.side}-${p.attempt}`} cx={x(p.cost) + n * 5} cy={y(p.q) - n * 5} r={5}
              className={p.side === 'A' ? 'fill-side-a' : 'fill-side-b'} opacity={0.85}>
              <title>{`Side ${p.side}, attempt ${p.attempt}: ${formatUsd(p.cost)}, ${Math.round(p.q * 100)}% of tests passed`}</title>
            </circle>
          )
        })}
      </svg>
      <div className="mt-2 flex gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5"><span className="inline-block size-2.5 rounded-full bg-side-a" />side A</span>
        <span className="flex items-center gap-1.5"><span className="inline-block size-2.5 rounded-full bg-side-b" />side B</span>
        <span className="text-dim">Top left is best: cheaper and more tests passed.</span>
      </div>
    </div>
  )
}
