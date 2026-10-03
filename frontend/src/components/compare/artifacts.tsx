// Technical views of one side: editor-style tabs, terminal, logs, changes, metrics, tests and
// events. Used both in the live run and in the history detail.

import type { ReactNode } from 'react'
import { useMemo, useState } from 'react'
import { Tabs as TabsPrimitive } from 'radix-ui'
import { Play } from 'lucide-react'
import { cn } from '@/lib/utils'
import { sideTerminal } from '@/api/http'
import type { SideKey, SideRun, TestRun, TerminalSource } from '@/api/types'
import { useLogs, useTestOutput, useTimeline } from '@/api/queries'
import { formatClock, formatDateTime, formatDuration, formatInt, formatRate, formatSeconds, formatUsd } from '@/lib/format'
import { TerminalView } from '@/components/terminal/TerminalView'
import { RecordingPlayer } from '@/components/terminal/RecordingPlayer'
import { DiffView as ChangesView } from './DiffView'
import { Button } from '@/components/ui/button'
import { Chip, ErrorNote, LoadingRows, Metric, Segmented } from '@/components/common/primitives'

// Fills the pane; the pane's container gives it a height, and each view scrolls on its own.
export const PANE_HEIGHT = 'min-h-0 flex-1'

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

/**
 * The side's terminal: live while it runs (the server replays the output so far on connect),
 * its final screen once it has ended, and a timed replay when a recording exists.
 */
export function SideTerminal({ id, run, readOnly }: { id: string; run: SideRun; readOnly?: boolean }) {
  const side = run.key
  const [replay, setReplay] = useState(false)
  // A new connection per mount (and when it becomes read-only).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const source = useMemo(() => sideTerminal(id, side), [id, side, readOnly])
  if (replay) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <RecordingPlayer id={id} side={side} className={PANE_HEIGHT} />
        <div className="border-t bg-term px-3 py-1.5">
          <Button size="sm" variant="ghost" onClick={() => setReplay(false)}>Back to the final screen</Button>
        </div>
      </div>
    )
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TerminalView source={source} readOnly={readOnly} className={PANE_HEIGHT} />
      {readOnly && run.hasRecording && (
        <div className="border-t bg-term px-3 py-1.5">
          <Button size="sm" variant="ghost" onClick={() => setReplay(true)}><Play className="size-3.5" />Replay with timing</Button>
        </div>
      )}
    </div>
  )
}

export function LogsView({ id, side }: { id: string; side: SideKey }) {
  const { data, error } = useLogs(id, side)
  if (error) return <div className="p-3"><ErrorNote error={error} /></div>
  return (
    <div className={cn('overflow-auto bg-term font-mono text-xs', PANE_HEIGHT)}>
      {data?.map((r, i) => (
        <div key={i} className="flex gap-3 border-b border-border/60 px-3.5 py-1.5">
          <span className="tnum shrink-0 text-dim">{formatClock(r.at)}</span>
          <span className={cn('w-12 shrink-0', r.level === 'warn' ? 'text-warn' : r.level === 'error' ? 'text-danger' : 'text-muted-foreground')}>{r.source}</span>
          <span className={cn('whitespace-pre-wrap', r.level === 'warn' && 'text-warn', r.level === 'error' && 'text-danger')}>{r.message}</span>
        </div>
      ))}
    </div>
  )
}

/** What the agent changed: see DiffView.tsx. */
export function DiffView(props: { id: string; run: SideRun; live?: boolean }) {
  return <ChangesView {...props} className={PANE_HEIGHT} />
}

export function MetricsView({ run }: { run: SideRun }) {
  const m = run.metrics
  const price = run.priceSnapshot.price
  const rows: [string, number | null, number | null][] = [
    ['input', m.usage.input, price?.input ?? null],
    ['cache read', m.usage.cacheRead, price?.cacheRead ?? price?.input ?? null],
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
                {tokens == null ? 'not reported' : rate == null ? '—' : formatUsd((tokens * rate) / 1_000_000)}
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
        <Metric label="human wait" value={run.config.mode === 'interactive' ? formatDuration(m.humanWaitSec) : '—'} sub={run.config.mode === 'interactive' ? 'estimated' : 'autonomous'} />
        <Metric label="requests" value={m.requests} />
        <Metric label="provider errors" value={m.errors} />
        <Metric label="tok/s" value={formatRate(m.tokensPerSec)} />
      </div>
      <div className="mt-3 grid grid-cols-4 gap-3 border-t pt-3">
        <Metric label="copy" value={formatSeconds(m.phases.copySec)} sub="shared by both sides" />
        <Metric label="image build" value={formatSeconds(m.phases.buildSec)} sub="sides build in parallel" />
        <Metric label="container start" value={formatSeconds(m.phases.startSec)} />
        <Metric label="verification" value={formatSeconds(m.phases.verifySec)} sub="diff and tests" />
      </div>
      <p className="mt-3 text-xs text-dim">
        {price
          ? `Price: models.dev · snapshot from ${formatDateTime(run.priceSnapshot.fetchedAt)}`
          : 'models.dev has no price for this model, so the cost cannot be calculated.'}
      </p>
    </div>
  )
}

