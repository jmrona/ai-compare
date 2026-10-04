import type { ReactNode } from 'react'
import { useState } from 'react'
import { ChevronDown, ChevronUp, CircleCheck, CircleX, ClipboardCheck, EyeOff, GitFork, Info, Lightbulb, ThumbsUp, TriangleAlert, Trophy } from 'lucide-react'
import type { Comparison, Criterion, Report, ScorePart, SideKey, SideReport } from '@/api/types'
import { formatDuration, formatInt, formatTokens, formatUsd } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Chip, SideTag } from '@/components/common/primitives'
import { BarChart, LineChart } from './charts'

const SIDES: SideKey[] = ['A', 'B']
const SIDE_COLOUR: Record<SideKey, string> = { A: 'var(--side-a)', B: 'var(--side-b)' }
const PART_COLOUR: Record<ScorePart['key'], string> = { functionality: '#8fb8e8', quality: '#a99bea', process: '#7fcfc0', efficiency: '#d9b46c' }
const STATUS: Record<SideReport['criteria'][number]['status'], { label: string; className: string }> = {
  met: { label: 'met', className: 'text-ok' },
  partial: { label: 'partial', className: 'text-warn' },
  not_met: { label: 'not met', className: 'text-danger' },
  not_verifiable: { label: 'not verifiable', className: 'text-muted-foreground' },
}
const SEVERITY_CLASS = { high: 'text-danger', medium: 'text-warn', low: 'text-muted-foreground' }
const SUGGESTION_KIND: Record<string, string> = {
  add_rule: 'add rule', add_skill: 'add skill', compact: 'compact', split: 'split skill', move_to_skill: 'move to a skill', remove: 'remove', other: 'change',
}
const pts = (v: number) => (Number.isInteger(v) ? String(v) : v.toFixed(1))

export function H2({ icon, children, sub, right }: { icon?: ReactNode; children: ReactNode; sub?: ReactNode; right?: ReactNode }) {
  return (
    <h2 className="mt-10 mb-3 flex flex-wrap items-baseline gap-x-2 gap-y-1 text-[15px] font-semibold">
      {icon && <span className="self-center text-muted-foreground">{icon}</span>}
      {children}
      {right}
      {sub && <span className="text-[12.5px] font-normal text-dim">{sub}</span>}
    </h2>
  )
}

export function PromptBlock({ text }: { text: string }) {
  const lines = text.split('\n').length
  const long = lines > 6 || text.length > 600
  const [open, setOpen] = useState(false)
  return (
    <div className="mt-3 max-w-[80ch]">
      <div
        id="report-prompt"
        className={cn('border bg-panel px-3 py-2.5 text-[13.5px] leading-[1.6] whitespace-pre-wrap', long && !open && 'max-h-[calc(1.6em*6+20px)] overflow-hidden [mask-image:linear-gradient(to_bottom,#000_45%,transparent)]')}
      >
        {text}
      </div>
      {long && (
        <button type="button" aria-expanded={open} aria-controls="report-prompt" onClick={() => setOpen(o => !o)} className="mt-1 flex items-center gap-1 text-[12.5px] text-muted-foreground hover:text-foreground">
          {open ? <ChevronUp className="size-3.5" /> : <ChevronDown className="size-3.5" />}
          {open ? 'Show less' : `Show the full prompt · ${lines} lines`}
        </button>
      )}
    </div>
  )
}

