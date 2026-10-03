import { toast } from 'sonner'
import type { Comparison, SideKey, SideStatus } from '@/api/types'
import { STATUS_LABEL, formatUsd } from '@/lib/format'

export interface NotifyTargets {
  openRun: (id: string) => void
  openReport: (id: string) => void
}

type Seen = { A: SideStatus; B: SideStatus; report: Comparison['report'] }

const ENDED: SideStatus[] = ['finished', 'error', 'cancelled', 'limit_reached']
const SIDES: SideKey[] = ['A', 'B']
const seen = new Map<string, Seen>()

export function notifyChanges(c: Comparison, to: NotifyTargets) {
  const now: Seen = { A: c.sides.A.status, B: c.sides.B.status, report: c.report }
  const before = seen.get(c.id)
  seen.set(c.id, now)
  if (!before) return

  for (const side of SIDES) {
    if (ENDED.includes(before[side]) || !ENDED.includes(now[side])) continue
    const run = c.sides[side]
    const show = run.status === 'finished' ? toast.success : run.status === 'error' ? toast.error : toast.warning
    show(`Side ${side} ${STATUS_LABEL[run.status]}`, {
      description: `#${c.id} · ${run.config.model} · ${formatUsd(run.metrics.costUsd)}`,
      action: { label: 'Open', onClick: () => to.openRun(c.id) },
    })
  }

  const ended = (s: Seen) => SIDES.every(k => ENDED.includes(s[k]))
  if (!ended(before) && ended(now)) chime()

  if (before.report !== 'ready' && now.report === 'ready') {
    toast.success('Report ready', {
      description: `#${c.id}${c.projectName ? ` · ${c.projectName}` : ''}`,
      action: { label: 'Open report', onClick: () => to.openReport(c.id) },
      duration: 15_000,
    })
  }
  if (before.report !== 'error' && now.report === 'error') {
    toast.error('The report could not be generated', {
      description: `#${c.id}`,
      action: { label: 'Open', onClick: () => to.openReport(c.id) },
    })
  }
}

let audio: AudioContext | null = null

function chime() {
  try {
    audio ??= new AudioContext()
    const start = audio.currentTime
    ;[660, 880].forEach((freq, i) => {
      const osc = audio!.createOscillator()
      const gain = audio!.createGain()
      const at = start + i * 0.18
      osc.type = 'sine'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, at)
      gain.gain.exponentialRampToValueAtTime(0.25, at + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.4)
      osc.connect(gain).connect(audio!.destination)
      osc.start(at)
      osc.stop(at + 0.45)
    })
  } catch {
    return
  }
}
