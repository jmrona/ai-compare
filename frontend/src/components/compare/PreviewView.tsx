// The Preview tab: a side's application, once the side has ended. Static files are served as
// they are; with a preview command in the profile, its container is started on demand.

import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useQuery } from '@connectrpc/connect-query'
import { ExternalLink, Play, RotateCw, Square } from 'lucide-react'
import { ComparisonService } from '@/gen/aicompare/v1/comparison_pb'
import { clients } from '@/api/transport'
import { methodKey } from '@/api/queries'
import type { Comparison, SideRun } from '@/api/types'
import { TERMINAL_STATUSES } from '@/api/types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Chip, ErrorNote } from '@/components/common/primitives'

export function PreviewView({ comparison: c, run, className }: { comparison: Comparison; run: SideRun; className?: string }) {
  const qc = useQueryClient()
  const input = { id: c.id, side: run.key }
  const { data } = useQuery(ComparisonService.method.getPreview, input, {
    select: r => r.preview,
    // Only while it boots: once running or stopped nothing changes on its own.
    refetchInterval: q => (q.state.data?.preview?.status === 'starting' ? 1500 : false),
  })
  const refresh = () => qc.invalidateQueries({ predicate: methodKey('GetPreview', c.id) })
  const start = useMutation({ mutationFn: () => clients.comparisons.startPreview(input), onSettled: refresh })
  const stop = useMutation({ mutationFn: () => clients.comparisons.stopPreview(input), onSettled: refresh })
  const [frameKey, setFrameKey] = useState(0)
  const ended = TERMINAL_STATUSES.includes(run.status)
  const kind = c.profile.previewCommand ? 'command' : 'static'
  const status = data?.status ?? 'stopped'

  if (!ended || !run.hasResult) {
    return (
      <div className={cn('grid place-content-center gap-2 bg-term p-6 text-center text-[13px] text-muted-foreground', className)}>
        <p>{ended ? 'This side has no saved files to preview.' : 'The preview is available once the side has ended.'}</p>
        <p className="text-xs text-dim">
          {kind === 'command' ? <>It will run <span className="font-mono">{c.profile.previewCommand}</span> on port {c.profile.previewPort}.</> : 'Its files will be served as a static site (set a preview command in the project profile for apps that need a server).'}
        </p>
      </div>
    )
  }

  return (
    <div className={cn('flex flex-col bg-term', className)}>
      <div className="flex flex-wrap items-center gap-2 border-b px-3 py-1.5 text-xs">
        <Chip tone={status === 'running' ? 'ok' : status === 'error' ? 'danger' : status === 'starting' ? 'warn' : 'dim'}>{status}</Chip>
        <span className="text-muted-foreground">{kind === 'command' ? <span className="font-mono">{c.profile.previewCommand} · :{c.profile.previewPort}</span> : 'static files'}</span>
        {data?.url && status === 'running' && (
          <a href={data.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-mono text-side-a hover:underline">
            {data.url}<ExternalLink className="size-3" />
          </a>
        )}
        <span className="ml-auto flex gap-1">
          {status === 'running' && <Button size="sm" variant="ghost" onClick={() => setFrameKey(k => k + 1)}><RotateCw className="size-3.5" />Reload</Button>}
          {status === 'running' || status === 'starting' ? (
            <Button size="sm" variant="ghost" disabled={stop.isPending} onClick={() => stop.mutate()}><Square className="size-3" />Stop</Button>
          ) : (
            <Button size="sm" disabled={start.isPending} onClick={() => start.mutate()}><Play className="size-3.5" />{status === 'error' ? 'Try again' : 'Start preview'}</Button>
          )}
        </span>
      </div>
      {start.error && <div className="p-3"><ErrorNote error={start.error} /></div>}
      {status === 'running' && data?.url ? (
        <iframe key={frameKey} title={`Preview of side ${run.key}`} src={data.url} className="min-h-0 w-full flex-1 border-0 bg-white" />
      ) : status === 'starting' ? (
        <p className="p-4 text-[13px] text-muted-foreground">Starting the application… dev servers can take a minute to compile.</p>
      ) : status === 'error' ? (
        <div className="min-h-0 flex-1 overflow-auto p-3">
          <p className="text-[13px] text-danger">{data?.error}</p>
          {data?.logs && <pre className="mt-2 font-mono text-xs whitespace-pre-wrap text-muted-foreground">{data.logs}</pre>}
        </div>
      ) : (
        <p className="p-4 text-[13px] text-muted-foreground">
          {kind === 'command'
            ? 'Start the preview to run this side\'s application in a fresh container from its result.'
            : 'Start the preview to serve the files this side produced.'}
          {' '}Each side opens on its own address, e.g. <span className="font-mono">{run.key.toLowerCase()}-{c.id}.localhost</span>.
        </p>
      )}
    </div>
  )
}