export function JudgeVerdict({ report }: { report: Report }) {
  const j = report.judge
  const sides = report.sides
  if (!j || !sides) return null
  const tone = j.winner ?? 'A'
  return (
    <section aria-label="Judge's verdict" className={cn('grid gap-3 border p-4', j.winner === 'B' ? 'border-side-b/45 bg-side-b/5' : j.winner === 'A' ? 'border-side-a/45 bg-side-a/5' : 'bg-panel')}>
      <div className="flex flex-wrap items-center gap-2.5">
        <Trophy className={cn('size-4', tone === 'A' ? 'text-side-a' : 'text-side-b')} />
        <strong className="text-[18px]">{j.winner ? `Side ${j.winner} wins` : 'A tie'}</strong>
        <Chip tone={j.confidence === 'high' ? 'ok' : j.confidence === 'medium' ? 'warn' : 'danger'}>
          confidence {j.confidence}{j.passesAgree ? ' · both passes agree' : ` · passes disagree (${j.passes.map(p => p ?? 'tie').join(', then ')})`}
        </Chip>
        <Chip>A {pts(sides.A.score.total)} / 100 · B {pts(sides.B.score.total)} / 100</Chip>
      </div>
      {report.headline && <p className="m-0 max-w-[72ch] text-[14.5px] text-foreground/90">{report.headline}</p>}
      <ul className="m-0 grid max-w-[72ch] list-disc gap-1 pl-5 text-[13.5px] leading-[1.6]">
        {j.reasons.map((r, i) => <li key={i}>{r}</li>)}
      </ul>
      <div className="grid gap-3 sm:grid-cols-2">
        {SIDES.map(s => j.ship[s] && (
          <div key={s} className="border-t pt-2 text-[13px]">
            <SideTag side={s} small /> <b>Would ship {s}?</b> {j.ship[s]!.yes ? 'Yes.' : 'No.'} {j.ship[s]!.reason}
          </div>
        ))}
      </div>
      <div className="flex flex-wrap gap-1.5">
        {j.labels.map(l => <Chip key={l.label} tone={l.side === 'A' ? 'a' : l.side === 'B' ? 'b' : 'dim'}>{l.label} · {l.side ?? 'tie'}</Chip>)}
      </div>
      {j.disagreements.length > 0 && (
        <div className="text-[12.5px] text-muted-foreground">
          Disagreements with the score: {j.disagreements.join(' ')}
        </div>
      )}
    </section>
  )
}

export function ScoreInfo() {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)} aria-label="How the score works" className="text-muted-foreground hover:text-foreground"><Info className="size-4" /></button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>How the score works</DialogTitle>
            <DialogDescription>Each side gets 0 to 100. ai-compare adds the points itself from the evidence in the report; models check the evidence but never pick the number.</DialogDescription>
          </DialogHeader>
          <div className="grid max-h-[65vh] gap-3 overflow-y-auto text-[13px] leading-relaxed">
            <table className="w-full border text-left text-[12.5px]">
              <thead className="bg-panel font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">
                <tr><th className="p-2 font-normal">Part</th><th className="p-2 font-normal">Points</th><th className="p-2 font-normal">How</th></tr>
              </thead>
              <tbody className="[&_td]:border-t [&_td]:p-2 [&_td]:align-top">
                <tr><td>Functionality</td><td className="font-mono">45</td><td>Acceptance criteria 30: required ×2, desirable ×1; met 1, partial 0.5, not met 0; not verifiable is left out. Existing tests still pass 8, new tests that pass 4, linter clean 3.</td></tr>
                <tr><td>Code quality</td><td className="font-mono">25</td><td>Starts at 20. Problems: high −8, medium −3, low −1. Strengths: +1 each, up to +5. Each one names a file and line.</td></tr>
                <tr><td>Process</td><td className="font-mono">10</td><td>Failed commands left unfixed −2 each (up to −6), ending with a question −2, more than 20 % of tool calls failing −2.</td></tr>
                <tr><td>Efficiency</td><td className="font-mono">20</td><td>Cost 10 and agent time 10: the better side gets 10, the other 10 × better ÷ its own. Only when functionality is at least 27.</td></tr>
              </tbody>
            </table>
            <p><b>Gates come first.</b> A side that did not finish, made no changes, broke tests or the linter that passed on the original project, or missed a required criterion cannot win, whatever its score.</p>
            <p><b>Missing checks.</b> Without existing tests, their 8 points go to new tests that pass. Without a linter, its 3 points go to the criteria, and the reviewer weighs consistency in Code quality.</p>
            <p><b>Scope.</b> Efficiency compares the two sides, so a score compares fully within a comparison or a series, and only roughly across comparisons.</p>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

export function ScoreBars({ sides }: { sides: Record<SideKey, SideReport> }) {
  const parts = sides.A.score.parts
  return (
    <div className="border bg-panel p-3.5">
      <div className="grid gap-2.5">
        {SIDES.map(s => (
          <div key={s} className="grid grid-cols-[28px_minmax(0,1fr)_56px] items-center gap-3">
            <SideTag side={s} small />
            <div className="flex h-[22px] overflow-hidden border bg-term" role="img" aria-label={`${s}: ${sides[s].score.parts.map(p => `${p.label} ${pts(p.points)}`).join(', ')}`}>
              {sides[s].score.parts.map(p => p.points > 0 && (
                <span key={p.key} className="flex items-center justify-center overflow-hidden font-mono text-[10.5px] whitespace-nowrap text-background" style={{ width: `${p.points}%`, background: PART_COLOUR[p.key] }}>
                  {pts(p.points)}
                </span>
              ))}
            </div>
            <span className={cn('text-right font-mono text-[22px] font-semibold', s === 'A' ? 'text-side-a' : 'text-side-b')}>{pts(sides[s].score.total)}</span>
          </div>
        ))}
      </div>
      <div className="mt-2 flex flex-wrap gap-3.5 text-xs text-muted-foreground">
        {parts.map(p => <span key={p.key} className="flex items-center gap-1.5"><span className="size-2.5" style={{ background: PART_COLOUR[p.key] }} />{p.label} / {pts(p.max)}</span>)}
      </div>
    </div>
  )
}

