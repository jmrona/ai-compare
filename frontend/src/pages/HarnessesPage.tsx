// Harness presets (/harnesses): reusable sets of harness files a side can run with instead of the
// project's own. They live on disk in ai-compare's data volume (harnesses/<slug>/ with preset.md,
// project/ and home/); see PresetService.

import type { DragEvent, ReactNode } from 'react'
import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { parse as parseToml } from 'smol-toml'
import { Copy, FilePlus, Folder, FolderInput, LayoutGrid, Lock, Pencil, Plus, Search, Trash2, Upload } from 'lucide-react'
import { PRESET_CARDS, agentsMdWith } from '@/lib/presetCards'
import { clients } from '@/api/transport'
import {
  useCreatePreset,
  useDeletePreset,
  useDeletePresetFile,
  useDuplicatePreset,
  useImportIntoPreset,
  useInspectProject,
  useMovePresetFile,
  usePreset,
  usePresetFile,
  usePresets,
  useUpdatePreset,
  useWritePresetFile,
} from '@/api/queries'
import type { Cli, Preset, PresetFile, PresetRoot } from '@/api/types'
import { cn } from '@/lib/utils'
import { formatDateTime } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { TopBar } from '@/components/app/AppShell'
import { Chip, ErrorNote, Field, LoadingRows, Panel, Segmented } from '@/components/common/primitives'
import { FolderBrowser } from '@/components/compare/FolderBrowser'
import { FileTree } from '@/components/common/FileTree'

const CLIS: Cli[] = ['opencode', 'codex', 'claude']
const NO_BASE = '-'
const ROOTS: { root: PresetRoot; hint: string }[] = [
  { root: 'project', hint: 'copied to the project root: AGENTS.md, CLAUDE.md, .claude/, .agents/, .opencode/, .mcp.json…' },
  { root: 'home', hint: "copied to the agent's home folder, e.g. .codex/config.toml" },
]

/* ── List ─────────────────────────────────────────────────── */

export function HarnessListPage() {
  const { data, error, isLoading } = usePresets()
  const [q, setQ] = useState('')
  const shown = (data ?? []).filter(p => (p.title + ' ' + p.description).toLowerCase().includes(q.trim().toLowerCase()))
  return (
    <>
      <TopBar crumbs={[{ label: 'Harnesses' }]}>
        <Button size="sm" asChild>
          <Link to="/harnesses/new"><Plus className="size-3.5" />New preset</Link>
        </Button>
      </TopBar>
      <div className="mx-auto w-full max-w-[1200px] p-4">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <p className="max-w-[75ch] text-[13px] text-muted-foreground">
            A preset replaces a project's harness files on one side of a comparison, so two sides can run with different instructions.
            Each comparison keeps a copy of the preset it used.
          </p>
          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-dim" />
            <Input aria-label="Search presets" className="pl-8" placeholder="Search presets" value={q} onChange={e => setQ(e.target.value)} />
          </div>
        </div>
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
            {shown.map(p => (
              <Link
                key={p.slug}
                to="/harnesses/$slug"
                params={{ slug: p.slug }}
                className="grid content-start gap-2 bg-panel p-4 outline-none hover:bg-raise focus-visible:bg-raise"
              >
                <span className="font-medium">{p.title}</span>
                {p.description && <span className="text-[12.5px] leading-relaxed text-muted-foreground">{p.description}</span>}
                <span className="font-mono text-[11.5px] text-dim">
                  {[p.clis.join(' · ') || 'any CLI', `${p.files.length} ${p.files.length === 1 ? 'file' : 'files'}`, `used ${p.uses} ${p.uses === 1 ? 'time' : 'times'}`].join(' · ')}
                </span>
                <span className="text-[11px] text-dim">edited {formatDateTime(p.updatedAt)}</span>
              </Link>
            ))}
            {data.length === 0 && (
              <div className="grid content-center gap-2 bg-panel p-4 text-[12.5px] text-muted-foreground sm:col-span-1 xl:col-span-3">
                No presets yet. Create one from scratch, import the harness files of a project, or drop files into it.
              </div>
            )}
          </div>
        )}
      </div>
    </>
  )
}

/* ── New ──────────────────────────────────────────────────── */

