// The Changes tab: what the agent changed against the baseline commit, file by file. Each file
// starts collapsed, so the list of changed files reads at a glance; generated files (lock files,
// build output, very long diffs) are marked as such.
// Dependency folders (node_modules…) are left out by the backend and only counted.

import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { DiffLine, SideRun } from '@/api/types'
import { useDiff } from '@/api/queries'
import { formatInt } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Chip, ErrorNote, LoadingRows, Segmented } from '@/components/common/primitives'

interface FileDiff {
  path: string
  lines: DiffLine[]
  added: number
  removed: number
  generated: boolean
}

const GENERATED_PATH = /(^|\/)(dist|build|out|coverage)\/|(\.lock|-lock\.json|-lock\.yaml|\.min\.(js|css)|\.map)$/
const LONG_DIFF = 400

function splitByFile(lines: DiffLine[]): FileDiff[] {
  const files: FileDiff[] = []
  for (const l of lines) {
    if (l.kind === 'file') {
      files.push({ path: l.text, lines: [], added: 0, removed: 0, generated: GENERATED_PATH.test(l.text) })
      continue
    }
    const f = files.at(-1)
    if (!f) continue
    f.lines.push(l)
    if (l.kind === '+') f.added++
    if (l.kind === '-') f.removed++
  }
  for (const f of files) f.generated ||= f.lines.length > LONG_DIFF
  return files
}

export function DiffView({ id, run, live, className }: { id: string; run: SideRun; live?: boolean; className?: string }) {
  const [kind, setKind] = useState<'solution' | 'harness'>('solution')
  const { data, error, dataUpdatedAt, refetch, isFetching, isLoading } = useDiff(id, run.key, kind)
  const files = useMemo(() => splitByFile(data?.lines ?? []), [data])
  // Only the files the user opened or closed; the rest stay collapsed.
  const [toggled, setToggled] = useState<Record<string, boolean>>({})
  const isOpen = (f: FileDiff) => toggled[f.path] ?? false
  const setAll = (open: boolean) => setToggled(Object.fromEntries(files.map(f => [f.path, open])))
  const added = files.reduce((n, f) => n + f.added, 0)
  const removed = files.reduce((n, f) => n + f.removed, 0)

  return (
    <div className={cn('flex flex-col bg-term', className)}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-3.5 py-2 font-mono text-xs">
        <Segmented
          label="Which changes"
          value={kind}
          onChange={setKind}
          options={[
            { value: 'solution', label: 'Solution' },
            { value: 'harness', label: `Harness${run.harnessFiles.length ? ` · ${run.harnessFiles.length}` : ''}` },
          ]}
        />
        {files.length > 0 && (
          <span className="text-muted-foreground">
            {files.length} {files.length === 1 ? 'file' : 'files'} · <span className="text-ok">+{formatInt(added)}</span> <span className="text-danger">−{formatInt(removed)}</span>
          </span>
        )}
        <span className="ml-auto flex items-center gap-1">
          {files.length > 1 && (
            <>
              <Button size="sm" variant="ghost" onClick={() => setAll(true)}>Expand all</Button>
              <Button size="sm" variant="ghost" onClick={() => setAll(false)}>Collapse all</Button>
            </>
          )}
          {live && (
            <Button size="sm" variant="ghost" onClick={() => refetch()} disabled={isFetching}>
              {isFetching ? 'refreshing…' : `snapshot ${dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString('en-GB') : ''} · refresh`}
            </Button>
          )}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-auto font-mono text-xs leading-[1.6]">
        {error && <div className="p-3"><ErrorNote error={error} /></div>}
        {isLoading && <div className="p-3"><LoadingRows rows={4} /></div>}
        {data && !data.ready && <p className="p-3.5 text-muted-foreground">The changes can be read once the agent is running.</p>}
        {data?.ready && files.length === 0 && (
          <p className="p-3.5 text-muted-foreground">
            {kind === 'solution' ? 'No changes to the project.' : 'No changes to harness files (AGENTS.md, .claude/, opencode.json…).'}
          </p>
        )}
        {data && data.dependencyFiles > 0 && kind === 'solution' && (
          <p className="border-b bg-raise/50 px-3.5 py-1.5 text-dim">
            {formatInt(data.dependencyFiles)} changed {data.dependencyFiles === 1 ? 'file' : 'files'} in dependency folders (node_modules, .venv…) not shown: installed packages, not the agent's work.
          </p>
        )}
        {files.map(f => {
          const open = isOpen(f)
          return (
            <section key={f.path} className="border-b">
              <button
                type="button"
                aria-expanded={open}
                onClick={() => setToggled(t => ({ ...t, [f.path]: !open }))}
                className="sticky top-0 z-10 flex w-full items-center gap-2 border-b bg-raise px-3 py-1.5 text-left outline-none hover:bg-raise/80 focus-visible:ring-1 focus-visible:ring-ring"
              >
                {open ? <ChevronDown className="size-3.5 shrink-0 text-dim" /> : <ChevronRight className="size-3.5 shrink-0 text-dim" />}
                <span className="min-w-0 truncate font-semibold text-foreground">{f.path}</span>
                {f.generated && <Chip>{f.lines.length > LONG_DIFF ? 'long' : 'generated'}</Chip>}
                <span className="ml-auto shrink-0">
                  <span className="text-ok">+{formatInt(f.added)}</span> <span className="text-danger">−{formatInt(f.removed)}</span>
                </span>
              </button>
              {open && (
                <pre className="py-1">
                  {f.lines.map((l, i) => (
                    <div
                      key={i}
                      className={cn(
                        'px-3.5 whitespace-pre-wrap break-all',
                        l.kind === '+' && 'bg-ok/10 text-ok',
                        l.kind === '-' && 'bg-danger/10 text-danger',
                        l.kind === '@@' && 'text-dim',
                      )}
                    >
                      {l.text}
                    </div>
                  ))}
                </pre>
              )}
            </section>
          )
        })}
        {data?.truncated && <p className="px-3.5 py-2 text-warn">The diff is too long to show in full; download the side's files to see everything.</p>}
      </div>
    </div>
  )
}
