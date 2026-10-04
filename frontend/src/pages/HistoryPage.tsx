import { useMemo, useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { Check, Search } from 'lucide-react'
import type { Comparison, SideKey } from '@/api/types'
import { useHistory } from '@/api/queries'
import { formatDay, formatDuration, formatTime, formatUsd } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, LoadingRows, SideTag } from '@/components/common/primitives'

type StateFilter = 'all' | 'report' | 'no-report' | 'errors'

/** Groups by day: Today, Yesterday, then the date. */
function groupByDay(items: Comparison[]) {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const groups = new Map<string, { title: string; subtitle: string; items: Comparison[] }>()
  for (const c of items) {
    const d = new Date(c.createdAt)
    d.setHours(0, 0, 0, 0)
    const daysAgo = Math.round((today.getTime() - d.getTime()) / 86_400_000)
    const key = d.toISOString()
    const day = formatDay(c.createdAt)
    const title = daysAgo === 0 ? 'Today' : daysAgo === 1 ? 'Yesterday' : day.charAt(0).toUpperCase() + day.slice(1)
    if (!groups.has(key)) groups.set(key, { title, subtitle: daysAgo <= 1 ? day : '', items: [] })
    groups.get(key)!.items.push(c)
  }
  return [...groups.values()]
}

const cheaper = (c: Comparison): SideKey | null => {
  const a = c.sides.A.metrics.costUsd
  const b = c.sides.B.metrics.costUsd
  if (a == null || b == null || a === b) return null
  return a < b ? 'A' : 'B'
}

export function HistoryPage() {
  const navigate = useNavigate()
  const { data, error, isLoading } = useHistory()
  const { preset } = useSearch({ from: '/history' })
  const [q, setQ] = useState('')
  const [model, setModel] = useState('all')
  const [state, setState] = useState<StateFilter>('all')

  // Only models that appear in the history are worth filtering by.
  const usedModels = useMemo(() => [...new Set((data ?? []).flatMap(c => [c.sides.A.config.model, c.sides.B.config.model]))].sort(), [data])

  const filtered = useMemo(() => {
    const text = q.trim().toLowerCase()
    return (data ?? []).filter(c => {
      if (preset && c.sides.A.config.harness.preset !== preset && c.sides.B.config.harness.preset !== preset) return false
      if (text && !(c.prompt + ' ' + c.projectName + ' ' + c.id).toLowerCase().includes(text)) return false
      if (model !== 'all' && c.sides.A.config.model !== model && c.sides.B.config.model !== model) return false
      const failed = ['error', 'limit_reached'].includes(c.sides.A.status) || ['error', 'limit_reached'].includes(c.sides.B.status)
      if (state === 'report' && c.report !== 'ready') return false
      if (state === 'no-report' && c.report === 'ready') return false
      if (state === 'errors' && !failed) return false
      return true
    })
  }, [data, q, model, state, preset])

  return (
    <>
      <TopBar crumbs={[{ label: 'History' }]}>
        {data && <span className="text-[12.5px] text-muted-foreground">{data.length} comparisons</span>}
      </TopBar>
      <div className="mx-auto w-full max-w-[1200px] px-4 py-6">
        {preset && (
          <div className="mb-3 flex flex-wrap items-center gap-2 text-[12.5px] text-muted-foreground">
            Showing comparisons that used the preset <span className="font-mono text-foreground">{preset}</span>.
            <Link to="/history" className="underline underline-offset-2 hover:text-foreground">Show all</Link>
          </div>
        )}
        <div className="mb-6 flex flex-wrap items-center gap-2">
          <div className="relative w-full sm:w-72">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-dim" />
            <Input id="history-search" aria-label="Search the history" className="pl-8" placeholder="Search prompts or projects" value={q} onChange={e => setQ(e.target.value)} />
          </div>
          <Select value={model} onValueChange={setModel}>
            <SelectTrigger aria-label="Filter by model" className="w-44"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All models</SelectItem>
              {usedModels.map(m => <SelectItem key={m} value={m} className="font-mono">{m}</SelectItem>)}
            </SelectContent>
          </Select>
          <Select value={state} onValueChange={v => setState(v as StateFilter)}>
            <SelectTrigger aria-label="Filter by status" className="w-44"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Any status</SelectItem>
              <SelectItem value="report">With report</SelectItem>
              <SelectItem value="no-report">Without report</SelectItem>
              <SelectItem value="errors">With errors or limits</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {isLoading && <LoadingRows rows={5} />}
        {error && <ErrorNote error={error} />}
        {data && filtered.length === 0 && (
          <div className="border border-dashed p-8 text-center text-muted-foreground">
            {data.length === 0 ? 'No finished comparisons yet. They will appear here once the first one ends.' : 'No comparisons match these filters.'}
          </div>
        )}

        {groupByDay(filtered).map(g => (
          <section key={g.title} className="mb-8">
            <h2 className="mb-2 flex items-baseline gap-2 text-[15px] font-semibold">
              {g.title}
              {g.subtitle && <span className="text-[13px] font-normal text-dim">{g.subtitle}</span>}
            </h2>
            <ul className="divide-y overflow-hidden border">
              {g.items.map(c => {
                const win = cheaper(c)
                const failed = (['A', 'B'] as const).filter(s => ['error', 'limit_reached'].includes(c.sides[s].status))
                return (
                  <li key={c.id}>
                    <button
                      onClick={() => navigate({ to: '/history/$id', params: { id: c.id } })}
                      className="grid w-full gap-x-6 gap-y-2 bg-panel px-4 py-3.5 text-left transition-colors outline-none hover:bg-raise focus-visible:bg-raise md:grid-cols-[52px_minmax(0,1fr)_190px_150px_110px]"
                    >
                      <span className="tnum font-mono text-[12.5px] text-dim">{formatTime(c.createdAt)}</span>
                      <span className="min-w-0">
                        <span className="block truncate text-sm leading-snug text-foreground">{c.prompt}</span>
                        <span className="mt-0.5 block font-mono text-xs text-dim">
                          {c.projectName} · #{c.id}
                          {c.seriesId && <> · attempt {c.attempt}/{c.seriesSize}</>}
                        </span>
                      </span>
                      <span className="grid content-center gap-1 text-[13px]">
                        {(['A', 'B'] as const).map(s => (
                          <span key={s} className="flex items-center gap-2">
                            <SideTag side={s} small />
                            <span className="truncate font-mono">{c.sides[s].config.model}</span>
                          </span>
                        ))}
                      </span>
                      <span className="tnum grid content-center gap-1 font-mono text-[12.5px]">
                        {(['A', 'B'] as const).map(s => (
                          <span key={s}>
                            <span className={cn(win === s && 'text-ok')}>{formatUsd(c.sides[s].metrics.costUsd)}</span>
                            <span className="text-dim"> · {formatDuration(c.sides[s].metrics.agentSec)}</span>
                          </span>
                        ))}
                      </span>
                      <span className="flex items-center md:justify-end">
                        {failed.length > 0 ? (
                          <Chip tone="danger">{failed.map(s => `${s}: ${c.sides[s].status === 'error' ? 'error' : 'limit'}`).join(' · ')}</Chip>
                        ) : c.report === 'ready' ? (
                          <Chip tone="ok"><Check className="size-3" />report</Chip>
                        ) : (
                          <Chip>no report</Chip>
                        )}
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </section>
        ))}
      </div>
    </>
  )
}
