import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ArrowLeftRight, Check, ChevronDown, ChevronRight, Copy, FolderOpen, Play } from 'lucide-react'
import type { Catalog, Comparison, Limits, ModelInfo, ProjectProfile, Settings, SideConfig, SideKey } from '@/api/types'
import { PROJECT_HARNESS } from '@/api/types'
import { agentModels, pickEffort } from '@/lib/catalog'
import { useActiveComparison, useCatalog, useComparison, useInspectProject, usePresets, useSettings, useStartComparison } from '@/api/queries'
import { formatBytes, formatInt } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { TopBar } from '@/components/app/AppShell'
import { Chip, Dot, ErrorNote, Field, LoadingRows, Panel, Segmented } from '@/components/common/primitives'
import { SideForm } from '@/components/compare/SideForm'
import { FolderBrowser } from '@/components/compare/FolderBrowser'

const EXAMPLE_PATH = /Windows/.test(navigator.userAgent) ? 'C:\\Users\\me\\projects\\my-app' : '/Users/me/projects/my-app'

/** Profile for a comparison without a project: the default runtime and no commands. */
const EMPTY_PROFILE: ProjectProfile = { runtime: 'node:22-bookworm-slim', setup: '', test: '', hiddenTestsPath: '', previewCommand: '', previewPort: null }

const isAbsolutePath = (p: string) => /^[a-zA-Z]:[\\/]/.test(p) || p.startsWith('/')

type Source = 'copy' | 'empty'

/** A side preset to a model (or empty if the catalogue has none for the provider). */
const baseSide = (model: ModelInfo | undefined, mode: SideConfig['mode'], limits: Limits): SideConfig => ({
  cli: 'opencode',
  provider: 'openai',
  model: model?.id ?? '',
  effort: pickEffort(model),
  mode,
  limits: { ...limits },
  harness: PROJECT_HARNESS,
})

export function NewComparisonPage() {
  // The form starts from the default limits in Settings and the newest models in the catalogue, so it waits for both,
  // and, for "Run again", from an earlier comparison.
  const { from } = useSearch({ from: '/' })
  const settings = useSettings()
  const catalog = useCatalog()
  const earlier = useComparison(from ?? '', { enabled: !!from })
  if (settings.isLoading || catalog.isLoading || (from && earlier.isLoading)) return <div className="p-4"><LoadingRows rows={4} /></div>
  if (settings.error || !settings.data) return <div className="p-4"><ErrorNote error={settings.error} /></div>
  if (catalog.error || !catalog.data) {
    return (
      <div className="grid max-w-xl gap-3 p-4">
        <ErrorNote error={catalog.error} />
        <p className="text-sm text-muted-foreground">The model list comes from models.dev through the backend. Check that the backend is running and can reach the internet.</p>
        <Button variant="outline" className="w-fit" onClick={() => catalog.refetch()}>Try again</Button>
      </div>
    )
  }
  // A new key starts a fresh form when "Run again" points at another comparison.
  return <NewComparisonForm key={from ?? 'new'} settings={settings.data} catalog={catalog.data} earlier={earlier.data} />
}

