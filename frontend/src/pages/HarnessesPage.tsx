// Harness presets are phase 2. These screens work against mock data so the flow can be reviewed early.

import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { Copy, Folder, Lock, Pencil, Plus, Trash2, Upload } from 'lucide-react'
import { usePreset, usePresetFile, usePresets } from '@/api/queries'
import type { Cli } from '@/api/types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, Field, LoadingRows, Panel } from '@/components/common/primitives'

const PHASE_NOTE = 'In phase 1 every comparison uses the harness of the project itself. Presets arrive in phase 2.'

export function HarnessListPage() {
  const { data, error, isLoading } = usePresets()
  return (
    <>
      <TopBar crumbs={[{ label: 'Harnesses' }]}>
        <Chip>phase 2</Chip>
        <Button size="sm" asChild>
          <Link to="/harnesses/new"><Plus className="size-3.5" />New preset</Link>
        </Button>
      </TopBar>
      <div className="p-4">
        <p className="mb-3 max-w-[70ch] text-[13px] text-muted-foreground">{PHASE_NOTE}</p>
        {isLoading && <LoadingRows />}
        {error && <ErrorNote error={error} />}
        {data && (
          <div className="grid gap-px overflow-hidden border bg-border sm:grid-cols-2 xl:grid-cols-4">
            <div className="grid content-start gap-2 bg-panel p-4">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">No harness</span>
                <Chip><Lock className="size-3" />system</Chip>
              </div>
              <p className="text-[12.5px] leading-relaxed text-muted-foreground">Removes the project's harness files and adds nothing. Measures the CLI and model out of the box.</p>
              <span className="font-mono text-[11.5px] text-dim">opencode · codex · claude</span>
            </div>
            {data.map(p => (
              <Link
                key={p.slug}
                to="/harnesses/$slug"
                params={{ slug: p.slug }}
                className="grid content-start gap-2 bg-panel p-4 outline-none hover:bg-raise focus-visible:bg-raise"
              >
                <span className="font-medium">{p.title}</span>
                <span className="text-[12.5px] leading-relaxed text-muted-foreground">{p.description}</span>
                <span className="font-mono text-[11.5px] text-dim">
                  {p.clis.join(' · ')} · {p.files.length} {p.files.length === 1 ? 'file' : 'files'} · used {p.uses} times
                </span>
              </Link>
            ))}
          </div>
        )}
      </div>
    </>
  )
}

export function HarnessNewPage() {
  const navigate = useNavigate()
  const [clis, setClis] = useState<Record<Cli, boolean>>({ opencode: true, codex: true, claude: false })
  const detected = ['AGENTS.md', 'CLAUDE.md', '.claude/skills/migrations/', '.agents/rules/', '.mcp.json']
  const [picked, setPicked] = useState<Record<string, boolean>>(Object.fromEntries(detected.map(f => [f, f !== 'CLAUDE.md'])))
  const back = () => navigate({ to: '/harnesses' })

  return (
    <>
      <TopBar crumbs={[{ label: 'Harnesses', to: '/harnesses' }, { label: 'New preset' }]}>
        <Button size="sm" variant="outline" onClick={back}>Cancel</Button>
        <Button size="sm" onClick={back}>Create preset</Button>
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1000px] gap-3 p-4">
        <Panel title="Details">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="preset-name" hint="Becomes the folder harnesses/strict-backend-v2/.">
              <Input id="preset-name" defaultValue="Strict backend v2" />
            </Field>
            <Field label="Compatible CLIs">
              <div className="flex h-8 items-center gap-4">
                {(['opencode', 'codex', 'claude'] as const).map(c => (
                  <Label key={c} className="flex items-center gap-1.5 font-mono font-normal">
                    <Checkbox checked={clis[c]} onCheckedChange={v => setClis({ ...clis, [c]: v === true })} />
                    {c}
                  </Label>
                ))}
              </div>
            </Field>
            <Field label="Description" htmlFor="preset-description" className="sm:col-span-2">
              <Textarea id="preset-description" rows={2} defaultValue="Same as Strict backend, with the error rule rewritten." />
            </Field>
          </div>
        </Panel>
        <div className="grid gap-3 md:grid-cols-2">
          <Panel title="Import from a project">
            <div className="flex gap-2">
              <Input id="import-path" aria-label="Project path to import from" className="font-mono" defaultValue="C:\Users\Jose\Desktop\projects\stock-api" />
              <Button variant="outline">Detect</Button>
            </div>
            <div className="mt-3 grid gap-1.5">
              {detected.map(f => (
                <Label key={f} className="flex items-center gap-2 font-mono text-[12.5px] font-normal">
                  <Checkbox checked={picked[f]} onCheckedChange={v => setPicked({ ...picked, [f]: v === true })} />
                  {f}
                </Label>
              ))}
            </div>
          </Panel>
          <Panel title="Drop files">
            <div className="grid gap-2">
              {[['project/', 'AGENTS.md, .claude/, .agents/, .mcp.json…'], ['home/', '.codex/config.toml']].map(([root, hint]) => (
                <div key={root} className="flex items-center gap-3 border border-dashed p-4">
                  <Upload className="size-4 text-dim" />
                  <div>
                    <div className="font-mono text-[13px]">{root}</div>
                    <div className="text-xs text-dim">{hint}</div>
                  </div>
                </div>
              ))}
            </div>
          </Panel>
        </div>
      </div>
    </>
  )
}

