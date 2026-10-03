import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { Download, Flag, Plug, Square } from 'lucide-react'
import type { Comparison, SideKey, SideRun } from '@/api/types'
import { TERMINAL_STATUSES } from '@/api/types'
import { isLive, useComparison, useGenerateReport, useSettings, useSideAction } from '@/api/queries'
import { downloadUrl } from '@/api/http'
import { STATUS_LABEL, formatDuration, formatRate, formatSeconds, formatTokens, formatUsd, modeLabel } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { TopBar } from '@/components/app/AppShell'
import { Chip, Dot, ErrorNote, LoadingRows, Metric, SideTag, StatusLabel } from '@/components/common/primitives'
import { DiffView, LogsView, MetricsView, PaneTabs, SideTerminal, TestsView, TimelineView } from '@/components/compare/artifacts'

export function RunPage() {
  const { id } = useParams({ from: '/comparisons/$id' })
  const { data: c, error, isLoading } = useComparison(id)
  const awaySec = useRestoredSession(id, isLive(c))

  if (isLoading) return <div className="p-4"><LoadingRows rows={4} /></div>
  if (error || !c) {
    return (
      <>
        <TopBar crumbs={[{ label: 'Comparisons', to: '/history' }, { label: `#${id}` }]} />
        <div className="grid max-w-xl gap-3 p-4">
          <ErrorNote error={error} />
          <Link to="/history" className="text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground">Go to the history</Link>
        </div>
      </>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TopBar crumbs={[{ label: 'Comparisons', to: '/history' }, { label: `#${c.id} ${c.projectName}` }]}>
        {isLive(c) ? (
          <span className="inline-flex items-center gap-1.5 bg-warn/10 px-2 py-0.5 text-xs text-warn">
            <Dot tone="warn" live />running · {formatDuration(Math.max(c.sides.A.metrics.elapsedSec, c.sides.B.metrics.elapsedSec))}
          </span>
        ) : (
          <Chip tone="ok">finished</Chip>
        )}
        <Button size="sm" variant="outline" asChild>
          <Link to="/">New comparison</Link>
        </Button>
      </TopBar>

      {awaySec != null && (
        <div role="status" className="flex flex-wrap items-center gap-2 border-b border-side-a/30 bg-side-a/10 px-4 py-2 text-[12.5px] text-side-a">
          <Plug className="size-3.5" />
          Session restored. The comparison kept running for {formatDuration(awaySec)} while you were away; the terminals reconnected with their output.
        </div>
      )}

      <p className="truncate border-b px-4 py-2 font-mono text-[12.5px] text-muted-foreground" title={c.prompt}>
        <span className="mr-2 text-dim">prompt</span>{c.prompt}
      </p>

      <div className="grid min-h-0 flex-1 gap-px bg-border xl:grid-cols-2 xl:grid-rows-[minmax(0,1fr)]">
        <SidePane comparison={c} side="A" />
        <SidePane comparison={c} side="B" />
      </div>

      <ReportBar comparison={c} />
    </div>
  )
}

function SidePane({ comparison, side }: { comparison: Comparison; side: SideKey }) {
  const run = comparison.sides[side]
  const [tab, setTab] = useState('terminal')
  const action = useSideAction(comparison.id)
  const done = TERMINAL_STATUSES.includes(run.status)
  const interactive = run.config.mode === 'interactive'
  const m = run.metrics
  // The saved result once the side has ended, a snapshot of the container while it runs.
  const canDownload = run.hasResult || run.status === 'running'

  return (
    <section aria-label={`Side ${side}`} className="flex h-[85dvh] min-h-0 min-w-0 flex-col bg-panel xl:h-auto">
      <div className="flex flex-wrap items-center gap-2 px-3.5 pt-2.5 pb-1.5">
        <SideTag side={side} />
        <span className="font-mono text-sm font-semibold">{run.config.model}</span>
        <Chip>{run.config.cli}</Chip>
        <Chip>{run.config.effort}</Chip>
        <Chip>{modeLabel(run.config.mode)}</Chip>
        <span className="ml-auto"><StatusLabel status={run.status} /></span>
      </div>
      <div className="flex flex-wrap items-end gap-x-5 gap-y-2 px-3.5 pb-2.5">
        <Metric label="time" value={formatDuration(m.elapsedSec)} sub={m.prepSec > 0 ? `prep ${formatSeconds(m.prepSec)}` : prepPhase(run.status)} />
        <Metric label="tokens" value={formatTokens(m.usage.input + m.usage.cacheRead + m.usage.output)} />
        <Metric label="cost" value={formatUsd(m.costUsd)} sub={run.config.limits.maxCostUsd != null ? `of ${formatUsd(run.config.limits.maxCostUsd)}` : undefined} />
        <Metric label="tok/s" value={formatRate(m.tokensPerSec)} />
        <span className="ml-auto flex flex-wrap items-center justify-end gap-2">
          {done && run.endReason && <span className="text-xs text-dim">{run.endReason}</span>}
          {canDownload && (
            <Button size="sm" variant="outline" asChild title={done ? 'Download the files this model produced' : 'Download the files as they are right now'}>
              <a href={downloadUrl(comparison.id, side)} download>
                <Download className="size-3.5" />Download
              </a>
            </Button>
          )}
          {!done && (<>
            <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => action.mutate({ side, action: 'finish' })}>
              <Flag className="size-3.5" />Finish
            </Button>
            <Button size="sm" variant="destructive" disabled={action.isPending} onClick={() => action.mutate({ side, action: 'cancel' })}>
              <Square className="size-3" />Cancel
            </Button>
          </>)}
        </span>
      </div>
      <PaneTabs
        side={side}
        value={tab}
        onChange={setTab}
        tabs={[
          { value: 'terminal', label: 'Terminal', content: <SideTerminal id={comparison.id} run={run} readOnly={!interactive || done} /> },
          { value: 'logs', label: 'Logs', content: <LogsView id={comparison.id} side={side} /> },
          { value: 'changes', label: 'Changes', content: <DiffView id={comparison.id} run={run} live={run.status === 'running'} /> },
          { value: 'metrics', label: 'Metrics', content: <MetricsView run={run} /> },
          { value: 'tests', label: 'Tests', content: <TestsView id={comparison.id} run={run} /> },
          { value: 'events', label: 'Events', content: <TimelineView id={comparison.id} run={run} /> },
          { value: 'preview', label: 'Preview', tag: 'phase 2', disabled: true },
        ]}
      />
    </section>
  )
}

