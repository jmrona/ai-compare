import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { Download, FileDown, RefreshCw, RotateCcw, Trash2, TriangleAlert } from 'lucide-react'
import type { Comparison, SideKey, Tests } from '@/api/types'
import { downloadUrl } from '@/api/http'
import { useComparison, useDeleteComparison, useExportReport, useGenerateReport, useReport, useSetUserVerdict } from '@/api/queries'
import { STATUS_LABEL, harnessLabel, formatDateTime, formatRate, formatDuration, formatTokens, formatUsd, modeLabel } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { TopBar } from '@/components/app/AppShell'
import { ErrorNote, LoadingRows, Panel, Segmented, SideTag } from '@/components/common/primitives'
import { PreviewView } from '@/components/compare/PreviewView'
import { DiffView, HarnessView, LogsView, PaneTabs, SideTerminal, TestsView, TimelineView } from '@/components/compare/artifacts'
import {
  AnalysisSection, AuditSection, GatesPanel, H2, HarnessCostSection, JudgeVerdict, NotVerified, PromptBlock, ScoreBars, ScoreInfo,
  ScoreSections, SessionSection, SubagentsSection, UserVerdictPanel,
} from '@/components/report/ReportSections'

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
  const exportReport = useExportReport(c.id)
  const verdict = useSetUserVerdict(c.id)
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

  const current = report?.status === 'ready' && report.version >= 2 && report.sides ? report : null
  const download = () =>
    exportReport.mutate(undefined, {
      onSuccess: r => {
        const url = URL.createObjectURL(new Blob([r.markdown], { type: 'text/markdown' }))
        const a = document.createElement('a')
        a.href = url
        a.download = r.filename
        a.click()
        URL.revokeObjectURL(url)
      },
    })

  return (
    <>
      <TopBar crumbs={[{ label: 'History', to: '/history' }, { label: `#${c.id} ${c.projectName}` }]}>
        <Button size="sm" variant="outline" onClick={() => navigate({ to: '/', search: { from: c.id } })} title="Open a new comparison with this one's project, prompt and sides"><RefreshCw className="size-3.5" />Run again</Button>
        {(['A', 'B'] as const).map(s => c.sides[s].hasResult && (
          <Button key={s} size="sm" variant="outline" asChild title={`Download the files ${c.sides[s].config.model} produced`}>
            <a href={downloadUrl(c.id, s)} download><Download className="size-3.5" />{s}</a>
          </Button>
        ))}
        {current && <Button size="sm" variant="outline" disabled={exportReport.isPending} onClick={download}><FileDown className="size-3.5" />Export Markdown</Button>}
        {report && report.status !== 'generating' && c.report !== 'none' && (
          <Button size="sm" variant="outline" disabled={generate.isPending} onClick={() => generate.mutate()}><RotateCcw className="size-3.5" />Regenerate report</Button>
        )}
        <Button size="sm" variant="destructive" onClick={() => setConfirmDelete(true)}><Trash2 className="size-3.5" />Delete</Button>
      </TopBar>

      <div className="mx-auto grid w-full max-w-[1200px] gap-8 px-4 py-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <article className="min-w-0">
          <div className="font-mono text-xs text-dim">
            {formatDateTime(c.createdAt)} · {c.projectName} · {formatDuration(Math.max(A.metrics.elapsedSec, B.metrics.elapsedSec))} in total
          </div>
          <h1 className="mt-2 max-w-[60ch] text-[20px] leading-snug font-semibold text-balance">{firstLine(c.prompt)}</h1>
          <PromptBlock text={c.prompt} />
          {c.seriesId && (
            <p className="mt-2 text-[12.5px] text-muted-foreground">
              Attempt {c.attempt} of {c.seriesSize} ·{' '}
              <Link to="/series/$id" params={{ id: c.seriesId }} className="text-side-a underline-offset-2 hover:underline">series overview with aggregates</Link>
            </p>
          )}

          {(c.report === 'none' || c.report === 'error') && (
            <div className="mt-8 border border-dashed p-6">
              {c.report === 'error' ? (
                <p className="max-w-[60ch] text-danger">The report could not be generated: {report?.error || 'unknown error'}</p>
              ) : (
                <p className="max-w-[60ch] text-muted-foreground">This comparison has no report yet. The report checks each side against the acceptance criteria, reviews both diffs blind, scores both sides and lets a judge decide.</p>
              )}
              <Button className="mt-4" onClick={() => generate.mutate()} disabled={generate.isPending}>
                {c.report === 'error' ? 'Generate again' : 'Generate report'}
              </Button>
              {generate.error && <div className="mt-3"><ErrorNote error={generate.error} /></div>}
            </div>
          )}
          {(c.report === 'generating' || (c.report === 'ready' && isLoading)) && (
            <div className="mt-8 grid gap-3">
              <p className="text-muted-foreground">Generating the report: criteria, verification, review, score, judge and harness audit…</p>
              <LoadingRows rows={4} />
            </div>
          )}

          {report && report.status === 'ready' && !current && (
            <div className="mt-8 border border-dashed p-6">
              <p className="max-w-[60ch] text-muted-foreground">This report was made before scores, gates and the judge existed. Generate it again to see them.</p>
              <Button className="mt-4" onClick={() => generate.mutate()} disabled={generate.isPending}>Generate again</Button>
            </div>
          )}

          {current && (
            <>
              {current.warnings.length > 0 && (
                <div className="mt-4 grid gap-1 border border-warn/30 bg-warn/5 px-3 py-2 text-[12.5px] text-warn">
                  {current.warnings.map(w => <span key={w} className="flex gap-2"><TriangleAlert className="mt-0.5 size-3.5 shrink-0" />{w}</span>)}
                </div>
              )}

              <H2 sub={`${current.judgeModel} · two passes with the sides swapped`}>Judge's verdict</H2>
              <JudgeVerdict report={current} />

              <H2 right={<ScoreInfo />} sub="computed by ai-compare from the evidence below, not by a model">Score</H2>
              <ScoreBars sides={current.sides!} />
              <ScoreSections report={current} />
              <NotVerified sides={current.sides!} />
              <HarnessCostSection c={c} sides={current.sides!} />
              <SubagentsSection c={c} sides={current.sides!} />
              <SessionSection sides={current.sides!} />
              <AnalysisSection sides={current.sides!} judgeModel={current.judgeModel} />
              <AuditSection c={c} sides={current.sides!} judgeModel={current.judgeModel} />
            </>
          )}
        </article>

        <aside className="grid content-start gap-3">
          {current && (
            <>
              <Panel title="Score" right={<ScoreInfo />}>
                <div className="grid grid-cols-2 text-center">
                  {(['A', 'B'] as const).map(s => (
                    <div key={s} className="py-1">
                      <div className={cn('font-mono text-[34px] leading-none font-semibold', s === 'A' ? 'text-side-a' : 'text-side-b')}>{current.sides![s].score.total.toFixed(0)}</div>
                      <div className="mt-1 font-mono text-[11px] text-dim">{s} / 100</div>
                    </div>
                  ))}
                </div>
              </Panel>
              <Panel title="Gates"><GatesPanel sides={current.sides!} /></Panel>
            </>
          )}
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
                ['Harness', harnessLabel(A.config.harness), harnessLabel(B.config.harness)],
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
            </p>
          </Panel>
          {current && (
            <Panel title="Report">
              <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-[12.5px]">
                <dt className="text-dim">Reasoning</dt><dd className="m-0 truncate text-right font-mono">{current.judgeModel}</dd>
                <dt className="text-dim">Writing</dt><dd className="m-0 truncate text-right font-mono">{current.model}</dd>
                <dt className="text-dim">Criteria</dt><dd className="m-0 text-right font-mono">{current.criteriaBy === 'user' ? 'written by you' : 'by the judge'}</dd>
                <dt className="text-dim">Cost</dt><dd className="m-0 text-right font-mono">{formatUsd(current.costUsd)}</dd>
              </dl>
              {exportReport.error && <div className="mt-2"><ErrorNote error={exportReport.error} /></div>}
            </Panel>
          )}
          {current && (
            <Panel title="Your verdict">
              <UserVerdictPanel key={current.userVerdict?.verdict ?? ''} report={current} busy={verdict.isPending} onSave={v => verdict.mutate(v)} />
            </Panel>
          )}
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
              { value: 'harness', label: 'Harness', tag: c.sides[side].harnessFiles.length ? 'changed' : undefined, content: <HarnessView id={c.id} run={c.sides[side]} /> },
              { value: 'tests', label: 'Tests', content: <TestsView id={c.id} run={c.sides[side]} /> },
              { value: 'events', label: 'Events', content: <TimelineView id={c.id} run={c.sides[side]} /> },
              { value: 'preview', label: 'Preview', content: <PreviewView comparison={c} run={c.sides[side]} className="min-h-0 flex-1" /> },
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

function firstLine(prompt: string): string {
  const line = prompt.trim().split('\n')[0]
  return line.length > 120 ? line.slice(0, 119) + '…' : line
}

function testsText(t: Tests): string {
  const lint = t.lint ? ` · lint ${t.lint.status}` : ''
  if (t.skippedReason || !t.visible) return lint ? lint.slice(3) : '—'
  return (t.hidden ? `${t.visible.status} · hidden ${t.hidden.status}` : t.visible.status) + lint
}

function limitsText(l: Comparison['sides']['A']['config']['limits']) {
  const parts = [
    l.timeoutMin != null && `${l.timeoutMin} min`,
    l.maxTokensK != null && `${l.maxTokensK}k tokens`,
    l.maxCostUsd != null && formatUsd(l.maxCostUsd),
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : 'none'
}