export function HarnessDetailPage() {
  const { slug } = useParams({ from: '/harnesses/$slug' })
  const navigate = useNavigate()
  const { data: preset, error, isLoading } = usePreset(slug)
  const [file, setFile] = useState('AGENTS.md')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const { data: content } = usePresetFile(slug, file)

  if (isLoading) return <div className="p-4"><LoadingRows /></div>
  if (error || !preset) return <div className="p-4"><ErrorNote error={error} /></div>

  const roots = (['project', 'home'] as const).map(root => ({ root, files: preset.files.filter(f => f.root === root) }))

  return (
    <>
      <TopBar crumbs={[{ label: 'Harnesses', to: '/harnesses' }, { label: preset.title }]}>
        <Chip>phase 2</Chip>
        <Button size="sm" variant="outline"><Copy className="size-3.5" />Duplicate</Button>
        <Button size="sm" variant="outline"><Pencil className="size-3.5" />Rename</Button>
        <Button size="sm" variant="destructive" onClick={() => setConfirmDelete(true)}><Trash2 className="size-3.5" />Delete</Button>
      </TopBar>
      <div className="grid min-h-[600px] md:grid-cols-[260px_1fr]">
        <div className="border-b bg-panel p-2 md:border-r md:border-b-0">
          <div className="px-2 pt-1 pb-2 label-caps">Files · {preset.files.length}</div>
          {roots.map(({ root, files }) =>
            files.length === 0 ? null : (
              <div key={root} className="mb-2">
                <div className="flex items-center gap-1.5 px-2 py-1 font-mono text-xs text-muted-foreground"><Folder className="size-3.5" />{root}/</div>
                {files.map(f => (
                  <button
                    key={f.path}
                    onClick={() => setFile(f.path)}
                    className={cn('flex w-full items-center gap-1.5 py-1 pr-2 pl-6 text-left font-mono text-xs outline-none focus-visible:bg-raise', file === f.path ? 'bg-raise text-foreground' : 'text-muted-foreground hover:text-foreground')}
                  >
                    <span className="truncate">{f.path}</span>
                    <span className="ml-auto shrink-0 text-[10px] text-dim">{f.category}</span>
                  </button>
                ))}
              </div>
            ),
          )}
          <div className="m-2 border border-dashed p-3 text-center text-xs text-dim">Drop files here</div>
        </div>
        <div className="min-w-0">
          <div className="flex items-center justify-between border-b bg-background px-4 py-1.5">
            <span className="font-mono text-[12.5px]">{file}</span>
            <Button size="sm" variant="ghost"><Pencil className="size-3.5" />Edit</Button>
          </div>
          <div className="mx-auto max-w-[70ch] px-6 py-6">{content && <Markdown source={content} />}</div>
        </div>
      </div>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete “{preset.title}”?</DialogTitle>
            <DialogDescription>The preset folder is deleted. Comparisons that used it keep their own copy.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
            <Button variant="destructive" onClick={() => navigate({ to: '/harnesses' })}>Delete preset</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

/** Minimal Markdown for previews (headings and lists). The real app will use react-markdown. */
function Markdown({ source }: { source: string }) {
  return (
    <div className="grid gap-1">
      {source.split('\n').map((line, i) => {
        if (line.startsWith('# ')) return <h1 key={i} className="mb-2 text-xl font-semibold">{line.slice(2)}</h1>
        if (line.startsWith('## ')) return <h2 key={i} className="mt-4 mb-1 text-[15px] font-semibold">{line.slice(3)}</h2>
        if (line.startsWith('- ')) return <div key={i} className="flex gap-2 text-sm leading-[1.7] text-foreground/90"><span className="text-dim">–</span>{line.slice(2)}</div>
        if (!line.trim()) return null
        return <p key={i} className="text-sm leading-[1.7] text-foreground/90">{line}</p>
      })}
    </div>
  )
}