export function HarnessNewPage() {
  const navigate = useNavigate()
  const create = useCreatePreset()
  const importFiles = useImportIntoPreset()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [clis, setClis] = useState<Cli[]>(['opencode'])
  const [importing, setImporting] = useState<{ path: string; paths: string[] } | null>(null)
  const [cards, setCards] = useState<string[]>([])
  const write = useWritePresetFile()
  const { data: presets } = usePresets()
  const duplicate = useDuplicatePreset()
  const update = useUpdatePreset()
  const [base, setBase] = useState<Preset | null>(null)
  const pending = create.isPending || duplicate.isPending || update.isPending || importFiles.isPending || write.isPending
  const error = create.error ?? duplicate.error ?? update.error ?? importFiles.error ?? write.error

  const pickBase = (slug: string) => {
    const b = presets?.find(p => p.slug === slug) ?? null
    setBase(b)
    if (b) {
      setDescription(b.description)
      setClis(b.clis)
    }
  }

  const save = async () => {
    const p = base
      ? await duplicate.mutateAsync({ slug: base.slug, title }).then(d => update.mutateAsync({ slug: d.slug, title, description, clis, notes: base.notes }))
      : await create.mutateAsync({ title, description, clis })
    if (importing && importing.paths.length > 0) await importFiles.mutateAsync({ slug: p.slug, projectPath: importing.path, paths: importing.paths })
    if (cards.length > 0) {
      // Cards go after anything imported into AGENTS.md.
      const existing = await clients.presets.getPresetFile({ slug: p.slug, root: 'project', path: 'AGENTS.md' }).then(r => new TextDecoder().decode(r.content)).catch(() => '')
      await write.mutateAsync({ slug: p.slug, root: 'project', path: 'AGENTS.md', content: agentsMdWith(PRESET_CARDS.filter(c => cards.includes(c.id)), existing) })
    }
    navigate({ to: '/harnesses/$slug', params: { slug: p.slug } })
  }

  return (
    <>
      <TopBar crumbs={[{ label: 'Harnesses', to: '/harnesses' }, { label: 'New preset' }]}>
        <Button size="sm" variant="outline" asChild><Link to="/harnesses">Cancel</Link></Button>
        <Button size="sm" disabled={!title.trim() || pending} onClick={save}>
          {pending ? 'Creating…' : 'Create preset'}
        </Button>
      </TopBar>
      <div className="mx-auto grid w-full max-w-[1000px] gap-3 p-4">
        {error && <ErrorNote error={error} />}
        <DetailsPanel title={title} setTitle={setTitle} description={description} setDescription={setDescription} clis={clis} setClis={setClis} />
        <Panel title="Start from an existing preset" right={base && <span className="text-xs text-dim">{base.files.length} {base.files.length === 1 ? 'file' : 'files'} copied</span>}>
          <Select value={base?.slug ?? NO_BASE} onValueChange={v => pickBase(v === NO_BASE ? '' : v)}>
            <SelectTrigger aria-label="Existing preset to start from" className="w-full sm:w-96"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_BASE}>Nothing: an empty preset</SelectItem>
              {(presets ?? []).map(p => <SelectItem key={p.slug} value={p.slug}>{p.title}</SelectItem>)}
            </SelectContent>
          </Select>
          <p className="mt-2 text-xs text-dim">Its files and notes are copied into the new preset; cards and imports below are added on top. The original stays as it is.</p>
        </Panel>
        <Panel title="Start from cards" right={<span className="text-xs text-dim">{cards.length} selected · written to AGENTS.md</span>}>
          <CardPicker selected={cards} onChange={setCards} />
        </Panel>
        <ImportPanel onChange={setImporting} />
        <p className="text-xs text-dim">After creating it you can drop files and folders into the preset, and write or edit files.</p>
      </div>
    </>
  )
}

