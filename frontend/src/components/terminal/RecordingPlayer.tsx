// Replays a side's terminal recording (asciicast v2) with its original timing.

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Play, RotateCcw } from 'lucide-react'
import { recordingUrl } from '@/api/http'
import type { SideKey, TerminalSource } from '@/api/types'
import { Button } from '@/components/ui/button'
import { ErrorNote, LoadingRows, Segmented } from '@/components/common/primitives'
import { TerminalView } from './TerminalView'

type Frame = [seconds: number, data: string]

/** Output frames of a recording; resize events are left to the viewer's own size. */
function parseCast(text: string): Frame[] {
  const frames: Frame[] = []
  for (const line of text.split('\n').slice(1)) {
    if (!line.trim()) continue
    try {
      const [t, kind, data] = JSON.parse(line) as [number, string, string]
      if (kind === 'o') frames.push([t, data])
    } catch {
      // A line cut off by a crash: skip it.
    }
  }
  return frames
}

/** Long silences are shortened, as asciinema does, so a replay does not stall. */
const MAX_PAUSE = 2

function player(frames: Frame[], speed: number): TerminalSource {
  return {
    subscribe(onData) {
      let i = 0
      let timer = 0
      let last = 0
      const step = () => {
        while (i < frames.length) {
          const [t, data] = frames[i]
          last = t
          i++
          onData(data)
          const next = frames[i]
          if (next) {
            const wait = (Math.min(next[0] - last, MAX_PAUSE) * 1000) / speed
            if (wait > 8) {
              timer = window.setTimeout(step, wait)
              return
            }
          }
        }
      }
      step()
      return () => window.clearTimeout(timer)
    },
    send() {},
    resize() {},
  }
}

const SPEEDS = ['1', '2', '4', '16'] as const

export function RecordingPlayer({ id, side, className }: { id: string; side: SideKey; className?: string }) {
  const { data, error, isLoading } = useQuery({
    queryKey: ['recording', id, side],
    queryFn: async () => {
      const res = await fetch(recordingUrl(id, side))
      if (!res.ok) throw new Error(`The recording could not be loaded (${res.status})`)
      return parseCast(await res.text())
    },
    staleTime: Infinity,
  })
  const [speed, setSpeed] = useState<(typeof SPEEDS)[number]>('2')
  const [run, setRun] = useState(0)
  const frames = data
  // A new source restarts the replay; the counter forces one for "Replay".
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const source = useMemo(() => (frames ? player(frames, Number(speed)) : null), [frames, speed, run])

  if (isLoading) return <div className="p-3"><LoadingRows rows={4} /></div>
  if (error || !source) return <div className="p-3"><ErrorNote error={error} /></div>
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b bg-term px-3 py-1.5 text-xs text-muted-foreground">
        <Play className="size-3.5" />Replay of the recording
        <span className="ml-auto flex items-center gap-2">
          <Segmented label="Replay speed" value={speed} onChange={setSpeed} options={SPEEDS.map(s => ({ value: s, label: `${s}×` }))} />
          <Button size="sm" variant="ghost" onClick={() => setRun(r => r + 1)}><RotateCcw className="size-3.5" />Restart</Button>
        </span>
      </div>
      <TerminalView key={`${speed}-${run}`} source={source} readOnly className={className} />
    </div>
  )
}
