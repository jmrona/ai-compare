import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { Download, RefreshCw, Trash2, TriangleAlert } from 'lucide-react'
import type { Comparison, Finding, SideKey, Tests } from '@/api/types'
import { downloadUrl } from '@/api/http'
import { useComparison, useDeleteComparison, useGenerateReport, useReport } from '@/api/queries'
import { STATUS_LABEL, formatDateTime, formatRate, formatDuration, formatTokens, formatUsd, modeLabel } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, LoadingRows, Panel, Segmented, SideTag } from '@/components/common/primitives'
import { DiffView, LogsView, PaneTabs, SideTerminal, TestsView, TimelineView } from '@/components/compare/artifacts'

const SEVERITY: Record<Finding['severity'], { label: string; tone: 'danger' | 'warn' | 'dim' }> = {
  high: { label: 'High', tone: 'danger' },
  medium: { label: 'Medium', tone: 'warn' },
  low: { label: 'Low', tone: 'dim' },
}

export function ReportPage() {
  const { id } = useParams({ from: '/history/$id' })
  const { data: c, error, isLoading } = useComparison(id)

  if (isLoading) return <div className="p-4"><LoadingRows rows={6} /></div>
  if (error || !c) {
    return (
      <>
        <TopBar crumbs={[{ label: 'History', to: '/history' }, { label: `#${id}` }]} />
        <div className="grid max-w-xl gap-3 p-4">
          <ErrorNote error={error} />
          <Link to="/history" className="text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground">Go to the history</Link>
        </div>
      </>
    )
  }
  return <Report c={c} />
}