function DetailsPanel({ title, setTitle, description, setDescription, clis, setClis }: {
  title: string
  setTitle: (v: string) => void
  description: string
  setDescription: (v: string) => void
  clis: Cli[]
  setClis: (v: Cli[]) => void
}) {
  return (
    <Panel title="Details">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Name" htmlFor="preset-name">
          <Input id="preset-name" value={title} onChange={e => setTitle(e.target.value)} placeholder="Strict backend" />
        </Field>
        <Field label="Compatible CLIs" hint="Each CLI reads the files it understands; the others are ignored.">
          <div className="flex min-h-8 flex-wrap items-center gap-x-4 gap-y-1.5">
            {CLIS.map(c => (
              <Label key={c} className="flex items-center gap-1.5 font-mono font-normal">
                <Checkbox checked={clis.includes(c)} onCheckedChange={v => setClis(v === true ? [...clis, c] : clis.filter(x => x !== c))} />
                {c}
              </Label>
            ))}
          </div>
        </Field>
        <Field label="Description" htmlFor="preset-description" className="sm:col-span-2">
          <Textarea id="preset-description" rows={2} value={description} onChange={e => setDescription(e.target.value)} placeholder="What these instructions are for" />
        </Field>
      </div>
    </Panel>
  )
}

/** Picks harness files of a project (detected by InspectProject, preselected) or any other entry
 *  at its root to import. */
function ImportPanel({ onChange, onImport, busy }: {
  onChange?: (v: { path: string; paths: string[] } | null) => void
  onImport?: (v: { path: string; paths: string[] }) => void
  busy?: boolean
}) {
  const inspect = useInspectProject()
  const [path, setPath] = useState('')
  const [browsing, setBrowsing] = useState(false)
  const [picked, setPicked] = useState<string[]>([])
  const detected = inspect.data?.harnessFiles.map(f => f.path) ?? []
  const others = inspect.data?.otherEntries ?? []

  const detect = (p = path) =>
    inspect.mutate(p.trim(), {
      onSuccess: i => {
        const all = i.harnessFiles.map(f => f.path)
        setPicked(all)
        onChange?.({ path: p.trim(), paths: all })
      },
    })
  const toggle = (f: string, on: boolean) => {
    const next = on ? [...picked, f] : picked.filter(x => x !== f)
    setPicked(next)
    onChange?.({ path: path.trim(), paths: next })
  }

  return (
    <Panel title="Import from a project">
      <div className="flex gap-2">
        <Input aria-label="Project path to import from" className="font-mono" placeholder="Absolute path of a project" value={path} onChange={e => setPath(e.target.value)} />
        <Button variant="outline" onClick={() => setBrowsing(true)}>Browse…</Button>
        <Button variant="outline" disabled={!path.trim() || inspect.isPending} onClick={() => detect()}>{inspect.isPending ? 'Detecting…' : 'Detect'}</Button>
      </div>
      <FolderBrowser open={browsing} onOpenChange={setBrowsing} startPath="" onSelect={p => { setPath(p); detect(p) }} />
      {inspect.error && <div className="mt-3"><ErrorNote error={inspect.error} /></div>}
      {inspect.data && detected.length === 0 && <p className="mt-3 text-xs text-muted-foreground">No usual harness files at the root of this project; pick other files or folders below.</p>}
      {inspect.data && (
        <div className="mt-3 grid gap-1.5">
          {detected.map(f => (
            <Label key={f} className="flex items-center gap-2 font-mono text-[12.5px] font-normal">
              <Checkbox checked={picked.includes(f)} onCheckedChange={v => toggle(f, v === true)} />
              {f}
            </Label>
          ))}
          {others.length > 0 && (
            <details className="mt-1" open={detected.length === 0}>
              <summary className="cursor-pointer text-xs text-muted-foreground">Other files and folders at the root · {others.length}</summary>
              <p className="mt-1 text-xs text-dim">For instructions kept elsewhere, such as rules/ or skills/.</p>
              <div className="mt-1.5 grid max-h-56 gap-1.5 overflow-y-auto sm:grid-cols-2">
                {others.map(f => (
                  <Label key={f} className="flex min-w-0 items-center gap-2 font-mono text-[12.5px] font-normal">
                    <Checkbox checked={picked.includes(f)} onCheckedChange={v => toggle(f, v === true)} />
                    <span className="truncate" title={f}>{f}</span>
                  </Label>
                ))}
              </div>
            </details>
          )}
          <p className="text-xs text-dim">They go into the preset's project/ folder with their paths. .env files, dependencies and CLI worktrees are never imported.</p>
          {onImport && (
            <Button className="mt-1 w-fit" size="sm" disabled={busy || picked.length === 0} onClick={() => onImport({ path: path.trim(), paths: picked })}>
              <FolderInput className="size-3.5" />{busy ? 'Importing…' : `Import ${picked.length} into the preset`}
            </Button>
          )}
        </div>
      )}
    </Panel>
  )
}