/** While preparing, the time sub-label says which step is running. */
function prepPhase(status: SideRun['status']): string | undefined {
  return ['copying', 'building', 'starting'].includes(status) ? STATUS_LABEL[status] + '…' : undefined
}

function ReportBar({ comparison: c }: { comparison: Comparison }) {
  const navigate = useNavigate()
  const generate = useGenerateReport(c.id)
  const { data: settings } = useSettings()
  const aDone = TERMINAL_STATUSES.includes(c.sides.A.status)
  const bDone = TERMINAL_STATUSES.includes(c.sides.B.status)
  const both = aDone && bDone
  const waitingFor = !aDone && !bDone ? 'Both sides are still working.' : !aDone ? 'Side A is still working.' : 'Side B is still working.'
  const early = settings?.autoReport ? ' The review and analysis of a side that has ended are prepared in the background.' : ''

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t bg-panel px-4 py-2.5">
      <span className="text-[12.5px] text-muted-foreground">
        {!both ? waitingFor + early : c.report === 'error' ? 'The report could not be generated; you can try again.' : 'Both sides have ended.'}
      </span>
      {c.report === 'ready' ? (
        <Button asChild><Link to="/history/$id" params={{ id: c.id }}>View report</Link></Button>
      ) : (
        <Button
          disabled={!both || generate.isPending || c.report === 'generating'}
          onClick={() => generate.mutate(undefined, { onSuccess: () => navigate({ to: '/history/$id', params: { id: c.id } }) })}
        >
          {c.report === 'generating' ? 'Generating report…' : c.report === 'error' ? 'Generate again' : 'Generate report'}
        </Button>
      )}
    </div>
  )
}

/**
 * Detects a reopened tab: it records when the comparison was last on screen and, if a while
 * has passed and it is still running, returns how many seconds the user was away.
 */
function useRestoredSession(id: string, live: boolean): number | null {
  const key = `ai-compare:last-seen:${id}`
  // Read once, before this visit starts overwriting the timestamp.
  const [awaySec] = useState<number | null>(() => {
    try {
      const last = Number(localStorage.getItem(key))
      return last && Date.now() - last > 15_000 ? (Date.now() - last) / 1000 : null
    } catch {
      return null // No storage available: skip the notice.
    }
  })

  useEffect(() => {
    if (!live) return
    const mark = () => {
      try {
        localStorage.setItem(key, String(Date.now()))
      } catch {
        // Same as above.
      }
    }
    mark()
    const timer = window.setInterval(mark, 5000)
    window.addEventListener('beforeunload', mark)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('beforeunload', mark)
      mark()
    }
  }, [key, live])

  return live ? awaySec : null
}