function Table({ head, children, foot }: { head: ReactNode; children: ReactNode; foot?: ReactNode }) {
  return (
    <div className="overflow-x-auto border">
      <table className="w-full text-left text-[13px]">
        <thead className="bg-panel font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">{head}</thead>
        <tbody className="[&_td]:border-t [&_td]:px-2.5 [&_td]:py-2 [&_td]:align-top">{children}</tbody>
        {foot && <tfoot className="bg-panel font-semibold [&_td]:border-t [&_td]:px-2.5 [&_td]:py-2">{foot}</tfoot>}
      </table>
    </div>
  )
}

const Ev = ({ children }: { children: ReactNode }) => <span className="mt-0.5 block font-mono text-[11.5px] text-dim">{children}</span>

function partOf(s: SideReport, key: ScorePart['key']) {
  return s.score.parts.find(p => p.key === key)
}

function LinesTable({ sides, partKey, skip = [] }: { sides: Record<SideKey, SideReport>; partKey: ScorePart['key']; skip?: string[] }) {
  const a = partOf(sides.A, partKey)
  const b = partOf(sides.B, partKey)
  if (!a || !b) return null
  const labels = [...new Set([...a.lines, ...b.lines].map(l => l.label))].filter(l => !skip.includes(l))
  const line = (p: ScorePart, label: string) => p.lines.find(l => l.label === label)
  return (
    <Table
      head={<tr><th className="p-2 font-normal">Rule</th><th className="p-2 font-normal">A</th><th className="p-2 font-normal">B</th></tr>}
      foot={<tr><td>{a.label} · {pts(a.max)} points</td><td className="text-right font-mono">{pts(a.points)}</td><td className="text-right font-mono">{pts(b.points)}</td></tr>}
    >
      {labels.map(label => (
        <tr key={label}>
          <td>{label}{line(a, label)?.max ? <span className="text-dim"> · {pts(line(a, label)!.max)}</span> : null}</td>
          {[a, b].map((p, i) => {
            const l = line(p, label)
            return <td key={i}>{l ? <>{l.detail}<Ev>{pts(l.points)}</Ev></> : <span className="text-dim">—</span>}</td>
          })}
        </tr>
      ))}
    </Table>
  )
}

export function ScoreSections({ report }: { report: Report }) {
  const sides = report.sides!
  const fa = partOf(sides.A, 'functionality')
  const fb = partOf(sides.B, 'functionality')
  return (
    <>
      <h3 className="mt-6 mb-2 text-[13.5px] font-semibold">Functionality <span className="font-mono text-xs font-normal text-dim">A {pts(fa?.points ?? 0)} · B {pts(fb?.points ?? 0)} of 45</span></h3>
      <CriteriaTable criteria={report.criteria} by={report.criteriaBy} sides={sides} />
      <div className="mt-2.5"><LinesTable sides={sides} partKey="functionality" /></div>

      <h3 className="mt-6 mb-2 text-[13.5px] font-semibold">Code quality <span className="font-mono text-xs font-normal text-dim">base 20, problems subtract, strengths add up to +5</span></h3>
      <div className="grid gap-3 md:grid-cols-2">
        {SIDES.map(s => (
          <div key={s} className="min-w-0">
            <div className="mb-2 flex items-center gap-2 font-mono text-[12.5px]"><SideTag side={s} small />{pts(partOf(sides[s], 'quality')?.points ?? 0)} of 25</div>
            <div className="grid gap-1.5">
              {sides[s].review.problems.map((p, i) => (
                <Item key={`p${i}`} tag={<span className={cn('font-mono text-[11px] uppercase', SEVERITY_CLASS[p.severity])}>{p.severity}</span>} delta={-({ high: 8, medium: 3, low: 1 }[p.severity])}>
                  {p.title}<Ev>{p.location}</Ev>{p.impact && <span className="mt-1 block text-[12.5px] text-muted-foreground">{p.impact}</span>}
                </Item>
              ))}
              {sides[s].review.strengths.map((st, i) => (
                <Item key={`s${i}`} tag={<span className="font-mono text-[11px] text-ok uppercase">strength</span>} delta={i < 5 ? 1 : 0}>
                  {st.title}<Ev>{st.location}</Ev>
                </Item>
              ))}
              {sides[s].review.problems.length + sides[s].review.strengths.length === 0 && <p className="text-[13px] text-muted-foreground">Nothing to report.</p>}
            </div>
          </div>
        ))}
      </div>

      <h3 className="mt-6 mb-2 text-[13.5px] font-semibold">Process <span className="font-mono text-xs font-normal text-dim">from the CLI session</span></h3>
      <LinesTable sides={sides} partKey="process" />

      <h3 className="mt-6 mb-2 text-[13.5px] font-semibold">Efficiency <span className="font-mono text-xs font-normal text-dim">counts only with functionality ≥ 27</span></h3>
      <LinesTable sides={sides} partKey="efficiency" />
    </>
  )
}

