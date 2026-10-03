import { useState } from 'react'
import { RefreshCw, Search } from 'lucide-react'
import type { ProviderId } from '@/api/types'
import { useCatalog, useRefreshCatalog } from '@/api/queries'
import { formatDateTime, formatPrice } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TopBar } from '@/components/app/AppShell'
import { ErrorNote, LoadingRows, Segmented } from '@/components/common/primitives'

export function PricingPage() {
  const { data, error, isLoading } = useCatalog()
  const refresh = useRefreshCatalog()
  const [provider, setProvider] = useState<ProviderId>('openai')
  const [q, setQ] = useState('')
  const models = data?.models.filter(m => m.provider === provider && m.id.includes(q.trim().toLowerCase())) ?? []

  return (
    <>
      <TopBar crumbs={[{ label: 'Pricing' }]}>
        {data && (
          <span className="font-mono text-xs text-muted-foreground">
            models.dev · {data.fromCache ? 'cached' : 'refreshed'} {formatDateTime(data.fetchedAt)}
          </span>
        )}
        <Button size="sm" variant="outline" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
          <RefreshCw className={refresh.isPending ? 'size-3.5 animate-spin' : 'size-3.5'} />
          {refresh.isPending ? 'Refreshing…' : 'Refresh'}
        </Button>
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1100px] gap-4 px-4 py-6">
        <p className="max-w-[72ch] text-[13px] leading-relaxed text-muted-foreground">
          Prices from models.dev, in USD per million tokens. Read-only: each comparison keeps a snapshot of the prices it used when it started.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Segmented
            label="Provider"
            value={provider}
            onChange={setProvider}
            options={[
              { value: 'openai', label: 'OpenAI' },
              { value: 'anthropic', label: 'Anthropic · phase 2', disabled: true },
              { value: 'local', label: 'Local · phase 2', disabled: true },
            ]}
          />
          <div className="relative w-full max-w-xs">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-dim" />
            <Input id="pricing-search" aria-label="Search models" className="pl-8" placeholder="Search models" value={q} onChange={e => setQ(e.target.value)} />
          </div>
          <span className="text-xs text-dim">Sample values while using mock data.</span>
        </div>

        {isLoading && <LoadingRows rows={4} />}
        {error && <ErrorNote error={error} />}
        {refresh.error && <ErrorNote error={refresh.error} />}

        {data && (
          <div className="overflow-x-auto border">
            <table className="w-full min-w-[720px] text-[13px]">
              <thead>
                <tr className="border-b bg-panel text-left text-[10.5px] tracking-[0.07em] text-dim uppercase">
                  <th className="px-4 py-2.5 font-normal">model</th>
                  <th className="px-4 py-2.5 text-right font-normal">input</th>
                  <th className="px-4 py-2.5 text-right font-normal">cached input</th>
                  <th className="px-4 py-2.5 text-right font-normal">cache write</th>
                  <th className="px-4 py-2.5 text-right font-normal">output</th>
                  <th className="px-4 py-2.5 text-right font-normal">context</th>
                </tr>
              </thead>
              <tbody className="tnum font-mono">
                {models.map(m => (
                  <tr key={m.id} className="border-b last:border-0">
                    <td className="px-4 py-2.5">{m.id}</td>
                    {m.price ? (
                      <>
                        <td className="px-4 py-2.5 text-right">{formatPrice(m.price.input)}</td>
                        <td className="px-4 py-2.5 text-right">{m.price.cacheRead == null ? '—' : formatPrice(m.price.cacheRead)}</td>
                        <td className="px-4 py-2.5 text-right text-dim" title={m.price.cacheWrite == null ? 'Not charged by this provider' : undefined}>
                          {m.price.cacheWrite == null ? '—' : formatPrice(m.price.cacheWrite)}
                        </td>
                        <td className="px-4 py-2.5 text-right">{formatPrice(m.price.output)}</td>
                      </>
                    ) : (
                      <td colSpan={4} className="px-4 py-2.5 text-right font-sans text-[12.5px] text-warn">
                        models.dev publishes no price · cost will show as n/a
                      </td>
                    )}
                    <td className="px-4 py-2.5 text-right text-muted-foreground">{m.contextK}k</td>
                  </tr>
                ))}
                {models.length === 0 && (
                  <tr><td colSpan={6} className="px-4 py-6 text-center font-sans text-muted-foreground">No models match “{q}”.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        <p className="max-w-[72ch] text-[12.5px] leading-relaxed text-dim">
          There is no such thing as “cached output”: the fourth category is the cache write, which Anthropic charges for and OpenAI does not (—). Reasoning tokens are billed as output. Local models will appear as “local · no cost”.
        </p>
      </div>
    </>
  )
}
