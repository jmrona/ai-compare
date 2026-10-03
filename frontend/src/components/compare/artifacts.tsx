// Technical views of one side: editor-style tabs, logs, diff, metrics, tests and events.
// Used both in the live run and in the history detail.

import type { ReactNode } from 'react'
import { useMemo } from 'react'
import { Tabs as TabsPrimitive } from 'radix-ui'
import { cn } from '@/lib/utils'
import { api } from '@/api/client'
import type { SideKey, SideRun, TerminalSource } from '@/api/types'
import { useDiff, useLogs, useTestOutput, useTimeline } from '@/api/queries'
import { formatDateTime, formatDuration, formatInt, formatUsd } from '@/lib/format'
import { TerminalView } from '@/components/terminal/TerminalView'
import { ErrorNote, Metric } from '@/components/common/primitives'

// Grows to fill the pane when its parent has a height (live run), otherwise stays at 380px.
export const PANE_HEIGHT = 'min-h-[380px] flex-1'

export interface PaneTab {
  value: string
  label: string
  tag?: string
  disabled?: boolean
  content?: ReactNode
}

/** Editor-style tabs: the active tab gets a top border in the side colour. */
export function PaneTabs({ side, tabs, value, onChange }: { side: SideKey; tabs: PaneTab[]; value: string; onChange: (v: string) => void }) {
  return (
    <TabsPrimitive.Root value={value} onValueChange={onChange} className="flex min-h-0 min-w-0 flex-1 flex-col">
      <TabsPrimitive.List aria-label={`Side ${side} artefacts`} className="flex overflow-x-auto border-y bg-background">
        {tabs.map(t => (
          <TabsPrimitive.Trigger
            key={t.value}
            value={t.value}
            disabled={t.disabled}
            className={cn(
              'border-r px-3.5 py-1.5 text-[12.5px] whitespace-nowrap text-muted-foreground outline-none hover:text-foreground focus-visible:bg-raise disabled:opacity-40',
              'data-[state=active]:bg-term data-[state=active]:text-foreground',
              side === 'A' ? 'data-[state=active]:shadow-[inset_0_2px_0_var(--side-a)]' : 'data-[state=active]:shadow-[inset_0_2px_0_var(--side-b)]',
            )}
          >
            {t.label}
            {t.tag && <span className="ml-1.5 text-[10px] tracking-wide text-dim uppercase">{t.tag}</span>}
          </TabsPrimitive.Trigger>
        ))}
      </TabsPrimitive.List>
      {tabs.filter(t => t.content).map(t => (
        <TabsPrimitive.Content key={t.value} value={t.value} className="flex min-h-0 flex-1 flex-col outline-none">
          {t.content}
        </TabsPrimitive.Content>
      ))}
    </TabsPrimitive.Root>
  )
}

export function SideTerminal({ id, side, readOnly }: { id: string; side: SideKey; readOnly?: boolean }) {
  // One source per mount: coming back to the tab reconnects and receives the accumulated output.
  const source = useMemo(() => api.openTerminal(id, side), [id, side])
  return <TerminalView source={source} readOnly={readOnly} className={PANE_HEIGHT} />
}

export function LogsView({ id, side }: { id: string; side: SideKey }) {
  const { data, error } = useLogs(id, side)
  if (error) return <div className="p-3"><ErrorNote error={error} /></div>
  return (
    <div className={cn('overflow-auto bg-term font-mono text-xs', PANE_HEIGHT)}>
      {data?.map((r, i) => (
        <div key={i} className="flex gap-3 border-b border-border/60 px-3.5 py-1.5">
          <span className="tnum shrink-0 text-dim">{r.at}</span>
          <span className={cn('w-12 shrink-0', r.level === 'warn' ? 'text-warn' : r.level === 'error' ? 'text-danger' : 'text-muted-foreground')}>{r.source}</span>
          <span className={cn(r.level === 'warn' && 'text-warn', r.level === 'error' && 'text-danger')}>{r.message}</span>
        </div>
      ))}
    </div>
  )
}