function Item({ tag, delta, children }: { tag: ReactNode; delta: number; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[64px_minmax(0,1fr)_auto] items-start gap-2.5 border bg-panel px-2.5 py-2 text-[13px]">
      {tag}
      <div className="min-w-0">{children}</div>
      <span className={cn('font-mono text-xs font-semibold', delta > 0 ? 'text-ok' : delta < 0 ? 'text-danger' : 'text-dim')}>{delta > 0 ? `+${delta}` : delta < 0 ? `−${-delta}` : '0'}</span>
    </div>
  )
}

function CriteriaTable({ criteria, by, sides }: { criteria: Criterion[]; by: Report['criteriaBy']; sides: Record<SideKey, SideReport> }) {
  if (criteria.length === 0) return <p className="text-[13px] text-muted-foreground">No acceptance criteria.</p>
  const check = (s: SideKey, i: number) => sides[s].criteria.find(c => c.index === i)
  return (
    <>
      <Table head={<tr><th className="p-2 font-normal">Acceptance criterion</th><th className="p-2 font-normal">Weight</th><th className="p-2 font-normal">A</th><th className="p-2 font-normal">B</th></tr>}>
        {criteria.map((c, i) => (
          <tr key={i}>
            <td>{c.text}</td>
            <td className="font-mono text-[12px] whitespace-nowrap">{c.required ? 'required ×2' : 'desirable ×1'}</td>
            {SIDES.map(s => {
              const ch = check(s, i)
              return (
                <td key={s}>
                  {ch ? <><span className={cn('font-mono text-xs font-medium whitespace-nowrap', STATUS[ch.status].className)}>{STATUS[ch.status].label}</span><Ev>{ch.method !== 'none' ? `${ch.method} · ` : ''}{ch.evidence}</Ev></> : <span className="text-dim">—</span>}
                </td>
              )
            })}
          </tr>
        ))}
      </Table>
      <p className="mt-1.5 text-xs text-dim">Written by {by === 'user' ? 'you, before the comparison started' : 'the judge model from the prompt, without seeing the results'}.</p>
    </>
  )
}

export function NotVerified({ sides }: { sides: Record<SideKey, SideReport> }) {
  const total = sides.A.notVerified.length + sides.B.notVerified.length
  if (total === 0) return null
  return (
    <>
      <H2 icon={<EyeOff className="size-4 text-warn" />} sub="said out loud so that it can be weighed, never left out quietly">Not verified</H2>
      <div className="grid gap-3 border border-warn/35 bg-warn/5 p-3.5 md:grid-cols-2">
        {SIDES.map(s => (
          <div key={s} className="min-w-0">
            <div className="mb-1.5"><SideTag side={s} small /></div>
            <ul className="m-0 grid list-disc gap-1 pl-5 text-[13px]">
              {sides[s].notVerified.map((n, i) => <li key={i}>{n}</li>)}
              {sides[s].notVerified.length === 0 && <li className="list-none text-muted-foreground">Everything was checked.</li>}
            </ul>
          </div>
        ))}
      </div>
    </>
  )
}

const COST_COLOURS = ['#4a5568', '#6b7a90', '#c792ea', '#e0a7f0', '#9aa3ae']

