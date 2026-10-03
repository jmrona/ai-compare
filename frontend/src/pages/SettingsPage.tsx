import { Trash2 } from 'lucide-react'
import type { Limits, Settings } from '@/api/types'
import { useCatalog, useSettings, useUpdateSettings } from '@/api/queries'
import { agentModels } from '@/lib/catalog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, Field, LoadingRows, Panel } from '@/components/common/primitives'
import { LimitRow } from '@/components/compare/SideForm'

export function SettingsPage() {
  const { data: settings, error, isLoading } = useSettings()
  const { data: catalog } = useCatalog()
  const update = useUpdateSettings()

  if (isLoading) return <div className="p-4"><LoadingRows rows={6} /></div>
  if (error || !settings) return <div className="p-4"><ErrorNote error={error} /></div>

  const save = (patch: Partial<Settings>) => update.mutate(patch)
  const setLimit = (k: keyof Limits, v: number | null) => save({ defaultLimits: { ...settings.defaultLimits, [k]: v } })
  const totalGb = settings.disk.reduce((sum, d) => sum + d.gb, 0)

  return (
    <>
      <TopBar crumbs={[{ label: 'Settings' }]}>
        {update.isPending && <span className="text-xs text-muted-foreground">Saving…</span>}
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1100px] gap-3 px-4 py-6 lg:grid-cols-2">
        <Panel title="API keys · .env">
          <div className="grid gap-1.5 font-mono text-[12.5px]">
            {([['OPENAI_API_KEY', settings.keys.openai, false], ['ANTHROPIC_API_KEY', settings.keys.anthropic, true]] as const).map(([name, set, phase2]) => (
              <div key={name} className="flex items-center justify-between gap-2 border bg-term px-3 py-2">
                <span>{name}</span>
                <span className="flex items-center gap-2">
                  {phase2 ? <Chip>phase 2</Chip> : <Chip tone={set ? 'ok' : 'warn'}>{set ? 'set' : 'missing'}</Chip>}
                  {set && <Button size="sm" variant="ghost">Test</Button>}
                </span>
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-dim">Keys never enter the agent containers: the proxy adds them to each request. Edit them in the .env file at the repo root.</p>
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
            <Field label="Report model" htmlFor="report-model">
              <Select value={settings.reportModel} onValueChange={v => save({ reportModel: v })}>
                <SelectTrigger id="report-model" className="w-full font-mono"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {agentModels(catalog, 'openai').map(m => <SelectItem key={m.id} value={m.id} className="font-mono">{m.id}</SelectItem>)}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-center gap-3">
              <Switch id="auto-report" checked={settings.autoReport} onCheckedChange={v => save({ autoReport: v })} />
              <Label htmlFor="auto-report" className="text-[13px] font-normal">Generate automatically when a comparison ends</Label>
            </div>
          </div>
        </Panel>

        <Panel title="Resources per side">
          <div className="grid grid-cols-2 gap-3">
            <Field label="CPUs" htmlFor="resources-cpus">
              <Input id="resources-cpus" className="tnum font-mono" type="number" min={1} value={settings.resources.cpus} onChange={e => save({ resources: { ...settings.resources, cpus: Number(e.target.value) } })} />
            </Field>
            <Field label="Memory (GB)" htmlFor="resources-memory">
              <Input id="resources-memory" className="tnum font-mono" type="number" min={1} value={settings.resources.memoryGb} onChange={e => save({ resources: { ...settings.resources, memoryGb: Number(e.target.value) } })} />
            </Field>
          </div>
          <p className="mt-2 text-xs text-dim">The same on both sides. The GPU used by local models cannot be split.</p>
        </Panel>

        <Panel title="Local models" right={<Chip>phase 2</Chip>}>
          <div className="flex gap-2">
            <Input id="local-url" aria-label="Local model server URL" className="font-mono" disabled value={settings.localBaseUrl} readOnly />
            <Button variant="outline" disabled>Test</Button>
          </div>
          <p className="mt-2 text-xs text-dim">Ollama, LM Studio, llama.cpp or vLLM on your machine. The model list comes from the server itself.</p>
        </Panel>

        <Panel title="CLI versions">
          <div className="grid gap-1.5 font-mono text-[12.5px]">
            {settings.cliVersions.map(v => (
              <div key={v.cli} className="flex items-center justify-between border bg-term px-3 py-2">
                <span>{v.cli}</span>
                {v.pinned == null ? (
                  <Chip>phase 2</Chip>
                ) : (
                  <span className="tnum flex items-center gap-2">
                    {v.pinned}
                    {v.latest && v.latest !== v.pinned && <Button size="sm" variant="outline">Update to {v.latest}</Button>}
                  </span>
                )}
              </div>
            ))}
          </div>
        </Panel>

        <Panel title="Retention and disk" className="lg:col-span-2">
          <div className="grid gap-6 md:grid-cols-2">
            <div className="grid content-start gap-3">
              <div className="flex items-center gap-3">
                <Switch
                  id="keep-stopped"
                  checked={settings.retention.keepStopped}
                  onCheckedChange={v => save({ retention: { ...settings.retention, keepStopped: v } })}
                />
                <Label htmlFor="keep-stopped" className="text-[13px] font-normal">Keep stopped containers</Label>
              </div>
              <div className="grid grid-cols-3 gap-3">
                <Field label="Containers (days)" htmlFor="retention-containers">
                  <Input id="retention-containers" className="tnum font-mono" disabled={!settings.retention.keepStopped} defaultValue={settings.retention.containersDays} />
                </Field>
                <Field label="Images (days)" htmlFor="retention-images">
                  <Input id="retention-images" className="tnum font-mono" defaultValue={settings.retention.imagesDays} />
                </Field>
                <Field label="Recordings (days)" htmlFor="retention-recordings">
                  <Input id="retention-recordings" className="tnum font-mono" defaultValue={settings.retention.recordingsDays} />
                </Field>
              </div>
              <div><Button size="sm" variant="destructive"><Trash2 className="size-3.5" />Clean up now</Button></div>
            </div>
            <div className="grid content-start gap-2 font-mono text-[12.5px]">
              {settings.disk.map(d => (
                <div key={d.label}>
                  <div className="flex justify-between"><span className="text-muted-foreground">{d.label}</span><span className="tnum">{d.gb.toFixed(1)} GB</span></div>
                  <div className="mt-1 h-1 bg-raise"><div className="h-1 bg-foreground/70" style={{ width: `${(d.gb / totalGb) * 100}%` }} /></div>
                </div>
              ))}
              <div className="mt-1 flex justify-between border-t pt-2"><span className="text-muted-foreground">total</span><span className="tnum">{totalGb.toFixed(1)} GB</span></div>
            </div>
          </div>
        </Panel>
      </div>
    </>
  )
}
