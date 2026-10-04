import { useState } from 'react'
import { Trash2 } from 'lucide-react'
import type { Limits, Retention, Settings } from '@/api/types'
import { useCatalog, useCleanUp, useSettings, useUpdateSettings } from '@/api/queries'
import { agentModels } from '@/lib/catalog'
import { formatBytes } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, Field, LoadingRows, Panel } from '@/components/common/primitives'
import { LimitRow } from '@/components/compare/SideForm'

const RETENTION_TARGETS: { key: keyof Retention; label: string; hint: string }[] = [
  { key: 'containers', label: 'Containers', hint: 'Stopped agent, test and preview containers.' },
  { key: 'images', label: 'Images', hint: 'Side and result images. An image still used by a kept container stays.' },
  { key: 'projectCopies', label: 'Project copies', hint: 'The copies of your project in staging; Run again copies the folder anew.' },
  { key: 'artefacts', label: 'Artefacts', hint: 'Downloads, recordings, diffs, test output and harness snapshots shown in the history.' },
]

export function SettingsPage() {
  const { data: settings, error, isLoading } = useSettings()
  const { data: catalog } = useCatalog()
  const update = useUpdateSettings()
  const cleanUp = useCleanUp()

  if (isLoading) return <div className="p-4"><LoadingRows rows={6} /></div>
  if (error || !settings) return <div className="p-4"><ErrorNote error={error} /></div>

  const save = (patch: Partial<Settings>) => update.mutate(patch)
  const setLimit = (k: keyof Limits, v: number | null) => save({ defaultLimits: { ...settings.defaultLimits, [k]: v } })
  const totalBytes = settings.disk.reduce((sum, d) => sum + d.bytes, 0)
  const reportModels = agentModels(catalog, 'openai')

  return (
    <>
      <TopBar crumbs={[{ label: 'Settings' }]}>
        {update.isPending && <span className="text-xs text-muted-foreground">Saving…</span>}
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1200px] gap-3 px-4 py-6 lg:grid-cols-2">
        {update.error && <div className="lg:col-span-2"><ErrorNote error={update.error} /></div>}

        <Panel title="API keys · .env">
          <div className="grid gap-1.5 font-mono text-[12.5px]">
            {([['OPENAI_API_KEY', settings.keys.openai], ['ANTHROPIC_API_KEY', settings.keys.anthropic]] as const).map(([name, set]) => (
              <div key={name} className="flex items-center justify-between gap-2 border bg-term px-3 py-2">
                <span>{name}</span>
                <Chip tone={set ? 'ok' : 'warn'}>{set ? 'set' : 'missing'}</Chip>
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-dim">Keys never enter the agent containers: the proxy adds them to each request. Edit them in the .env file at the repo root and restart with docker compose up -d.</p>
        </Panel>

        <Panel title="Default limits · optional">
          <div className="grid gap-2">
            <LimitRow id="default-timeout" label="Timeout" unit="min" value={settings.defaultLimits.timeoutMin} suggested={settings.suggestedLimits.timeoutMin} onChange={v => setLimit('timeoutMin', v)} />
            <LimitRow id="default-tokens" label="Tokens" unit="k" value={settings.defaultLimits.maxTokensK} suggested={settings.suggestedLimits.maxTokensK} onChange={v => setLimit('maxTokensK', v)} />
            <LimitRow id="default-cost" label="Cost" prefix="$" value={settings.defaultLimits.maxCostUsd} suggested={settings.suggestedLimits.maxCostUsd} onChange={v => setLimit('maxCostUsd', v)} />
          </div>
          <p className="mt-2 text-xs text-dim">Off by default. They are pre-filled on each side and can be changed per comparison.</p>
        </Panel>

        <Panel title="Report">
          <div className="grid gap-3">
            <Field label="Report model" htmlFor="report-model" hint="Writes the blind review, the analysis of each side and the judgement. Its cost is recorded apart.">
              <Select value={settings.reportModel} onValueChange={v => save({ reportModel: v })}>
                <SelectTrigger id="report-model" className="w-full font-mono"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {!reportModels.some(m => m.id === settings.reportModel) && <SelectItem value={settings.reportModel} className="font-mono">{settings.reportModel}</SelectItem>}
                  {reportModels.map(m => <SelectItem key={m.id} value={m.id} className="font-mono">{m.id}</SelectItem>)}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-center gap-3">
              <Switch id="auto-report" checked={settings.autoReport} onCheckedChange={v => save({ autoReport: v })} />
              <Label htmlFor="auto-report" className="text-[13px] font-normal">Generate automatically; each side's review starts as soon as it ends</Label>
            </div>
          </div>
        </Panel>

        <Panel title="Resources per side">
          <div className="grid grid-cols-2 gap-3">
            <Field label="CPUs" htmlFor="resources-cpus">
              <NumberInput id="resources-cpus" min={0.5} step={0.5} value={settings.resources.cpus} onCommit={v => save({ resources: { ...settings.resources, cpus: v } })} />
            </Field>
            <Field label="Memory (GB)" htmlFor="resources-memory">
              <NumberInput id="resources-memory" min={1} value={settings.resources.memoryGb} onCommit={v => save({ resources: { ...settings.resources, memoryGb: v } })} />
            </Field>
          </div>
          <p className="mt-2 text-xs text-dim">The same on both sides; applies to new comparisons. The GPU used by local models cannot be split.</p>
        </Panel>

        <Panel title="Local models" right={<Chip>phase 3</Chip>}>
          <Input id="local-url" aria-label="Local model server URL" className="font-mono" disabled value={settings.localBaseUrl} readOnly />
          <p className="mt-2 text-xs text-dim">Ollama, LM Studio, llama.cpp or vLLM on your machine (LOCAL_MODELS_BASE_URL in .env). The model list will come from the server itself.</p>
        </Panel>

        <Panel title="CLI versions">
          <div className="grid gap-1.5 font-mono text-[12.5px]">
            {settings.cliVersions.map(v => (
              <div key={v.cli} className="flex items-center justify-between border bg-term px-3 py-2">
                <span>{v.cli}</span>
                {v.pinned == null ? (
                  <Chip>phase 3</Chip>
                ) : (
                  <span className="tnum flex items-center gap-2">
                    {v.pinned}
                    {v.latest && v.latest !== v.pinned && <Chip tone="warn">{v.latest} available</Chip>}
                  </span>
                )}
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-dim">Pinned so that comparisons on different days run the same CLI. Changing it is a code change.</p>
        </Panel>

        <Panel title="Retention and disk" className="lg:col-span-2">
          <div className="grid gap-6 md:grid-cols-2">
            <div className="grid content-start gap-3">
              <div className="grid grid-cols-2 gap-3">
                <Field label="Ended more than · days" htmlFor="retention-days">
                  <NumberInput id="retention-days" min={0} max={365} value={settings.retentionDays} onCommit={v => save({ retentionDays: Math.round(v) })} />
                </Field>
                <Field label="and hours ago" htmlFor="retention-hours">
                  <NumberInput id="retention-hours" min={0} max={23} value={settings.retentionHours} onCommit={v => save({ retentionHours: Math.round(v) })} />
                </Field>
              </div>
              {settings.retentionDays === 0 && settings.retentionHours === 0 && (
                <p role="alert" className="border border-warn/30 bg-warn/10 px-3 py-2 text-xs text-warn">
                  With 0 days and 0 hours, every comparison that is not running loses what is selected below at the next clean-up (within the hour, or now with Clean up now),
                  including the containers and images of comparisons that have just ended.
                </p>
              )}
              <fieldset className="grid gap-2">
                <legend className="mb-1.5 text-xs text-muted-foreground">What to remove</legend>
                {RETENTION_TARGETS.map(t => (
                  <Label key={t.key} className="flex items-start gap-2 font-normal">
                    <Checkbox
                      className="mt-0.5"
                      checked={settings.retention[t.key]}
                      onCheckedChange={v => save({ retention: { ...settings.retention, [t.key]: v === true } })}
                    />
                    <span className="grid gap-0.5">
                      <span className="text-[13px]">{t.label}</span>
                      <span className="text-xs text-dim">{t.hint}</span>
                    </span>
                  </Label>
                ))}
              </fieldset>
              {settings.retention.artefacts && (
                <p className="border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                  Removing artefacts cannot be undone: the history keeps its entries and reports, but their files can no longer be downloaded,
                  and the terminal recordings, changes, tests and harness of those sides become empty.
                </p>
              )}
              <p className="text-xs text-dim">Only what ai-compare created is removed, nothing else on your Docker. Reports and the history are always kept.</p>
              <div className="flex flex-wrap items-center gap-2">
                <Button size="sm" variant="destructive" disabled={cleanUp.isPending || !Object.values(settings.retention).some(Boolean)} onClick={() => cleanUp.mutate()}>
                  <Trash2 className="size-3.5" />{cleanUp.isPending ? 'Cleaning up…' : 'Clean up now'}
                </Button>
                {cleanUp.data && (
                  <span className="text-xs text-muted-foreground">
                    {cleanUp.data.comparisons} comparisons · {cleanUp.data.containers} containers · {cleanUp.data.images} images · {cleanUp.data.projectCopies} project copies · {cleanUp.data.artefacts} artefact folders removed
                  </span>
                )}
              </div>
              {cleanUp.error && <ErrorNote error={cleanUp.error} />}
            </div>
            <div className="grid content-start gap-2 font-mono text-[12.5px]">
              {settings.disk.map(d => (
                <div key={d.label}>
                  <div className="flex justify-between"><span className="text-muted-foreground">{d.label}</span><span className="tnum">{formatBytes(d.bytes)}</span></div>
                  <div className="mt-1 h-1 bg-raise"><div className="h-1 bg-foreground/70" style={{ width: `${totalBytes ? (d.bytes / totalBytes) * 100 : 0}%` }} /></div>
                </div>
              ))}
              <div className="mt-1 flex justify-between border-t pt-2"><span className="text-muted-foreground">total (estimate)</span><span className="tnum">{formatBytes(totalBytes)}</span></div>
            </div>
          </div>
        </Panel>
      </div>
    </>
  )
}

/** A number field that saves when it loses focus or Enter is pressed, not on every keystroke. */
function NumberInput({ id, value, min, max, step, onCommit }: { id: string; value: number; min: number; max?: number; step?: number; onCommit: (v: number) => void }) {
  const [text, setText] = useState(String(value))
  const [seen, setSeen] = useState(value)
  if (value !== seen) {
    // The saved value changed (or was refused and reset): show it.
    setSeen(value)
    setText(String(value))
  }
  const commit = () => {
    const v = Number(text)
    if (Number.isFinite(v) && v >= min && (max == null || v <= max) && v !== value) onCommit(v)
    else setText(String(value))
  }
  return (
    <Input
      id={id}
      className="tnum font-mono"
      type="number"
      min={min}
      max={max}
      step={step}
      value={text}
      onChange={e => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={e => e.key === 'Enter' && commit()}
    />
  )
}