export function HarnessCostSection({ c, sides }: { c: Comparison; sides: Record<SideKey, SideReport> }) {
  if (!sides.A.harness && !sides.B.harness) return null
  const labels = (sides.A.harness ?? sides.B.harness)!.parts.map(p => p.label)
  const maxTotal = Math.max(...SIDES.map(s => sides[s].harness?.firstRequestTokens ?? 0), 1)
  const rows: [string, (h: NonNullable<SideReport['harness']>) => ReactNode][] = [
    ['Harness tokens per request', h => formatInt(h.perRequest)],
    ['Requests that carried it', h => formatInt(h.requests)],
    ['Harness tokens sent in total', h => formatInt(h.total)],
    ['Served from the cache', h => `${Math.round(h.cacheShare * 100)} %`],
    ['Cost attributable to the harness', h => <>{formatUsd(h.costUsd)}{h.shareOfSide != null && <Ev>{Math.round(h.shareOfSide * 100)} % of the side</Ev>}</>],
    ['Skills loaded during the run', h => (h.skillsLoaded.length ? h.skillsLoaded.map(s => `${s.label} · ${formatInt(s.tokens)}`).join(', ') : 'none')],
    ['Largest harness files, tokens per request', h => (h.files.length ? h.files.slice(0, 3).map(f => `${f.label} ${formatInt(f.tokens)}`).join(' · ') : 'none')],
  ]
  return (
    <>
      <H2 sub="what loading the instructions costs, measured on the first request and sent again on every one">Harness cost</H2>
      <div className="border bg-panel p-3.5">
        <div className="mb-2 font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">
          First request · {SIDES.map(s => `${formatInt(sides[s].harness?.firstRequestTokens ?? 0)} tokens (${s})`).join(' · ')}
        </div>
        <div className="grid gap-2">
          {SIDES.map(s => {
            const h = sides[s].harness
            return (
              <div key={s} className="grid grid-cols-[28px_minmax(0,1fr)_56px] items-center gap-3">
                <SideTag side={s} small />
                <div style={{ width: `${((h?.firstRequestTokens ?? 0) / maxTotal) * 100}%` }}>
                  <div className="flex h-[24px] overflow-hidden border" role="img" aria-label={h ? `${s} first request: ${h.parts.map(p => `${p.label} ${p.tokens}`).join(', ')}` : `${s}: not measured`}>
                    {h?.parts.map((p, i) => <span key={p.label} title={`${p.label} · ${formatInt(p.tokens)}`} style={{ width: `${(p.tokens / Math.max(1, h.firstRequestTokens)) * 100}%`, background: COST_COLOURS[i] }} />)}
                  </div>
                </div>
                <span className="text-right font-mono text-xs text-dim">{h ? formatTokens(h.firstRequestTokens) : '—'}</span>
              </div>
            )
          })}
        </div>
        <div className="mt-2 flex flex-wrap gap-3.5 text-xs text-muted-foreground">
          {labels.map((l, i) => <span key={l} className="flex items-center gap-1.5"><span className="size-2.5" style={{ background: COST_COLOURS[i] }} />{l}</span>)}
        </div>
      </div>
      <div className="mt-2.5">
        <Table head={<tr><th className="p-2 font-normal" /><th className="p-2 font-normal">A · {c.sides.A.config.harness.kind === 'preset' ? c.sides.A.config.harness.title : c.sides.A.config.harness.kind}</th><th className="p-2 font-normal">B · {c.sides.B.config.harness.kind === 'preset' ? c.sides.B.config.harness.title : c.sides.B.config.harness.kind}</th></tr>}>
          {rows.map(([label, f]) => (
            <tr key={label}>
              <td>{label}</td>
              {SIDES.map(s => <td key={s} className="font-mono text-[12.5px]">{sides[s].harness ? f(sides[s].harness!) : <span className="text-dim">not measured</span>}</td>)}
            </tr>
          ))}
        </Table>
      </div>
      <p className="mt-1.5 text-xs text-dim">The request's total is exact; each part's share is estimated by its share of characters.</p>
    </>
  )
}