function NewComparisonForm({ settings, catalog, earlier }: { settings: Settings; catalog: Catalog; earlier?: Comparison }) {
  const navigate = useNavigate()
  const { data: active } = useActiveComparison()
  const inspect = useInspectProject()
  const start = useStartComparison()
  const { data: presets } = usePresets()

  const [source, setSource] = useState<Source>(earlier && !earlier.projectPath ? 'empty' : 'copy')
  const [path, setPath] = useState(earlier?.projectPath ?? '')
  const [browsing, setBrowsing] = useState(false)
  // Run again keeps the profile it ran with (it may have been edited), not the detected one.
  const [profile, setProfile] = useState<ProjectProfile | null>(earlier ? earlier.profile : null)
  const [profileOpen, setProfileOpen] = useState(false)
  const [prompt, setPrompt] = useState(earlier?.prompt ?? '')
  const [repetitions, setRepetitions] = useState(earlier?.seriesSize || 1)
  // Side A gets the newest model and side B the next one, so a fresh form compares the two latest releases.
  const newest = agentModels(catalog, 'openai')
  const [sides, setSides] = useState<Record<SideKey, SideConfig>>(
    earlier
      ? { A: structuredClone(earlier.sides.A.config), B: structuredClone(earlier.sides.B.config) }
      : {
          A: baseSide(newest[0], 'autonomous', settings.defaultLimits),
          B: baseSide(newest[1] ?? newest[0], 'autonomous', settings.defaultLimits),
        },
  )
  // The earlier project folder is checked again: it may have changed or moved since.
  useEffect(() => {
    if (earlier?.projectPath) inspect.mutate(earlier.projectPath)
    // Only once, when the form opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const missingModels = (['A', 'B'] as const).filter(k => sides[k].model && !catalog.models.some(m => m.id === sides[k].model))

  const project = source === 'copy' ? inspect.data : undefined
  const ready = source === 'empty' || !!project
  const canRun = ready && prompt.trim().length > 0 && !!sides.A.model && !!sides.B.model && !start.isPending

  const validate = (p = path) => inspect.mutate(p.trim(), { onSuccess: i => setProfile(i.profile) })

  const changeSource = (s: Source) => {
    setSource(s)
    setProfile(s === 'empty' ? EMPTY_PROFILE : inspect.data?.profile ?? null)
  }

  const changePath = (p: string) => {
    setPath(p)
    // The inspection belongs to the old path.
    if (inspect.data || inspect.error) {
      inspect.reset()
      setProfile(null)
    }
  }

  const run = () => {
    if (!ready || !profile) return
    start.mutate(
      { projectPath: project?.path ?? '', profile, prompt: prompt.trim(), sides, repetitions },
      { onSuccess: ({ id }) => navigate({ to: '/comparisons/$id', params: { id } }) },
    )
  }

  const suggested = settings.suggestedLimits

  return (
    <>
      <TopBar crumbs={[{ label: 'Compare' }, { label: 'New comparison' }]} />

      {earlier && (
        <div className="flex flex-wrap items-center gap-2 border-b border-side-a/30 bg-side-a/10 px-4 py-2 text-[12.5px] text-side-a">
          Filled in from comparison #{earlier.id}: same project, prompt and sides. Change anything before running it again.
          {missingModels.length > 0 && <span className="text-warn">Side {missingModels.join(' and ')}'s model is no longer in the catalogue; choose another.</span>}
        </div>
      )}

      {active && (
        <div className="flex flex-wrap items-center gap-2 border-b border-warn/30 bg-warn/10 px-4 py-2 text-[12.5px] text-warn">
          <Dot tone="warn" live />
          Comparison #{active.id} is still running.
          <Link to="/comparisons/$id" params={{ id: active.id }} className="underline underline-offset-2">Go back to it</Link>
        </div>
      )}

      <div className="mx-auto grid w-full max-w-[1200px] gap-3 px-4 py-6">
        <div className="grid content-start gap-3">
          <Panel
            title="Project"
            right={
              <div className="flex items-center gap-2">
                {project && <Chip tone="ok"><Check className="size-3" />valid</Chip>}
                <Segmented<Source>
                  label="Project source"
                  value={source}
                  onChange={changeSource}
                  options={[{ value: 'copy', label: 'Copy a folder' }, { value: 'empty', label: 'Empty folder' }]}
                />
              </div>
            }
          >
            {source === 'empty' && (
              <p className="text-xs text-muted-foreground">Both sides start from an empty folder. Useful for trying models on a task without preparing a project.</p>
            )}
            {source === 'copy' && (
              <>
                <form className="flex gap-2" onSubmit={e => { e.preventDefault(); validate() }}>
                  <Input
                    id="project-path"
                    aria-label="Absolute path to the project"
                    className="font-mono"
                    placeholder={EXAMPLE_PATH}
                    value={path}
                    onChange={e => changePath(e.target.value)}
                  />
                  <Button type="button" variant="outline" onClick={() => setBrowsing(true)}>
                    <FolderOpen className="size-3.5" />Browse…
                  </Button>
                  <Button type="submit" variant="outline" disabled={inspect.isPending || !path.trim()}>
                    {inspect.isPending ? 'Checking…' : 'Check'}
                  </Button>
                </form>
                <FolderBrowser
                  open={browsing}
                  onOpenChange={setBrowsing}
                  startPath={isAbsolutePath(path.trim()) ? path.trim() : ''}
                  onSelect={p => { changePath(p); validate(p) }}
                />
                {inspect.error && <div className="mt-3"><ErrorNote error={inspect.error} /></div>}
                {!project && !inspect.error && (
                  <p className="mt-3 text-xs text-muted-foreground">Absolute path to the folder on your machine, typed or chosen with Browse. It is copied as it is, read-only; the original is never modified.</p>
                )}
              </>
            )}
            {project && (
              <>
                <dl className="mt-3 grid grid-cols-3 gap-3 font-mono text-[12.5px]">
                  <div><dt className="text-[10.5px] tracking-[0.07em] text-dim uppercase">files</dt><dd className="tnum">{formatInt(project.fileCount)}</dd></div>
                  <div><dt className="text-[10.5px] tracking-[0.07em] text-dim uppercase">size</dt><dd className="tnum">{formatBytes(project.sizeBytes)}</dd></div>
                  <div><dt className="text-[10.5px] tracking-[0.07em] text-dim uppercase">git</dt><dd>{project.isGit ? 'yes · .git left out' : 'no'}</dd></div>
                </dl>
                <div className="mt-3 grid gap-1.5 border-t pt-3 text-[12.5px]">
                  <span className="text-[10.5px] tracking-[0.07em] text-dim uppercase">Project harness · copied as is</span>
                  {project.harnessFiles.length === 0 && <span className="text-muted-foreground">None. The agents will run without project instructions.</span>}
                  {project.harnessFiles.map(f => {
                    const read = f.readBy.includes('opencode')
                    return (
                      <div key={f.path} className="flex flex-wrap items-center gap-1.5">
                        <Chip tone={read ? 'ok' : 'dim'} className="font-mono">{f.path}</Chip>
                        <span className={read ? 'text-muted-foreground' : 'text-dim'}>{read ? 'read by opencode' : 'not read by opencode'}</span>
                      </div>
                    )
                  })}
                  {project.excluded.map(e => (
                    <div key={e} className="flex flex-wrap items-center gap-1.5">
                      <Chip tone={e.startsWith('.env') ? 'warn' : 'dim'} className="font-mono">{e}</Chip>
                      <span className="text-muted-foreground">{e.startsWith('.env') ? 'excluded for safety' : 'not copied'}</span>
                    </div>
                  ))}
                </div>
              </>
            )}
          </Panel>

          {profile && (
            <Panel
              title="Project profile"
              bodyClassName={profileOpen ? 'p-3' : 'hidden'}
              right={
                <button
                  type="button"
                  aria-expanded={profileOpen}
                  onClick={() => setProfileOpen(o => !o)}
                  className="flex items-center gap-1 font-mono text-xs text-muted-foreground hover:text-foreground"
                >
                  {profile.runtime} · {profile.setup} · {profile.test}
                  {profileOpen ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
                </button>
              }
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Runtime" htmlFor="profile-runtime">
                  <Input id="profile-runtime" className="font-mono" value={profile.runtime} onChange={e => setProfile({ ...profile, runtime: e.target.value })} />
                </Field>
                <Field label="Setup" htmlFor="profile-setup" hint="Runs inside the image, off the clock.">
                  <Input id="profile-setup" className="font-mono" value={profile.setup} onChange={e => setProfile({ ...profile, setup: e.target.value })} />
                </Field>
                <Field label="Tests" htmlFor="profile-test" hint="Runs at the end, in a fresh container.">
                  <Input id="profile-test" className="font-mono" value={profile.test} onChange={e => setProfile({ ...profile, test: e.target.value })} />
                </Field>
                <Field label="Preview command (optional)" htmlFor="profile-preview" hint="Starts the app for the Preview tab, listening on 0.0.0.0. Empty serves the files as a static site.">
                  <Input id="profile-preview" className="font-mono" placeholder="npm run dev -- --host 0.0.0.0 --port 3000" value={profile.previewCommand} onChange={e => setProfile({ ...profile, previewCommand: e.target.value })} />
                </Field>
                <Field label="Preview port" htmlFor="profile-preview-port" hint="Also passed to the command as PORT.">
                  <Input id="profile-preview-port" className="tnum font-mono" type="number" min={1} max={65535} placeholder="3000" value={profile.previewPort ?? ''} onChange={e => setProfile({ ...profile, previewPort: e.target.value ? Number(e.target.value) : null })} />
                </Field>
                <Field label="Hidden tests (optional)" htmlFor="profile-hidden" hint="A folder the agent never sees.">
                  <Input id="profile-hidden" className="font-mono" placeholder="C:\path\to\hidden-tests" value={profile.hiddenTestsPath} onChange={e => setProfile({ ...profile, hiddenTestsPath: e.target.value })} />
                </Field>
              </div>
            </Panel>
          )}

          <Panel title="Prompt" right={<span className="tnum font-mono text-[11.5px] text-dim">{prompt.length} chars</span>}>
            <Textarea
              id="prompt"
              aria-label="Prompt shared by both sides"
              rows={8}
              className="font-mono leading-relaxed"
              placeholder="Describe the task as you would ask an agent. Both sides get exactly the same prompt."
              value={prompt}
              onChange={e => setPrompt(e.target.value)}
            />
          </Panel>
        </div>

        <div className="grid content-start gap-3">
          <Panel
            title="Sides"
            bodyClassName="p-0"
            right={
              <div className="flex gap-1">
                <Button size="sm" variant="ghost" onClick={() => setSides(s => ({ ...s, B: structuredClone(s.A) }))}>
                  <Copy className="size-3.5" />A → B
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSides(s => ({ A: s.B, B: s.A }))}>
                  <ArrowLeftRight className="size-3.5" />A ↔ B
                </Button>
              </div>
            }
          >
            <div className="grid gap-px bg-border md:grid-cols-2">
              {(['A', 'B'] as const).map(k => (
                <SideForm key={k} side={k} value={sides[k]} catalog={catalog} suggested={suggested} presets={presets ?? []} emptyProject={source === 'empty'} onChange={v => setSides(s => ({ ...s, [k]: v }))} />
              ))}
            </div>
          </Panel>

          <div className="flex flex-wrap items-center justify-between gap-3 border bg-panel px-3 py-2.5">
            <span className="text-[12.5px] text-muted-foreground">
              {!ready ? 'Check the project path to continue.' : !prompt.trim() ? 'Write the prompt to continue.' : source === 'empty' ? 'Empty folder · side A + side B · in parallel' : '3 layers: project (shared) + side A + side B · in parallel'}{repetitions > 1 && ` · ${repetitions} attempts, one after another`}
            </span>
            <label className="flex items-center gap-2 text-[12.5px] text-muted-foreground" title="Runs the whole comparison several times, one attempt after another, to see how much results vary.">
              Repetitions
              <Input type="number" min={1} max={10} aria-label="Repetitions" className="tnum h-8 w-16 font-mono" value={repetitions}
                onChange={e => setRepetitions(Math.min(10, Math.max(1, Number(e.target.value) || 1)))} />
            </label>
            <Button size="lg" disabled={!canRun} onClick={run}>
              <Play className="size-3.5" />
              {start.isPending ? 'Preparing…' : 'Run comparison'}
            </Button>
          </div>
          {start.error && <ErrorNote error={start.error} />}
        </div>
      </div>
    </>
  )
}
