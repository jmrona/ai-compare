import { AlertTriangle } from 'lucide-react'
import type { Catalog, Cli, Effort, Limits, ProviderId, SideConfig, SideKey } from '@/api/types'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Field, Segmented, SideTag } from '@/components/common/primitives'

const CLIS: { value: Cli; label: string; phase2?: boolean }[] = [
  { value: 'opencode', label: 'opencode' },
  { value: 'codex', label: 'codex', phase2: true },
  { value: 'claude', label: 'claude', phase2: true },
]
const PROVIDERS: { value: ProviderId; label: string; phase2?: boolean }[] = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic', phase2: true },
  { value: 'local', label: 'Local (Ollama, LM Studio)', phase2: true },
]
const EFFORTS: Effort[] = ['minimal', 'low', 'medium', 'high']

export function SideForm({ side, value, onChange, catalog, suggested }: {
  side: SideKey
  value: SideConfig
  onChange: (v: SideConfig) => void
  catalog: Catalog | undefined
  suggested: { timeoutMin: number; maxTokensK: number; maxCostUsd: number }
}) {
  const id = (k: string) => `side-${side}-${k}`
  const set = <K extends keyof SideConfig>(k: K, v: SideConfig[K]) => onChange({ ...value, [k]: v })
  const models = catalog?.models.filter(m => m.provider === value.provider) ?? []
  const model = models.find(m => m.id === value.model)
  const setLimit = (k: keyof Limits, v: number | null) => set('limits', { ...value.limits, [k]: v })

  return (
    <div className="grid min-w-0 content-start gap-3 bg-panel p-3">
      <div className="flex items-center gap-2">
        <SideTag side={side} />
        <span className="font-medium">Side {side}</span>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="CLI" htmlFor={id('cli')}>
          <Select value={value.cli} onValueChange={v => set('cli', v as Cli)}>
            <SelectTrigger id={id('cli')} className="w-full font-mono"><SelectValue /></SelectTrigger>
            <SelectContent>
              {CLIS.map(c => (
                <SelectItem key={c.value} value={c.value} disabled={c.phase2} className="font-mono">
                  {c.label}{c.phase2 && <span className="ml-auto text-[10px] text-dim">phase 2</span>}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="Provider" htmlFor={id('provider')}>
          <Select value={value.provider} onValueChange={v => set('provider', v as ProviderId)}>
            <SelectTrigger id={id('provider')} className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {PROVIDERS.map(p => (
                <SelectItem key={p.value} value={p.value} disabled={p.phase2}>
                  {p.label}{p.phase2 && <span className="ml-auto text-[10px] text-dim">phase 2</span>}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="Model · models.dev" htmlFor={id('model')}>
          <Select value={value.model} onValueChange={v => set('model', v)}>
            <SelectTrigger id={id('model')} className="w-full font-mono"><SelectValue placeholder="Loading models…" /></SelectTrigger>
            <SelectContent>
              {models.map(m => (
                <SelectItem key={m.id} value={m.id} className="font-mono">{m.id}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="Effort" htmlFor={id('effort')}>
          <Select value={value.effort} onValueChange={v => set('effort', v as Effort)}>
            <SelectTrigger id={id('effort')} className="w-full font-mono"><SelectValue /></SelectTrigger>
            <SelectContent>
              {EFFORTS.map(e => <SelectItem key={e} value={e} className="font-mono">{e}</SelectItem>)}
            </SelectContent>
          </Select>
        </Field>
      </div>

      {model && !model.price && (
        <div className="flex gap-2 border border-warn/30 bg-warn/5 p-2.5 text-[12.5px] text-warn">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>models.dev has no price for {model.id}. Tokens will still be measured, but the cost will show as n/a.</span>
        </div>
      )}

      <Field label="Mode">
        <Segmented
          label={`Side ${side} mode`}
          value={value.mode}
          onChange={v => set('mode', v)}
          options={[{ value: 'interactive', label: 'Interactive' }, { value: 'autonomous', label: 'Autonomous' }]}
        />
      </Field>

      <div className="grid gap-2 border-t pt-3">
        <span className="text-[11px] tracking-[0.07em] text-dim uppercase">Limits · optional</span>
        <LimitRow id={id('timeout')} label="Timeout" unit="min" value={value.limits.timeoutMin} suggested={suggested.timeoutMin} onChange={v => setLimit('timeoutMin', v)} />
        <LimitRow id={id('tokens')} label="Tokens" unit="k" value={value.limits.maxTokensK} suggested={suggested.maxTokensK} onChange={v => setLimit('maxTokensK', v)} />
        {value.provider !== 'local' && (
          <LimitRow id={id('cost')} label="Cost" prefix="$" value={value.limits.maxCostUsd} suggested={suggested.maxCostUsd} onChange={v => setLimit('maxCostUsd', v)} />
        )}
      </div>
    </div>
  )
}

/** A limit with a switch: off means no limit. */
export function LimitRow({ id, label, unit, prefix, value, suggested, onChange }: {
  id: string
  label: string
  unit?: string
  prefix?: string
  value: number | null
  suggested: number
  onChange: (v: number | null) => void
}) {
  const on = value != null
  return (
    <div className="flex min-h-8 items-center gap-3">
      <Switch id={id} checked={on} onCheckedChange={c => onChange(c ? suggested : null)} aria-label={`${label} limit`} />
      <label htmlFor={id} className="w-20 text-[13px]">{label}</label>
      {on ? (
        <div className="relative w-28">
          {prefix && <span className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-dim">{prefix}</span>}
          <Input
            id={id + '-value'}
            inputMode="decimal"
            aria-label={`Maximum ${label.toLowerCase()}`}
            className={`tnum font-mono ${prefix ? 'pl-6' : ''} ${unit ? 'pr-10' : ''}`}
            value={value}
            onChange={e => {
              const n = Number(e.target.value)
              if (!Number.isNaN(n)) onChange(n)
            }}
          />
          {unit && <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-dim">{unit}</span>}
        </div>
      ) : (
        <span className="text-[12.5px] text-dim">no limit</span>
      )}
    </div>
  )
}