export function SubagentsSection({ c, sides }: { c: Comparison; sides: Record<SideKey, SideReport> }) {
  const any = SIDES.some(s => sides[s].subagents.length > 0)
  return (
    <>
      <H2 icon={<GitFork className="size-4" />} sub="what each subagent did, with which model, and what it cost">Subagents</H2>
      {!any && <p className="text-[13px] text-muted-foreground">Neither side launched a subagent.</p>}
      {any && (
        <Table head={<tr>{['Side', 'Subagent and task', 'Model', 'Tokens', 'Cost', 'Time', 'Share of the side', 'Result'].map(h => <th key={h} className="p-2 font-normal">{h}</th>)}</tr>}>
          {SIDES.flatMap(s => sides[s].subagents.map((a, i) => {
            const sideCost = c.sides[s].metrics.costUsd
            return (
              <tr key={`${s}${i}`}>
                <td><SideTag side={s} small /></td>
                <td><b>{a.type}</b> · {a.description}<Ev>{Object.entries(a.tools).map(([t, n]) => `${t} ${n}`).join(' · ')}</Ev></td>
                <td className="font-mono text-[12.5px]">{a.model}</td>
                <td className="font-mono text-[12.5px]">{formatTokens(a.tokens)}</td>
                <td className="font-mono text-[12.5px]">{formatUsd(a.costUsd)}</td>
                <td className="font-mono text-[12.5px]">{formatDuration(a.durationSec)}</td>
                <td className="font-mono text-[12.5px]">{sideCost ? `${Math.round((a.costUsd / sideCost) * 100)} %` : '—'}</td>
                <td><span className={cn('font-mono text-xs', a.status === 'completed' ? 'text-ok' : 'text-warn')}>{a.status}</span></td>
              </tr>
            )
          }))}
        </Table>
      )}
    </>
  )
}

const TOOL_COLOURS: Record<string, string> = { read: '#6cb6ff', glob: '#4f8fd6', grep: '#4f8fd6', edit: '#63d38f', apply_patch: '#63d38f', write: '#3fae6c', bash: '#e8c35a', todowrite: '#a99bea', skill: '#7fcfc0', task: '#d9b46c' }

export function SessionSection({ sides }: { sides: Record<SideKey, SideReport> }) {
  const a = sides.A.session
  const b = sides.B.session
  const tile = (label: string, va: ReactNode, vb: ReactNode, sub?: string) => (
    <div className="min-w-0 bg-panel px-3 py-2.5">
      <div className="font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">{label}</div>
      <div className="font-mono text-[16px] font-semibold"><span className="text-side-a">{va}</span> <span className="text-[11px] font-normal text-dim">/</span> <span className="text-side-b">{vb}</span></div>
      {sub && <div className="font-mono text-[11px] text-dim">{sub}</div>}
    </div>
  )
  const maxCalls = Math.max(a.toolCalls, b.toolCalls, 1)
  return (
    <>
      <H2 sub="from the CLI session and the proxy">Session</H2>
      <div className="grid grid-cols-2 gap-px border bg-border sm:grid-cols-4">
        {tile('Requests', a.requests, b.requests)}
        {tile('Input from cache', `${Math.round(a.cacheShare * 100)} %`, `${Math.round(b.cacheShare * 100)} %`)}
        {tile('Reasoning', `${a.reasoningSteps}×`, `${b.reasoningSteps}×`, `${formatInt(a.reasoningTokens)} / ${formatInt(b.reasoningTokens)} tokens`)}
        {tile('First edit after', a.firstEditSec != null ? formatDuration(a.firstEditSec) : '—', b.firstEditSec != null ? formatDuration(b.firstEditSec) : '—')}
      </div>
      <div className="mt-3 grid gap-3 lg:grid-cols-2">
        <div className="border bg-panel p-3">
          <div className="mb-1 font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">Context size per request · tokens over time</div>
          <LineChart
            label="Context size per request over time for A and B"
            xLabel="seconds"
            yFormat={v => formatTokens(v)}
            series={SIDES.map(s => ({ label: s, color: SIDE_COLOUR[s], points: sides[s].session.requestPoints.map(p => ({ x: p.atSec, y: p.context })) }))}
          />
        </div>
        <div className="border bg-panel p-3">
          <div className="mb-1 font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">Cost per request</div>
          <BarChart
            label="Cost per request for A and B"
            xLabel="request"
            yFormat={v => `$${v.toFixed(3)}`}
            series={SIDES.map(s => ({ label: s, color: SIDE_COLOUR[s], values: sides[s].session.requestPoints.map(p => p.costUsd ?? 0) }))}
          />
        </div>
        <div className="border bg-panel p-3">
          <div className="mb-1 font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">Reasoning tokens per step</div>
          <BarChart
            label="Reasoning tokens per step for A and B"
            xLabel="step"
            yFormat={v => formatTokens(v)}
            series={SIDES.map(s => ({ label: s, color: SIDE_COLOUR[s], values: sides[s].session.reasoningPoints }))}
          />
        </div>
        <div className="border bg-panel p-3">
          <div className="mb-2.5 font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">Tools used · failed in red</div>
          {SIDES.map(s => {
            const ss = sides[s].session
            const entries = Object.entries(ss.tools).sort((x, y) => y[1] - x[1])
            return (
              <div key={s} className="mb-2 grid grid-cols-[28px_minmax(0,1fr)_32px] items-center gap-2">
                <SideTag side={s} small />
                <div style={{ width: `${(ss.toolCalls / maxCalls) * 100}%` }}>
                  <div className="flex h-[22px] overflow-hidden border" role="img" aria-label={`${s} tools: ${entries.map(([t, n]) => `${t} ${n}`).join(', ')}; ${ss.toolFailures} failed`}>
                    {entries.map(([t, n]) => <span key={t} title={`${t} · ${n}`} style={{ width: `${((n - 0) / Math.max(1, ss.toolCalls)) * 100}%`, background: TOOL_COLOURS[t] ?? '#9aa3ae' }} />)}
                  </div>
                  {ss.toolFailures > 0 && <div className="h-1 bg-danger" style={{ width: `${(ss.toolFailures / Math.max(1, ss.toolCalls)) * 100}%` }} title={`${ss.toolFailures} failed`} />}
                </div>
                <span className="font-mono text-xs text-dim">{ss.toolCalls}</span>
              </div>
            )
          })}
          <div className="flex flex-wrap gap-3 text-[11.5px] text-muted-foreground">
            {[...new Set([...Object.keys(a.tools), ...Object.keys(b.tools)])].map(t => <span key={t} className="flex items-center gap-1.5"><span className="size-2.5" style={{ background: TOOL_COLOURS[t] ?? '#9aa3ae' }} />{t}</span>)}
          </div>
          <dl className="mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-[12.5px]">
            {([
              ['Subagents', sides.A.subagents.length, sides.B.subagents.length],
              ['Failed tool calls', a.toolFailures, b.toolFailures],
              ['Failed commands left unfixed', a.failedCommands.filter(f => !f.fixed).length, b.failedCommands.filter(f => !f.fixed).length],
              ['Provider errors and 429s', `${a.providerErrors} · ${a.rateLimited}`, `${b.providerErrors} · ${b.rateLimited}`],
              ['Long-context price (over the threshold)', a.longContextRequests, b.longContextRequests],
            ] as const).map(([k, va, vb]) => (
              <div key={k} className="contents">
                <dt className="text-dim">{k}</dt>
                <dd className="m-0 text-right font-mono"><span className="text-side-a">{va}</span> / <span className="text-side-b">{vb}</span></dd>
              </div>
            ))}
          </dl>
        </div>
      </div>
    </>
  )
}

