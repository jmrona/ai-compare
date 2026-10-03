// The Harness tab: the harness files a side ran with (its preset snapshot or the project's own
// files, as they were when it started), read-only. Agents are not meant to change them; if one
// did, the tab says so and shows those changes.

import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'
import type { SideRun } from '@/api/types'
import { useHarness } from '@/api/queries'
import { formatInt, harnessLabel } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Chip, ErrorNote, LoadingRows } from '@/components/common/primitives'
import { FileTree } from '@/components/common/FileTree'
import { DiffView } from './DiffView'

export function HarnessView({ id, run, className }: { id: string; run: SideRun; className?: string }) {
  const { data, error, isLoading } = useHarness(id, run.key)
  const [picked, setPicked] = useState<string | null>(null)
  const [showChanges, setShowChanges] = useState(false)
  const files = data?.files ?? []
  const current = files.find(f => treePath(f) === picked) ?? files.find(f => f.root === 'project' && f.path === 'AGENTS.md') ?? files[0]
  const changed = run.harnessFiles.length

  return (
    <div className={cn('flex flex-col bg-term', className)}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-3.5 py-2 font-mono text-xs">
        <Chip>{harnessLabel(run.config.harness)}</Chip>
        {data && <span className="text-muted-foreground">{files.length} {files.length === 1 ? 'file' : 'files'} · as they were when the side started</span>}
        {changed > 0 && (
          <span className="ml-auto flex items-center gap-2">
            <span className="text-warn">the agent changed {changed} harness {changed === 1 ? 'file' : 'files'}</span>
            <Button size="sm" variant="ghost" onClick={() => setShowChanges(v => !v)}>{showChanges ? 'Show the files' : 'Show the changes'}</Button>
          </span>
        )}
      </div>

      {showChanges ? (
        <DiffView id={id} run={run} kind="harness" className="min-h-0 flex-1" />
      ) : (
        <div className="flex min-h-0 flex-1">
          {error && <div className="p-3"><ErrorNote error={error} /></div>}
          {isLoading && <div className="w-full p-3"><LoadingRows rows={4} /></div>}
          {data && !data.available && <p className="p-3.5 text-[13px] text-muted-foreground">This comparison was made before harness files were kept with it, and its project copy has been removed.</p>}
          {data?.available && files.length === 0 && (
            <p className="p-3.5 text-[13px] text-muted-foreground">
              {run.config.harness.kind === 'none' ? 'This side ran without harness files.' : 'No harness files: the agent ran without project instructions.'}
            </p>
          )}
          {files.length > 0 && (
            <>
              <nav aria-label="Harness files" className="w-64 shrink-0 overflow-y-auto border-r py-1">
                <FileTree files={files.map(f => ({ path: treePath(f) }))} selected={current && treePath(current)} onSelect={setPicked} />
              </nav>
              <div className="min-w-0 flex-1 overflow-auto">
                {current && <FileContent file={current} />}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

const treePath = (f: { root: string; path: string }) => (f.root === 'home' ? `~/${f.path}` : f.path)

function FileContent({ file }: { file: { root: string; path: string; size: number; content: string | null } }) {
  if (file.content == null) {
    return <p className="p-3.5 text-[13px] text-muted-foreground">Binary or too large to show ({formatInt(file.size)} bytes).</p>
  }
  if (/\.(md|mdx|markdown)$/i.test(file.path)) {
    return (
      <article className="prose-preset mx-auto w-full max-w-[75ch] px-6 py-5 text-sm leading-[1.7] text-foreground/90">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{file.content}</ReactMarkdown>
      </article>
    )
  }
  return <pre className="p-4 font-mono text-[12.5px] leading-relaxed whitespace-pre-wrap break-all">{file.content}</pre>
}
