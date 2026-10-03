import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { ArrowLeftRight, Check, ChevronDown, ChevronRight, Copy, Play } from 'lucide-react'
import type { Limits, ProjectProfile, Settings, SideConfig, SideKey } from '@/api/types'
import { useActiveComparison, useCatalog, useInspectProject, useSettings, useStartComparison } from '@/api/queries'
import { formatBytes, formatInt } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { TopBar } from '@/components/app/AppShell'
import { Chip, Dot, ErrorNote, Field, LoadingRows, Panel } from '@/components/common/primitives'
import { SideForm } from '@/components/compare/SideForm'

const EXAMPLE_PATH = 'C:\\Users\\Jose\\Desktop\\projects\\invoices-web'

const baseSide = (model: string, effort: SideConfig['effort'], mode: SideConfig['mode'], limits: Limits): SideConfig => ({
  cli: 'opencode',
  provider: 'openai',
  model,
  effort,
  mode,
  limits: { ...limits },
})

export function NewComparisonPage() {
  // The form starts from the default limits in Settings, so it waits for them.
  const { data: settings, error, isLoading } = useSettings()
  if (isLoading) return <div className="p-4"><LoadingRows rows={4} /></div>
  if (error || !settings) return <div className="p-4"><ErrorNote error={error} /></div>
  return <NewComparisonForm settings={settings} />
}

function NewComparisonForm({ settings }: { settings: Settings }) {
  const navigate = useNavigate()
  const { data: active } = useActiveComparison()
  const { data: catalog } = useCatalog()
  const inspect = useInspectProject()
  const start = useStartComparison()

  const [path, setPath] = useState(EXAMPLE_PATH)
  const [profile, setProfile] = useState<ProjectProfile | null>(null)
  const [profileOpen, setProfileOpen] = useState(false)
  const [prompt, setPrompt] = useState('')
  const [sides, setSides] = useState<Record<SideKey, SideConfig>>({
    A: baseSide('gpt-5.5', 'high', 'interactive', settings.defaultLimits),
    B: baseSide('gpt-5.5-mini', 'medium', 'autonomous', settings.defaultLimits),
  })

  const project = inspect.data
  const canRun = !!project && prompt.trim().length > 0 && !start.isPending

  const validate = () => inspect.mutate(path, { onSuccess: p => setProfile(p.profile) })

  const run = () => {
    if (!project || !profile) return
    start.mutate(
      { projectPath: project.path, profile, prompt: prompt.trim(), sides },
      { onSuccess: ({ id }) => navigate({ to: '/comparisons/$id', params: { id } }) },
    )
  }

  const suggested = settings.suggestedLimits

  return (
    <>
      <TopBar crumbs={[{ label: 'Compare' }, { label: 'New comparison' }]} />

      {active && (
        <div className="flex flex-wrap items-center gap-2 border-b border-warn/30 bg-warn/10 px-4 py-2 text-[12.5px] text-warn">
          <Dot tone="warn" live />
          Comparison #{active.id} is still running.
          <Link to="/comparisons/$id" params={{ id: active.id }} className="underline underline-offset-2">Go back to it</Link>
        </div>
      )}

      <div className="grid gap-3 p-4 xl:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        <div className="grid content-start gap-3">
          <Panel title="Project" right={project ? <Chip tone="ok"><Check className="size-3" />valid</Chip> : null}>
            <form className="flex gap-2" onSubmit={e => { e.preventDefault(); validate() }}>
              <Input
                id="project-path"
                aria-label="Absolute path to the project"
                className="font-mono"
                placeholder={EXAMPLE_PATH}
                value={path}
                onChange={e => setPath(e.target.value)}
              />
              <Button type="submit" variant="outline" disabled={inspect.isPending || !path.trim()}>
                {inspect.isPending ? 'Checking…' : 'Check'}
              </Button>
            </form>
            {inspect.error && <div className="mt-3"><ErrorNote error={inspect.error} /></div>}
            {!project && !inspect.error && (
              <p className="mt-3 text-xs text-muted-foreground">Absolute path to the folder on your machine. It is copied as it is, read-only; the original is never modified.</p>
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
                <SideForm key={k} side={k} value={sides[k]} catalog={catalog} suggested={suggested} onChange={v => setSides(s => ({ ...s, [k]: v }))} />
              ))}
            </div>
          </Panel>

          <div className="flex flex-wrap items-center justify-between gap-3 border bg-panel px-3 py-2.5">
            <span className="text-[12.5px] text-muted-foreground">
              {!project ? 'Check the project path to continue.' : !prompt.trim() ? 'Write the prompt to continue.' : '3 layers: project (shared) + side A + side B · in parallel'}
            </span>
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