export function AnalysisSection({ sides, judgeModel }: { sides: Record<SideKey, SideReport>; judgeModel: string }) {
  return (
    <>
      <H2 sub={judgeModel}>Analysis of each run</H2>
      <div className="grid gap-2">
        {SIDES.map(s => (
          <details key={s} className="border bg-panel">
            <summary className="flex cursor-pointer items-center gap-2 px-3 py-2.5 text-[13px]"><SideTag side={s} small />{sides[s].analysis.split('\n')[0].slice(0, 120)}</summary>
            <div className="grid max-w-[75ch] gap-2.5 px-3.5 pb-3 text-[13.5px] leading-[1.65] text-foreground/90">
              {sides[s].analysis.split(/\n\s*\n/).map((p, i) => <p key={i} className="m-0">{p}</p>)}
            </div>
          </details>
        ))}
      </div>
    </>
  )
}

export function AuditSection({ c, sides, judgeModel }: { c: Comparison; sides: Record<SideKey, SideReport>; judgeModel: string }) {
  if (!sides.A.audit && !sides.B.audit) return null
  return (
    <>
      <H2 icon={<ClipboardCheck className="size-4" />} sub={`${judgeModel} · what each harness did for its side, and how to improve it`}>Harness audit</H2>
      <div className="grid gap-3 md:grid-cols-2">
        {SIDES.map(s => {
          const a = sides[s].audit
          const h = c.sides[s].config.harness
          return (
            <div key={s} className="min-w-0">
              <div className="mb-2 flex flex-wrap items-center gap-2 font-mono text-[12.5px]">
                <SideTag side={s} small />{h.kind === 'preset' ? `preset ${h.title}` : h.kind === 'none' ? 'no harness' : "project's harness"}
                {sides[s].harness && <span className="text-dim">· {formatInt(sides[s].harness!.perRequest)} tokens per request</span>}
              </div>
              <div className="grid gap-1.5">
                {a?.strengths.map((it, i) => <AuditRow key={`s${i}`} icon={<ThumbsUp className="size-4 text-ok" />} tag="strength" title={it.title} meta={it.evidence} />)}
                {a?.gaps.map((it, i) => <AuditRow key={`g${i}`} icon={<TriangleAlert className="size-4 text-danger" />} tag="gap" title={it.title} meta={it.evidence} />)}
                {a?.suggestions.map((sg, i) => (
                  <AuditRow
                    key={`x${i}`}
                    icon={<Lightbulb className="size-4 text-warn" />}
                    tag={SUGGESTION_KIND[sg.kind] ?? sg.kind}
                    title={sg.change}
                    meta={`${sg.file}${sg.evidence ? ` · ${sg.evidence}` : ''}`}
                    gain={sg.tokensSaved > 0 ? `−${formatInt(sg.tokensSaved)} tok` : undefined}
                  />
                ))}
                {!a && <p className="text-[13px] text-muted-foreground">Not audited.</p>}
              </div>
            </div>
          )
        })}
      </div>
      <p className="mt-1.5 text-xs text-dim">Token savings are estimates per request, from the measured sizes.</p>
    </>
  )
}