const TEST_TONE: Record<TestRun['status'], 'ok' | 'danger' | 'warn'> = { passed: 'ok', failed: 'danger', error: 'warn' }

export function TestRunChip({ run, label }: { run: TestRun | null; label?: string }) {
  if (!run) return <Chip>{label ? `${label}: ` : ''}not run</Chip>
  return (
    <Chip tone={TEST_TONE[run.status]}>
      {label ? `${label}: ` : ''}{run.status}{run.status !== 'passed' && ` · exit ${run.exitCode}`}
    </Chip>
  )
}

const textSource = (text: string): TerminalSource => ({
  subscribe(onData) {
    onData(text.replace(/\r?\n/g, '\r\n'))
    return () => {}
  },
  send() {},
  resize() {},
})

/** The profile's tests, run in a fresh container from the side's result, and the hidden tests. */
export function TestsView({ id, run }: { id: string; run: SideRun }) {
  const { data, error } = useTestOutput(id, run.key)
  const [which, setWhich] = useState<'visible' | 'hidden'>('visible')
  const output = which === 'visible' ? data?.visibleOutput : data?.hiddenOutput
  const source = useMemo(() => (output ? textSource(output) : null), [output])
  const tests = data?.tests ?? run.tests
  if (error) return <div className="p-3"><ErrorNote error={error} /></div>
  return (
    <div className={cn('flex flex-col bg-term', PANE_HEIGHT)}>
      <div className="flex flex-wrap items-center gap-2 border-b px-3.5 py-2 text-xs">
        {tests.command && <span className="font-mono text-muted-foreground">{tests.command}</span>}
        {tests.skippedReason ? (
          <Chip>not run: {tests.skippedReason}</Chip>
        ) : (
          <>
            <TestRunChip run={tests.visible} label="tests" />
            {tests.hidden && <TestRunChip run={tests.hidden} label="with hidden tests" />}
          </>
        )}
        {tests.hidden && (
          <span className="ml-auto">
            <Segmented label="Which run" value={which} onChange={setWhich} options={[{ value: 'visible', label: 'Tests' }, { value: 'hidden', label: 'With hidden tests' }]} />
          </span>
        )}
      </div>
      {source ? (
        <TerminalView key={which} source={source} readOnly className={PANE_HEIGHT} />
      ) : (
        <p className="p-3.5 text-xs text-muted-foreground">
          {tests.skippedReason ? 'Nothing ran.' : !TERMINAL(run) ? 'The tests run in a fresh container once the agent ends.' : 'No output.'}
        </p>
      )}
    </div>
  )
}

const TERMINAL = (run: SideRun) => ['finished', 'error', 'cancelled', 'limit_reached'].includes(run.status)

const EVENT_TONE: Record<string, string> = { prompt: 'text-side-a', error: 'text-danger', patch: 'text-ok', tool: 'text-muted-foreground', message: 'text-foreground' }

/** What the agent did, from its CLI session: prompts, messages, tool calls, files changed. */
export function TimelineView({ id, run }: { id: string; run: SideRun }) {
  const { data, error, isLoading } = useTimeline(id, run.key)
  if (error) return <div className="p-3"><ErrorNote error={error} /></div>
  if (isLoading) return <div className="p-3"><LoadingRows rows={4} /></div>
  return (
    <div className={cn('overflow-auto bg-term px-3.5 py-3 font-mono text-[12.5px]', PANE_HEIGHT)}>
      {data && !data.ready && <p className="text-xs text-muted-foreground">The events are read from the CLI's session once the agent ends.</p>}
      {data?.ready && data.events.length === 0 && <p className="text-xs text-muted-foreground">The CLI recorded no events.</p>}
      {data?.events.map((e, i) => (
        <div key={i} className="flex gap-4 border-b border-border/60 py-1.5">
          <span className="tnum shrink-0 text-dim">{formatClock(e.at)}</span>
          <span className={cn('w-16 shrink-0', EVENT_TONE[e.kind] ?? 'text-muted-foreground')}>{e.kind}</span>
          <span className="min-w-0 break-words whitespace-pre-wrap">{e.detail}</span>
        </div>
      ))}
      {data?.sessionUsage && (
        <p className="mt-3 text-xs text-dim">
          Cross-check: the CLI's session counts {formatInt(data.sessionUsage.input + data.sessionUsage.cacheRead + (data.sessionUsage.cacheWrite ?? 0) + data.sessionUsage.output)} tokens
          {data.sessionCostUsd != null && ` (${formatUsd(data.sessionCostUsd)})`}; the proxy counts {formatInt(run.metrics.usage.input + run.metrics.usage.cacheRead + (run.metrics.usage.cacheWrite ?? 0) + run.metrics.usage.output)}.
          The proxy's figure is the one used: it also sees requests the CLI does not count, such as session titles.
        </p>
      )}
    </div>
  )
}