/* ── Detail ───────────────────────────────────────────────── */

type Selected = { root: PresetRoot; path: string } | null

export function HarnessDetailPage() {
  const { slug } = useParams({ from: '/harnesses/$slug' })
  const { data: preset, error, isLoading } = usePreset(slug)
  if (isLoading) return <div className="p-4"><LoadingRows /></div>
  if (error || !preset) {
    return (
      <>
        <TopBar crumbs={[{ label: 'Harnesses', to: '/harnesses' }, { label: slug }]} />
        <div className="p-4"><ErrorNote error={error} /></div>
      </>
    )
  }
  return <PresetEditor preset={preset} />
}

function PresetEditor({ preset }: { preset: Preset }) {
  const navigate = useNavigate()
  const write = useWritePresetFile()
  const importFiles = useImportIntoPreset()
  const [selected, setSelected] = useState<Selected>(() => {
    const first = preset.files.find(f => f.category === 'instructions') ?? preset.files[0]
    return first ? { root: first.root, path: first.path } : null
  })
  const [dialog, setDialog] = useState<null | 'details' | 'duplicate' | 'delete' | 'new-file' | 'import' | 'cards'>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const [dropError, setDropError] = useState<unknown>(null)
  const categories = useMemo(() => countBy(preset.files.map(f => f.category)), [preset.files])

  const onDrop = async (e: DragEvent, root: PresetRoot) => {
    e.preventDefault()
    setDropError(null)
    try {
      const files = await droppedFiles(e.dataTransfer)
      const found: string[] = []
      for (const f of files) {
        const r = await write.mutateAsync({ slug: preset.slug, root, path: f.path, content: f.content })
        found.push(...r.warnings)
      }
      setWarnings(found)
      if (files[0]) setSelected({ root, path: files[0].path })
    } catch (err) {
      setDropError(err)
    }
  }

  return (
    <>
      <TopBar crumbs={[{ label: 'Harnesses', to: '/harnesses' }, { label: preset.title }]}>
        <Button size="sm" variant="outline" onClick={() => setDialog('details')}><Pencil className="size-3.5" />Edit details</Button>
        <Button size="sm" variant="outline" onClick={() => setDialog('duplicate')}><Copy className="size-3.5" />Duplicate</Button>
        <Button size="sm" variant="destructive" onClick={() => setDialog('delete')}><Trash2 className="size-3.5" />Delete</Button>
      </TopBar>

      <div className="border-b px-4 py-3">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <h1 className="text-[17px] font-semibold">{preset.title}</h1>
          <span className="font-mono text-xs text-dim">{preset.clis.join(' · ') || 'any CLI'} · hash {preset.hash} · edited {formatDateTime(preset.updatedAt)}</span>
          {preset.uses > 0 ? (
            <Link to="/history" search={{ preset: preset.slug }} className="text-xs text-dim underline-offset-2 hover:text-foreground hover:underline">
              used by {preset.uses} {preset.uses === 1 ? 'side' : 'sides'}
            </Link>
          ) : (
            <span className="text-xs text-dim">not used yet</span>
          )}
        </div>
        {preset.description && <p className="mt-1 max-w-[80ch] text-[13px] text-muted-foreground">{preset.description}</p>}
        <div className="mt-2 flex flex-wrap gap-1.5">
          {Object.entries(categories).map(([c, n]) => <Chip key={c}>{c} · {n}</Chip>)}
        </div>
      </div>

      {warnings.length > 0 && <div className="border-b border-warn/30 bg-warn/10 px-4 py-2 text-[12.5px] text-warn">{warnings.join(' ')}</div>}
      {dropError != null && <div className="px-4 pt-3"><ErrorNote error={dropError} /></div>}

      <div className="grid min-h-[480px] flex-1 md:grid-cols-[300px_minmax(0,1fr)] md:grid-rows-[minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col border-b bg-panel md:border-r md:border-b-0">
          <div className="flex items-center gap-1 border-b px-2 py-1.5">
            <span className="label-caps px-1">Files · {preset.files.length}</span>
            <span className="ml-auto flex gap-0.5">
              <Button size="icon-sm" variant="ghost" aria-label="New file" title="New file" onClick={() => setDialog('new-file')}><FilePlus className="size-3.5" /></Button>
              <Button size="icon-sm" variant="ghost" aria-label="Import from a project" title="Import from a project" onClick={() => setDialog('import')}><FolderInput className="size-3.5" /></Button>
              <Button size="icon-sm" variant="ghost" aria-label="Add cards" title="Add cards to AGENTS.md" onClick={() => setDialog('cards')}><LayoutGrid className="size-3.5" /></Button>
            </span>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {ROOTS.map(({ root, hint }) => (
              <div key={root} className="mb-3" onDragOver={e => e.preventDefault()} onDrop={e => onDrop(e, root)}>
                <div className="flex items-center gap-1.5 px-2 py-1 font-mono text-xs text-muted-foreground" title={hint}><Folder className="size-3.5" />{root}/</div>
                <FileTree
                  className="pl-2"
                  files={preset.files.filter(f => f.root === root).map(f => ({ path: f.path, aside: f.category }))}
                  selected={selected?.root === root ? selected.path : null}
                  onSelect={path => setSelected({ root, path })}
                />
                <div className="mx-2 mt-1 flex items-center gap-2 border border-dashed px-3 py-2 text-[11px] text-dim">
                  <Upload className="size-3.5 shrink-0" />Drop files or folders into {root}/
                </div>
              </div>
            ))}
            {write.isPending && <p className="px-2 text-xs text-muted-foreground">Uploading…</p>}
          </div>
        </aside>
        <section className="flex min-h-[420px] min-w-0 flex-col md:min-h-0 md:overflow-y-auto">
          {selected && preset.files.some(f => f.root === selected.root && f.path === selected.path) ? (
            <FileView key={`${selected.root}/${selected.path}`} preset={preset} file={selected} onMoved={setSelected} onDeleted={() => setSelected(null)} onWarnings={setWarnings} />
          ) : (
            <div className="grid flex-1 place-content-center gap-2 p-8 text-center text-[13px] text-muted-foreground">
              <p>{preset.files.length === 0 ? 'This preset has no files yet.' : 'Choose a file to view or edit it.'}</p>
              <p className="text-xs text-dim">Drop files or folders onto project/ or home/, create a file, or import from a project.</p>
            </div>
          )}
        </section>
      </div>

      <DetailsDialog preset={preset} open={dialog === 'details'} onClose={() => setDialog(null)} />
      <DuplicateDialog preset={preset} open={dialog === 'duplicate'} onClose={() => setDialog(null)} onDone={slug => navigate({ to: '/harnesses/$slug', params: { slug } })} />
      <DeleteDialog preset={preset} open={dialog === 'delete'} onClose={() => setDialog(null)} onDone={() => navigate({ to: '/harnesses' })} />
      <NewFileDialog preset={preset} open={dialog === 'new-file'} onClose={() => setDialog(null)} onDone={f => { setSelected(f); setDialog(null) }} />
      <CardsDialog preset={preset} open={dialog === 'cards'} onClose={() => setDialog(null)} onDone={() => { setSelected({ root: 'project', path: 'AGENTS.md' }); setDialog(null) }} />
      <Dialog open={dialog === 'import'} onOpenChange={o => !o && setDialog(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Import from a project</DialogTitle>
            <DialogDescription>Files with the same path in the preset are replaced.</DialogDescription>
          </DialogHeader>
          <ImportPanel busy={importFiles.isPending} onImport={v => importFiles.mutate({ slug: preset.slug, projectPath: v.path, paths: v.paths }, { onSuccess: () => setDialog(null) })} />
          {importFiles.error && <ErrorNote error={importFiles.error} />}
        </DialogContent>
      </Dialog>
    </>
  )
}

/* ── One file: view, edit, move, delete ───────────────────── */

function FileView({ preset, file, onMoved, onDeleted, onWarnings }: {
  preset: Preset
  file: { root: PresetRoot; path: string }
  onMoved: (f: { root: PresetRoot; path: string }) => void
  onDeleted: () => void
  onWarnings: (w: string[]) => void
}) {
  const { data: content, error, isLoading } = usePresetFile(preset.slug, file.root, file.path)
  const write = useWritePresetFile()
  const del = useDeletePresetFile()
  const move = useMovePresetFile()
  const [draft, setDraft] = useState<string | null>(null)
  const [moving, setMoving] = useState(false)
  const syntax = draft == null ? null : checkSyntax(file.path, draft)
  const isMarkdown = /\.(md|mdx|markdown)$/i.test(file.path)

  const save = () =>
    draft != null && write.mutate({ slug: preset.slug, root: file.root, path: file.path, content: draft }, {
      onSuccess: r => { setDraft(null); onWarnings(r.warnings) },
    })

  return (
    <>
      <div className="sticky top-0 z-10 flex flex-wrap items-center gap-2 border-b bg-background px-4 py-1.5">
        <span className="min-w-0 truncate font-mono text-[12.5px]" title={`${file.root}/${file.path}`}><span className="text-dim">{file.root}/</span>{file.path}</span>
        <span className="ml-auto flex gap-1">
          {draft == null ? (
            <>
              <Button size="sm" variant="ghost" disabled={content == null} onClick={() => setDraft(content ?? '')}><Pencil className="size-3.5" />Edit</Button>
              <Button size="sm" variant="ghost" onClick={() => setMoving(true)}>Rename or move</Button>
              <Button size="sm" variant="ghost" disabled={del.isPending} onClick={() => del.mutate({ slug: preset.slug, root: file.root, path: file.path }, { onSuccess: onDeleted })}>
                <Trash2 className="size-3.5" />Delete
              </Button>
            </>
          ) : (
            <>
              <Button size="sm" variant="ghost" onClick={() => setDraft(null)}>Discard</Button>
              <Button size="sm" disabled={write.isPending || !!syntax} onClick={save}>{write.isPending ? 'Saving…' : 'Save'}</Button>
            </>
          )}
        </span>
      </div>
      {(error || write.error || del.error) && <div className="p-3"><ErrorNote error={error ?? write.error ?? del.error} /></div>}
      {syntax && <div className="border-b border-danger/30 bg-danger/10 px-4 py-1.5 font-mono text-xs text-danger">{syntax}</div>}
      {isLoading && <div className="p-4"><LoadingRows rows={5} /></div>}
      {draft != null ? (
        <Textarea
          aria-label={`Edit ${file.path}`}
          className="min-h-[420px] flex-1 rounded-none border-0 bg-term font-mono text-[12.5px] leading-relaxed focus-visible:ring-0"
          value={draft}
          spellCheck={false}
          onChange={e => setDraft(e.target.value)}
        />
      ) : content != null && isMarkdown ? (
        <article className="prose-preset mx-auto w-full max-w-[75ch] px-6 py-6 text-sm leading-[1.7] text-foreground/90">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
        </article>
      ) : content != null ? (
        <pre className="min-h-0 flex-1 overflow-auto bg-term p-4 font-mono text-[12.5px] leading-relaxed whitespace-pre-wrap">{content}</pre>
      ) : null}
      <MoveDialog preset={preset} file={file} open={moving} busy={move.isPending} error={move.error} onClose={() => setMoving(false)}
        onMove={(root, path) => move.mutate({ slug: preset.slug, root: file.root, path: file.path, newRoot: root, newPath: path }, { onSuccess: () => { setMoving(false); onMoved({ root, path }) } })} />
    </>
  )
}

/** JSON and TOML are checked as they are typed; a broken config file is not saved. */
function checkSyntax(path: string, text: string): string | null {
  try {
    if (/\.json$/i.test(path)) JSON.parse(text)
    if (/\.toml$/i.test(path)) parseToml(text)
  } catch (e) {
    return (e instanceof Error ? e.message : String(e)).split('\n')[0]
  }
  return null
}

/* ── Dialogs ──────────────────────────────────────────────── */

function SimpleDialog({ open, onClose, title, description, children, footer, className }: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children?: ReactNode
  footer: ReactNode
  className?: string
}) {
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()}>
      <DialogContent className={className}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {children}
        <DialogFooter>{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DetailsDialog({ preset, open, onClose }: { preset: Preset; open: boolean; onClose: () => void }) {
  const update = useUpdatePreset()
  const [title, setTitle] = useState(preset.title)
  const [description, setDescription] = useState(preset.description)
  const [clis, setClis] = useState<Cli[]>(preset.clis)
  const [notes, setNotes] = useState(preset.notes)
  return (
    <SimpleDialog className="sm:max-w-2xl" open={open} onClose={onClose} title="Edit details" description="The folder name (slug) stays the same."
      footer={<>
        <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
        <Button disabled={!title.trim() || update.isPending} onClick={() => update.mutate({ slug: preset.slug, title, description, clis, notes }, { onSuccess: onClose })}>Save</Button>
      </>}
    >
      <DetailsPanel title={title} setTitle={setTitle} description={description} setDescription={setDescription} clis={clis} setClis={setClis} />
      <Field label="Notes" htmlFor="preset-notes" hint="Free text kept in preset.md.">
        <Textarea id="preset-notes" rows={3} value={notes} onChange={e => setNotes(e.target.value)} />
      </Field>
      {update.error && <ErrorNote error={update.error} />}
    </SimpleDialog>
  )
}

function DuplicateDialog({ preset, open, onClose, onDone }: { preset: Preset; open: boolean; onClose: () => void; onDone: (slug: string) => void }) {
  const dup = useDuplicatePreset()
  const [title, setTitle] = useState(`${preset.title} (copy)`)
  return (
    <SimpleDialog open={open} onClose={onClose} title="Duplicate preset" description="A copy with all its files, to try a variant."
      footer={<>
        <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
        <Button disabled={!title.trim() || dup.isPending} onClick={() => dup.mutate({ slug: preset.slug, title }, { onSuccess: p => { onClose(); onDone(p.slug) } })}>Duplicate</Button>
      </>}
    >
      <Field label="Name of the copy" htmlFor="dup-title"><Input id="dup-title" value={title} onChange={e => setTitle(e.target.value)} /></Field>
      {dup.error && <ErrorNote error={dup.error} />}
    </SimpleDialog>
  )
}

function DeleteDialog({ preset, open, onClose, onDone }: { preset: Preset; open: boolean; onClose: () => void; onDone: () => void }) {
  const del = useDeletePreset()
  return (
    <SimpleDialog open={open} onClose={onClose} title={`Delete “${preset.title}”?`} description="The preset folder is deleted. Comparisons that used it keep their own copy."
      footer={<>
        <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
        <Button variant="destructive" disabled={del.isPending} onClick={() => del.mutate(preset.slug, { onSuccess: () => { onClose(); onDone() } })}>Delete preset</Button>
      </>}
    >
      {del.error && <ErrorNote error={del.error} />}
    </SimpleDialog>
  )
}

function NewFileDialog({ preset, open, onClose, onDone }: { preset: Preset; open: boolean; onClose: () => void; onDone: (f: { root: PresetRoot; path: string }) => void }) {
  const write = useWritePresetFile()
  const [root, setRoot] = useState<PresetRoot>('project')
  const [path, setPath] = useState('AGENTS.md')
  const exists = preset.files.some(f => f.root === root && f.path === path.trim())
  return (
    <SimpleDialog open={open} onClose={onClose} title="New file"
      footer={<>
        <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
        <Button disabled={!path.trim() || exists || write.isPending} onClick={() => write.mutate({ slug: preset.slug, root, path: path.trim(), content: '' }, { onSuccess: () => onDone({ root, path: path.trim() }) })}>Create</Button>
      </>}
    >
      <Field label="Where"><Segmented label="Root" value={root} onChange={setRoot} options={[{ value: 'project', label: 'project/' }, { value: 'home', label: 'home/' }]} /></Field>
      <Field label="Path" htmlFor="new-file-path" hint={exists ? 'A file with this path already exists.' : 'Folders are created as needed, e.g. .claude/skills/migrations/SKILL.md.'}>
        <Input id="new-file-path" className="font-mono" value={path} onChange={e => setPath(e.target.value)} />
      </Field>
      {write.error && <ErrorNote error={write.error} />}
    </SimpleDialog>
  )
}

function MoveDialog({ preset, file, open, busy, error, onClose, onMove }: {
  preset: Preset
  file: { root: PresetRoot; path: string }
  open: boolean
  busy: boolean
  error: unknown
  onClose: () => void
  onMove: (root: PresetRoot, path: string) => void
}) {
  const [root, setRoot] = useState<PresetRoot>(file.root)
  const [path, setPath] = useState(file.path)
  const taken = preset.files.some((f: PresetFile) => f.root === root && f.path === path.trim())
  return (
    <SimpleDialog open={open} onClose={onClose} title="Rename or move"
      footer={<>
        <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
        <Button disabled={!path.trim() || taken || busy} onClick={() => onMove(root, path.trim())}>Move</Button>
      </>}
    >
      <Field label="Where"><Segmented label="Root" value={root} onChange={setRoot} options={[{ value: 'project', label: 'project/' }, { value: 'home', label: 'home/' }]} /></Field>
      <Field label="Path" htmlFor="move-path" hint={taken ? 'That path is taken.' : undefined}>
        <Input id="move-path" className="font-mono" value={path} onChange={e => setPath(e.target.value)} />
      </Field>
      {error != null && <ErrorNote error={error} />}
    </SimpleDialog>
  )
}

/* ── Cards ────────────────────────────────────────────────── */

function CardPicker({ selected, onChange }: { selected: string[]; onChange: (ids: string[]) => void }) {
  return (
    <div className="grid gap-2 sm:grid-cols-2">
      {PRESET_CARDS.map(c => {
        const on = selected.includes(c.id)
        return (
          <Label key={c.id} className={cn('flex cursor-pointer items-start gap-2.5 border p-3 font-normal', on ? 'border-side-a/60 bg-side-a/5' : 'hover:bg-raise')}>
            <Checkbox className="mt-0.5" checked={on} onCheckedChange={v => onChange(v === true ? [...selected, c.id] : selected.filter(x => x !== c.id))} />
            <span className="grid gap-0.5">
              <span className="text-[13px] font-medium">{c.title}</span>
              <span className="text-xs text-muted-foreground">{c.description}</span>
            </span>
          </Label>
        )
      })}
    </div>
  )
}

function CardsDialog({ preset, open, onClose, onDone }: { preset: Preset; open: boolean; onClose: () => void; onDone: () => void }) {
  const write = useWritePresetFile()
  const exists = preset.files.some(f => f.root === 'project' && f.path === 'AGENTS.md')
  const { data: current } = usePresetFile(preset.slug, 'project', 'AGENTS.md', open && exists)
  const [cards, setCards] = useState<string[]>([])
  const add = () =>
    write.mutate({ slug: preset.slug, root: 'project', path: 'AGENTS.md', content: agentsMdWith(PRESET_CARDS.filter(c => cards.includes(c.id)), current ?? '') }, { onSuccess: () => { setCards([]); onDone() } })
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Add cards</DialogTitle>
          <DialogDescription>Each card adds a section to project/AGENTS.md{exists ? ', after what is already there' : ''}. opencode reads AGENTS.md; versions for the other CLIs come in phase 3.</DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto"><CardPicker selected={cards} onChange={setCards} /></div>
        {write.error && <ErrorNote error={write.error} />}
        <DialogFooter>
          <DialogClose asChild><Button variant="outline">Cancel</Button></DialogClose>
          <Button disabled={cards.length === 0 || write.isPending} onClick={add}>Add {cards.length || ''} to AGENTS.md</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ── Helpers ──────────────────────────────────────────────── */

function countBy(items: string[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const i of items) out[i] = (out[i] ?? 0) + 1
  return out
}

/** Reads dropped files and folders (recursively) with their relative paths. */
async function droppedFiles(dt: DataTransfer): Promise<{ path: string; content: Uint8Array }[]> {
  const out: { path: string; content: Uint8Array }[] = []
  const readFile = (entry: FileSystemFileEntry) => new Promise<File>((res, rej) => entry.file(res, rej))
  const readDir = (entry: FileSystemDirectoryEntry) =>
    new Promise<FileSystemEntry[]>((res, rej) => {
      const reader = entry.createReader()
      const all: FileSystemEntry[] = []
      const next = () => reader.readEntries(batch => (batch.length ? (all.push(...batch), next()) : res(all)), rej)
      next()
    })
  const walk = async (entry: FileSystemEntry): Promise<void> => {
    if (entry.isFile) {
      const f = await readFile(entry as FileSystemFileEntry)
      out.push({ path: entry.fullPath.replace(/^\//, ''), content: new Uint8Array(await f.arrayBuffer()) })
    } else if (entry.isDirectory) {
      for (const child of await readDir(entry as FileSystemDirectoryEntry)) await walk(child)
    }
  }
  const entries = [...dt.items].map(i => i.webkitGetAsEntry()).filter((e): e is FileSystemEntry => e != null)
  for (const e of entries) await walk(e)
  // Never .env files: their values would reach the provider.
  return out.filter(f => !/(^|\/)\.env(\.|$)/.test(f.path))
}