function Report({ c }: { c: Comparison }) {
  const navigate = useNavigate()
  const { data: report, isLoading } = useReport(c.id)
  const generate = useGenerateReport(c.id)
  const del = useDeleteComparison()
  const [side, setSide] = useState<SideKey>('A')
  const [tab, setTab] = useState('changes')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const A = c.sides.A
  const B = c.sides.B

  const best = (a: number | null, b: number | null, lowerWins = true): SideKey | null => {
    if (a == null || b == null || a === b) return null
    return (a < b) === lowerWins ? 'A' : 'B'
  }

  const jumpTo = (f: Finding) => {
    setSide(f.side)
    setTab('changes')
    document.getElementById('artefacts')?.scrollIntoView({ behavior: 'smooth' })
  }

  return (
    <>
      <TopBar crumbs={[{ label: 'History', to: '/history' }, { label: `#${c.id} ${c.projectName}` }]}>
        <Button size="sm" variant="outline" onClick={() => navigate({ to: '/', search: { from: c.id } })} title="Open a new comparison with this one's project, prompt and sides"><RefreshCw className="size-3.5" />Run again</Button>
        {(['A', 'B'] as const).map(s => c.sides[s].hasResult && (
          <Button key={s} size="sm" variant="outline" asChild title={`Download the files ${c.sides[s].config.model} produced`}>
            <a href={downloadUrl(c.id, s)} download><Download className="size-3.5" />{s}</a>
          </Button>
        ))}
        <Button size="sm" variant="destructive" onClick={() => setConfirmDelete(true)}><Trash2 className="size-3.5" />Delete</Button>
      </TopBar>

      <div className="mx-auto grid w-full max-w-[1160px] gap-8 px-4 py-8 lg:grid-cols-[minmax(0,1fr)_320px]">
        <article className="min-w-0">
          <div className="font-mono text-xs text-dim">
            {formatDateTime(c.createdAt)} · {c.projectName} · {formatDuration(Math.max(A.metrics.elapsedSec, B.metrics.elapsedSec))} in total
          </div>
          <h1 className="mt-2 max-w-[60ch] text-[22px] leading-snug font-semibold">{c.prompt}</h1>

          {(c.report === 'none' || c.report === 'error') && (
            <div className="mt-8 border border-dashed p-6">
              {c.report === 'error' ? (
                <p className="max-w-[60ch] text-danger">The report could not be generated: {report?.error || 'unknown error'}</p>
              ) : (
                <p className="max-w-[60ch] text-muted-foreground">This comparison has no report yet. The report reviews both diffs blind, analyses each side and compares the results.</p>
              )}
              <Button className="mt-4" onClick={() => generate.mutate()} disabled={generate.isPending}>
                {c.report === 'error' ? 'Generate again' : 'Generate report'}
              </Button>
              {generate.error && <div className="mt-3"><ErrorNote error={generate.error} /></div>}
            </div>
          )}
          {(c.report === 'generating' || (c.report === 'ready' && isLoading)) && (
            <div className="mt-8 grid gap-3">
              <p className="text-muted-foreground">Generating the report: blind review, per-side analysis and evaluation…</p>
              <LoadingRows rows={4} />
            </div>
          )}

          {report && report.status === 'ready' && (
            <>
              {report.warnings.length > 0 && (
                <div className="mt-4 grid gap-1 border border-warn/30 bg-warn/5 px-3 py-2 text-[12.5px] text-warn">
                  {report.warnings.map(w => <span key={w} className="flex gap-2"><TriangleAlert className="mt-0.5 size-3.5 shrink-0" />{w}</span>)}
                </div>
              )}
              <div className="mt-4 flex flex-wrap gap-1.5">
                {report.verdicts.map(v => (
                  <Chip key={v.label} tone={v.side === 'A' ? 'a' : v.side === 'B' ? 'b' : 'dim'}>
                    {v.label}: {v.side ?? 'tie'}
                  </Chip>
                ))}
              </div>

              <h2 className="mt-9 mb-3 text-[15px] font-semibold">Conclusions</h2>
              <div className="grid max-w-[68ch] gap-3 text-[14.5px] leading-[1.7] text-foreground/90">
                {report.conclusions.map((p, i) => <p key={i}>{p}</p>)}
              </div>

              <h2 className="mt-9 mb-3 text-[15px] font-semibold">Per-side analysis</h2>
              <div className="grid gap-px overflow-hidden border bg-border md:grid-cols-2">
                {(['A', 'B'] as const).map(s => (
                  <div key={s} className="bg-panel p-4">
                    <div className="mb-2 flex items-center gap-2"><SideTag side={s} /><span className="font-mono text-[13px]">{c.sides[s].config.model}</span></div>
                    <p className="text-[13.5px] leading-[1.65] text-muted-foreground">{report.perSide[s]}</p>
                  </div>
                ))}
              </div>

              <h2 className="mt-9 mb-3 flex items-baseline gap-2 text-[15px] font-semibold">
                Reviewer findings <span className="text-[12.5px] font-normal text-dim">blind review</span>
              </h2>
              {report.findings.length === 0 && <p className="text-muted-foreground">The reviewer found no issues.</p>}
              <div className="grid gap-2">
                {report.findings.map(f => (
                  <div key={f.title} className="grid gap-1 border bg-panel p-3.5 sm:grid-cols-[64px_1fr]">
                    <Chip tone={SEVERITY[f.severity].tone} className="w-fit">{SEVERITY[f.severity].label}</Chip>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2 text-sm font-medium"><SideTag side={f.side} small />{f.title}</div>
                      <p className="mt-1 text-[13.5px] leading-[1.6] text-muted-foreground">{f.impact}</p>
                      <button onClick={() => jumpTo(f)} className="mt-1 font-mono text-xs text-side-a hover:underline">{f.location}</button>
                    </div>
                  </div>
                ))}
              </div>
            </>
          )}
        </article>

        <aside className="grid content-start gap-3 lg:sticky lg:top-[62px] lg:self-start">
          <Panel title="Result" right={<span className="flex gap-[46px] pr-1"><SideTag side="A" small /><SideTag side="B" small /></span>}>
            <CompareRow label="Cost" a={formatUsd(A.metrics.costUsd)} b={formatUsd(B.metrics.costUsd)} best={best(A.metrics.costUsd, B.metrics.costUsd)} />
            <CompareRow label="Agent time" a={formatDuration(A.metrics.agentSec)} b={formatDuration(B.metrics.agentSec)} best={best(A.metrics.agentSec, B.metrics.agentSec)} />
            <CompareRow label="Human wait" a={formatDuration(A.metrics.humanWaitSec)} b={formatDuration(B.metrics.humanWaitSec)} best={null} />
            <CompareRow
              label="Tokens"
              a={formatTokens(A.metrics.usage.input + A.metrics.usage.cacheRead + A.metrics.usage.output)}
              b={formatTokens(B.metrics.usage.input + B.metrics.usage.cacheRead + B.metrics.usage.output)}
              best={null}
            />
            <CompareRow label="Tok/s" a={formatRate(A.metrics.tokensPerSec)} b={formatRate(B.metrics.tokensPerSec)} best={best(A.metrics.tokensPerSec, B.metrics.tokensPerSec, false)} />
            <CompareRow label="Tests" a={testsText(A.tests)} b={testsText(B.tests)} best={null} />
            <CompareRow label="Files changed" a={String(A.files.length)} b={String(B.files.length)} best={null} />
            <CompareRow label="Status" a={STATUS_LABEL[A.status]} b={STATUS_LABEL[B.status]} best={null} />
          </Panel>
          <Panel title="Configuration">
            <dl className="grid gap-2 text-[12.5px]">
              {([
                ['CLI', `${A.config.cli} ${A.cliVersion}`, `${B.config.cli} ${B.cliVersion}`],
                ['Model', A.config.model, B.config.model],
                ['Effort', A.config.effort, B.config.effort],
                ['Mode', modeLabel(A.config.mode), modeLabel(B.config.mode)],
                ['Limits', limitsText(A.config.limits), limitsText(B.config.limits)],
              ] as const).map(([k, a, b]) => (
                <div key={k} className="grid grid-cols-[64px_1fr_1fr] gap-2">
                  <dt className="text-dim">{k}</dt>
                  <dd className="truncate font-mono">{a}</dd>
                  <dd className="truncate font-mono">{b}</dd>
                </div>
              ))}
            </dl>
            <p className="mt-3 border-t pt-2 text-xs leading-relaxed text-dim">
              Harness: {c.harness}<br />
              Prices: models.dev, snapshot {formatDateTime(A.priceSnapshot.fetchedAt)}
              {report?.status === 'ready' && <><br />Report: {report.model} · {formatUsd(report.costUsd)}</>}
            </p>
          </Panel>
        </aside>
      </div>

      <section id="artefacts" className="border-t">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <h2 className="label-caps">Artefacts</h2>
          <Segmented
            label="Side"
            value={side}
            onChange={setSide}
            options={[{ value: 'A', label: `A · ${A.config.model}` }, { value: 'B', label: `B · ${B.config.model}` }]}
          />
        </div>
        <div className="flex h-[75dvh] min-h-[420px] flex-col bg-panel" key={side}>
          <PaneTabs
            side={side}
            value={tab}
            onChange={setTab}
            tabs={[
              { value: 'terminal', label: 'Terminal', content: <SideTerminal id={c.id} run={c.sides[side]} readOnly /> },
              { value: 'logs', label: 'Logs', content: <LogsView id={c.id} side={side} /> },
              { value: 'changes', label: 'Changes', content: <DiffView id={c.id} run={c.sides[side]} /> },
              { value: 'tests', label: 'Tests', content: <TestsView id={c.id} run={c.sides[side]} /> },
              { value: 'events', label: 'Events', content: <TimelineView id={c.id} run={c.sides[side]} /> },
            ]}
          />
        </div>
      </section>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete comparison #{c.id}?</DialogTitle>
            <DialogDescription>This deletes the report, the saved results, diffs and recordings, and ai-compare's containers and images for it. It cannot be undone.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
            <Button
              variant="destructive"
              disabled={del.isPending}
              onClick={() => del.mutate(c.id, { onSuccess: () => navigate({ to: '/history' }) })}
            >
              Delete comparison
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function CompareRow({ label, a, b, best }: { label: string; a: string; b: string; best: SideKey | null }) {
  return (
    <div className="grid grid-cols-[1fr_auto_auto] items-baseline gap-x-4 border-b py-2 last:border-0">
      <span className="text-[13px] text-muted-foreground">{label}</span>
      <span className={cn('tnum text-right font-mono text-[13px]', best === 'A' && 'text-ok')}>{a}</span>
      <span className={cn('tnum w-[70px] text-right font-mono text-[13px]', best === 'B' && 'text-ok')}>{b}</span>
    </div>
  )
}

function testsText(t: Tests): string {
  if (t.skippedReason || !t.visible) return '—'
  return t.hidden ? `${t.visible.status} · hidden ${t.hidden.status}` : t.visible.status
}

function limitsText(l: Comparison['sides']['A']['config']['limits']) {
  const parts = [
    l.timeoutMin != null && `${l.timeoutMin} min`,
    l.maxTokensK != null && `${l.maxTokensK}k tokens`,
    l.maxCostUsd != null && formatUsd(l.maxCostUsd),
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : 'none'
}