export function DiffView({ id, side, live }: { id: string; side: SideKey; live?: boolean }) {
  const { data, error, dataUpdatedAt, refetch, isFetching } = useDiff(id, side)
  if (error) return <div className="p-3"><ErrorNote error={error} /></div>
  return (
    <div className={cn('flex flex-col bg-term', PANE_HEIGHT)}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b px-3.5 py-2 font-mono text-xs">
        {data?.files.map(f => (
          <span key={f.path} className="text-muted-foreground">
            {f.path} <span className="text-ok">+{f.added}</span> <span className="text-danger">−{f.removed}</span>
          </span>
        ))}
        {live && (
          <button onClick={() => refetch()} className="ml-auto text-dim hover:text-foreground" disabled={isFetching}>
            {isFetching ? 'refreshing…' : `snapshot ${dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString('en-GB') : ''} · refresh`}
          </button>
        )}
      </div>
      <pre className="min-h-0 flex-1 overflow-auto py-2 font-mono text-xs leading-[1.6]">
        {data?.lines.map((l, i) => (
          <div key={i} className={cn('px-3.5', l.kind === '+' && 'bg-ok/10 text-ok', l.kind === '-' && 'bg-danger/10 text-danger', l.kind === '@@' && 'text-dim')}>
            {l.text}
          </div>
        ))}
      </pre>
    </div>
  )
}

export function MetricsView({ run }: { run: SideRun }) {
  const m = run.metrics
  const price = run.priceSnapshot.price
  const rows: [string, number | null, number | null][] = [
    ['input', m.usage.input, price?.input ?? null],
    ['cache read', m.usage.cacheRead, price?.cacheRead ?? null],
    ['cache write', m.usage.cacheWrite, price?.cacheWrite ?? null],
    ['output', m.usage.output, price?.output ?? null],
  ]
  return (
    <div className={cn('overflow-auto bg-term p-3.5', PANE_HEIGHT)}>
      <table className="w-full font-mono text-[12.5px]">
        <thead>
          <tr className="text-left text-[10.5px] tracking-[0.07em] text-dim uppercase">
            <th className="pb-2 font-normal">category</th>
            <th className="pb-2 text-right font-normal">tokens</th>
            <th className="pb-2 text-right font-normal">cost</th>
          </tr>
        </thead>
        <tbody className="tnum">
          {rows.map(([label, tokens, rate]) => (
            <tr key={label} className="border-t border-border/60">
              <td className="py-1.5">{label}</td>
              <td className="py-1.5 text-right">{tokens == null ? '—' : formatInt(tokens)}</td>
              <td className="py-1.5 text-right text-muted-foreground">
                {tokens == null ? 'not charged' : rate == null ? '—' : formatUsd((tokens * rate) / 1_000_000)}
              </td>
            </tr>
          ))}
          <tr className="border-t">
            <td className="py-1.5">total</td>
            <td className="py-1.5 text-right">{formatInt(m.usage.input + m.usage.cacheRead + (m.usage.cacheWrite ?? 0) + m.usage.output)}</td>
            <td className="py-1.5 text-right">{formatUsd(m.costUsd)} est.</td>
          </tr>
        </tbody>
      </table>
      <div className="mt-4 grid grid-cols-3 gap-3 border-t pt-3">
        <Metric label="preparation" value={formatDuration(m.prepSec)} sub="not counted" />
        <Metric label="agent" value={formatDuration(m.agentSec)} />
        <Metric label="human wait" value={formatDuration(m.humanWaitSec)} sub={run.config.mode === 'interactive' ? 'estimated' : 'autonomous'} />
        <Metric label="requests" value={m.requests} />
        <Metric label="retries" value={m.retries} />
        <Metric label="tok/s" value={m.tokensPerSec ?? '—'} />
      </div>
      <p className="mt-3 text-xs text-dim">
        {price
          ? `Price: models.dev · snapshot from ${formatDateTime(run.priceSnapshot.fetchedAt)}`
          : 'models.dev has no price for this model, so the cost cannot be calculated.'}
      </p>
    </div>
  )
}

const staticSource = (text: string): TerminalSource => ({
  subscribe(onData) {
    onData(text)
    return () => {}
  },
  send() {},
  resize() {},
})

export function TestsView({ id, side }: { id: string; side: SideKey }) {
  const { data } = useTestOutput(id, side)
  const source = useMemo(() => (data ? staticSource(data.join('\r\n')) : null), [data])
  if (!source) return <div className={cn('bg-term', PANE_HEIGHT)} />
  return <TerminalView source={source} readOnly className={PANE_HEIGHT} />
}

export function TimelineView({ id, side }: { id: string; side: SideKey }) {
  const { data } = useTimeline(id, side)
  return (
    <div className={cn('overflow-auto bg-term px-3.5 py-3 font-mono text-[12.5px]', PANE_HEIGHT)}>
      {data?.map(e => (
        <div key={e.at + e.kind} className="flex gap-4 border-b border-border/60 py-1.5">
          <span className="tnum text-dim">{e.at}</span>
          <span className="w-20 text-muted-foreground">{e.kind}</span>
          <span>{e.detail}</span>
        </div>
      ))}
    </div>
  )
}
