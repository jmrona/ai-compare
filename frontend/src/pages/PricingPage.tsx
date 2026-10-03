import { Fragment, useState } from 'react'
import { AlertTriangle, RefreshCw, Search } from 'lucide-react'
import type { ModelInfo, Price, ProviderId } from '@/api/types'
import { useCatalog, useRefreshCatalog } from '@/api/queries'
import { formatRelease } from '@/lib/catalog'
import { formatDateTime, formatInt, formatPrice } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, LoadingRows, Segmented } from '@/components/common/primitives'

const rate = (n: number | null) => (n == null ? '—' : formatPrice(n))

function PriceCells({ price, muted }: { price: Price; muted?: boolean }) {
  const cls = cn('px-4 py-2.5 text-right', muted && 'text-muted-foreground')
  return (
    <>
      <td className={cls}>{formatPrice(price.input)}</td>
      <td className={cls}>{rate(price.cacheRead)}</td>
      <td className={cn(cls, price.cacheWrite == null && 'text-dim')} title={price.cacheWrite == null ? 'Not charged by this provider' : undefined}>
        {rate(price.cacheWrite)}
      </td>
      <td className={cls}>{formatPrice(price.output)}</td>
    </>
  )
}

export function PricingPage() {
  const { data, error, isLoading } = useCatalog()
  const refresh = useRefreshCatalog()
  const [provider, setProvider] = useState<ProviderId>('openai')
  const [showDeprecated, setShowDeprecated] = useState(false)
  const [showNonAgent, setShowNonAgent] = useState(false)
  const [q, setQ] = useState('')

  const text = q.trim().toLowerCase()
  const ofProvider = data?.models.filter(m => m.provider === provider) ?? []
  const hidden = { deprecated: ofProvider.filter(m => m.deprecated).length, nonAgent: ofProvider.filter(m => !m.toolCall || !m.textOutput).length }
  const models = ofProvider.filter(
    (m: ModelInfo) =>
      (showDeprecated || !m.deprecated) &&
      (showNonAgent || (m.toolCall && m.textOutput)) &&
      (!text || m.id.includes(text) || m.name.toLowerCase().includes(text)),
  )

  return (
    <>
      <TopBar crumbs={[{ label: 'Pricing' }]}>
        {data && (
          <span className="font-mono text-xs text-muted-foreground">
            models.dev · {data.fromCache ? 'saved copy from' : 'checked'} {formatDateTime(data.fetchedAt)}
          </span>
        )}
        <Button size="sm" variant="outline" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
          <RefreshCw className={refresh.isPending ? 'size-3.5 animate-spin' : 'size-3.5'} />
          {refresh.isPending ? 'Checking…' : 'Check for updates'}
        </Button>
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1200px] gap-4 px-4 py-6">
        <p className="max-w-[78ch] text-[13px] leading-relaxed text-muted-foreground">
          Prices from models.dev in USD per million tokens, newest release first. Read-only: each comparison keeps a snapshot of the prices it used when it started.
        </p>

        {data?.warning && (
          <div role="status" className="flex gap-2 border border-warn/30 bg-warn/5 p-3 text-[12.5px] text-warn">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
            {data.warning}
          </div>
        )}

        <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
          <Segmented
            label="Provider"
            value={provider}
            onChange={setProvider}
            options={[
              { value: 'openai', label: 'OpenAI' },
              { value: 'anthropic', label: 'Anthropic' },
              { value: 'local', label: 'Local · phase 2', disabled: true },
            ]}
          />
          <div className="relative w-full max-w-xs">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-dim" />
            <Input id="pricing-search" aria-label="Search models" className="pl-8" placeholder="Search models" value={q} onChange={e => setQ(e.target.value)} />
          </div>
          <div className="flex items-center gap-2">
            <Switch id="show-deprecated" checked={showDeprecated} onCheckedChange={setShowDeprecated} />
            <Label htmlFor="show-deprecated" className="text-[12.5px] font-normal">Show deprecated ({hidden.deprecated})</Label>
          </div>
          <div className="flex items-center gap-2">
            <Switch id="show-non-agent" checked={showNonAgent} onCheckedChange={setShowNonAgent} />
            <Label htmlFor="show-non-agent" className="text-[12.5px] font-normal">Show models agents cannot use ({hidden.nonAgent})</Label>
          </div>
        </div>

        {isLoading && <LoadingRows rows={6} />}
        {error && <ErrorNote error={error} />}
        {refresh.error && <ErrorNote error={refresh.error} />}

        {data && (
          <div className="overflow-x-auto border">
            <table className="w-full min-w-[860px] text-[13px]">
              <thead>
                <tr className="border-b bg-panel text-left text-[10.5px] tracking-[0.07em] text-dim uppercase">
                  <th className="px-4 py-2.5 font-normal">model</th>
                  <th className="px-4 py-2.5 font-normal">released</th>
                  <th className="px-4 py-2.5 text-right font-normal">input</th>
                  <th className="px-4 py-2.5 text-right font-normal">cached input</th>
                  <th className="px-4 py-2.5 text-right font-normal">cache write</th>
                  <th className="px-4 py-2.5 text-right font-normal">output</th>
                  <th className="px-4 py-2.5 text-right font-normal">context</th>
                </tr>
              </thead>
              <tbody className="tnum font-mono">
                {models.map(m => (
                  <Fragment key={m.id}>
                    <tr className={cn('border-b last:border-0', m.deprecated && 'text-muted-foreground')}>
                      <td className="px-4 py-2.5">
                        <div className="flex flex-wrap items-center gap-2">
                          {m.id}
                          {m.deprecated && <Chip tone="warn">deprecated</Chip>}
                          {!(m.toolCall && m.textOutput) && <Chip>not for agents</Chip>}
                        </div>
                        <div className="font-sans text-xs text-dim">{m.name}</div>
                      </td>
                      <td className="px-4 py-2.5 whitespace-nowrap">{formatRelease(m.releaseDate)}</td>
                      {m.price ? (
                        <PriceCells price={m.price} />
                      ) : (
                        <td colSpan={4} className="px-4 py-2.5 text-right font-sans text-[12.5px] text-warn">models.dev publishes no price · cost will show as n/a</td>
                      )}
                      <td className="px-4 py-2.5 text-right text-muted-foreground">{m.contextK >= 1000 ? `${(m.contextK / 1000).toFixed(2).replace(/\.?0+$/, '')}M` : `${m.contextK}k`}</td>
                    </tr>
                    {m.longContext && (
                      <tr className="border-b bg-panel/50 text-xs">
                        <td colSpan={2} className="px-4 py-2 pl-8 font-sans text-dim">↳ prompts over {formatInt(m.longContext.aboveTokens)} tokens</td>
                        <PriceCells price={m.longContext.price} muted />
                        <td />
                      </tr>
                    )}
                  </Fragment>
                ))}
                {models.length === 0 && (
                  <tr><td colSpan={7} className="px-4 py-6 text-center font-sans text-muted-foreground">No models match these filters.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        <p className="max-w-[78ch] text-[12.5px] leading-relaxed text-dim">
          There is no such thing as “cached output”: the fourth category is the cache write. A dash means the provider does not charge it. Reasoning tokens are billed as output. Local models will appear as “local · no cost”.
        </p>
      </div>
    </>
  )
}