function AuditRow({ icon, tag, title, meta, gain }: { icon: ReactNode; tag: string; title: string; meta: string; gain?: string }) {
  return (
    <div className="grid grid-cols-[18px_minmax(0,1fr)_auto] items-start gap-2.5 border bg-panel px-2.5 py-2 text-[13px]">
      {icon}
      <div className="min-w-0">{title}<Ev>{meta}</Ev></div>
      {gain ? <span className="font-mono text-xs font-semibold whitespace-nowrap text-ok">{gain}</span> : <Chip>{tag}</Chip>}
    </div>
  )
}

export function GatesPanel({ sides }: { sides: Record<SideKey, SideReport> }) {
  return (
    <div>
      <div className="grid grid-cols-[minmax(0,1fr)_28px_28px] gap-1.5 pb-1 font-mono text-[10.5px] text-dim"><span /><span className="text-center">A</span><span className="text-center">B</span></div>
      {sides.A.gates.map((g, i) => {
        const gb = sides.B.gates[i]
        return (
          <div key={g.key} className="grid grid-cols-[minmax(0,1fr)_28px_28px] items-center gap-1.5 border-b py-1.5 text-[12.5px] last:border-0">
            <span>{g.label}</span>
            {[g, gb].map((x, k) => (
              <span key={k} className="flex justify-center" title={x?.reason}>
                {x?.passed ? <CircleCheck className="size-4 text-ok" aria-label="passed" /> : <CircleX className="size-4 text-danger" aria-label={`failed: ${x?.reason}`} />}
              </span>
            ))}
          </div>
        )
      })}
      <p className="mt-2 text-xs text-dim">A side that fails a gate cannot win.</p>
    </div>
  )
}

export function UserVerdictPanel({ report, onSave, busy }: { report: Report; onSave: (v: { verdict: '' | 'agree' | 'other' | 'tie'; note: string }) => void; busy: boolean }) {
  const [note, setNote] = useState(report.userVerdict?.note ?? '')
  const current = report.userVerdict?.verdict ?? ''
  const options = [
    { value: 'agree' as const, label: 'Judge is right' },
    { value: 'other' as const, label: report.judge?.winner ? `${report.judge.winner === 'A' ? 'B' : 'A'} was better` : 'One side was better' },
    { value: 'tie' as const, label: 'Tie' },
  ]
  return (
    <div className="grid gap-2">
      <p className="m-0 text-xs text-dim">Tells ai-compare whether the judge got it right, to tune it over time.</p>
      <div className="flex flex-wrap gap-1.5">
        {options.map(o => (
          <Button key={o.value} size="sm" variant={current === o.value ? 'default' : 'outline'} disabled={busy} onClick={() => onSave({ verdict: current === o.value ? '' : o.value, note })}>
            {o.label}
          </Button>
        ))}
      </div>
      <label htmlFor="verdict-note" className="font-mono text-[10.5px] tracking-[0.07em] text-dim uppercase">Note</label>
      <textarea
        id="verdict-note"
        rows={2}
        className="w-full border bg-term px-2 py-1.5 text-[13px] outline-none focus-visible:ring-1 focus-visible:ring-ring"
        placeholder="Why, in a line"
        value={note}
        onChange={e => setNote(e.target.value)}
        onBlur={() => current && note !== (report.userVerdict?.note ?? '') && onSave({ verdict: current, note })}
      />
    </div>
  )
}
