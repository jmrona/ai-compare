import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import type { SideKey, SideStatus } from '@/api/types'
import { TERMINAL_STATUSES } from '@/api/types'
import { STATUS_LABEL, STATUS_TONE, type Tone } from '@/lib/format'
import { Label } from '@/components/ui/label'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const TONE_TEXT: Record<Tone, string> = {
  a: 'text-side-a border-side-a/30',
  b: 'text-side-b border-side-b/30',
  ok: 'text-ok border-ok/30',
  warn: 'text-warn border-warn/30',
  danger: 'text-danger border-danger/30',
  dim: 'text-muted-foreground border-border',
}
const TONE_BG: Record<Tone, string> = {
  a: 'bg-side-a',
  b: 'bg-side-b',
  ok: 'bg-ok',
  warn: 'bg-warn',
  danger: 'bg-danger',
  dim: 'bg-dim',
}

/** Bordered block with a small upper-case header. */
export function Panel({ title, right, children, className, bodyClassName }: {
  title?: ReactNode
  right?: ReactNode
  children: ReactNode
  className?: string
  bodyClassName?: string
}) {
  return (
    <section className={cn('min-w-0 border bg-panel', className)}>
      {title && (
        <header className="flex min-h-[34px] flex-wrap items-center justify-between gap-2 border-b px-3 py-1.5">
          <h2 className="label-caps">{title}</h2>
          {right}
        </header>
      )}
      <div className={cn('p-3', bodyClassName)}>{children}</div>
    </section>
  )
}

export function SideTag({ side, small }: { side: SideKey; small?: boolean }) {
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center font-semibold text-[#111315]',
        side === 'A' ? 'bg-side-a' : 'bg-side-b',
        small ? 'size-4 text-[10px]' : 'size-[18px] text-[11px]',
      )}
      aria-label={`Side ${side}`}
    >
      {side}
    </span>
  )
}

export function Chip({ tone = 'dim', className, children, title }: { tone?: Tone; className?: string; children: ReactNode; title?: string }) {
  return (
    <span title={title} className={cn('inline-flex h-[18px] items-center gap-1 border px-1.5 text-[11.5px] whitespace-nowrap', TONE_TEXT[tone], className)}>
      {children}
    </span>
  )
}

export function Dot({ tone, live }: { tone: Tone; live?: boolean }) {
  return <span className={cn('inline-block size-[7px] shrink-0 rounded-full', TONE_BG[tone], live && 'live-pulse')} />
}

export function StatusLabel({ status }: { status: SideStatus }) {
  const tone = STATUS_TONE[status]
  const live = !TERMINAL_STATUSES.includes(status)
  return (
    <span className={cn('inline-flex items-center gap-1.5 text-xs', TONE_TEXT[tone].split(' ')[0])}>
      <Dot tone={tone} live={live} />
      {STATUS_LABEL[status]}
    </span>
  )
}

/** Small figure with a label, for pane headers and metrics. */
export function Metric({ label, value, sub, className }: { label: string; value: ReactNode; sub?: ReactNode; className?: string }) {
  return (
    <div className={cn('min-w-0', className)}>
      <div className="text-[10.5px] tracking-[0.07em] text-dim uppercase">{label}</div>
      <div className="tnum font-mono text-[15px] text-foreground">{value}</div>
      {sub && <div className="text-[11px] text-dim">{sub}</div>}
    </div>
  )
}

export function Field({ label, htmlFor, hint, children, className }: {
  label: string
  htmlFor?: string
  hint?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('grid min-w-0 gap-1', className)}>
      <Label htmlFor={htmlFor} className="text-[11px] font-normal tracking-[0.07em] text-dim uppercase">{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

/** Pick one of a few options (shadcn ToggleGroup). */
export function Segmented<T extends string>({ value, onChange, options, label }: {
  value: T
  onChange: (v: T) => void
  options: { value: T; label: string; disabled?: boolean }[]
  label: string
}) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      spacing={0}
      value={value}
      aria-label={label}
      onValueChange={v => v && onChange(v as T)}
      className="bg-term"
    >
      {options.map(o => (
        <ToggleGroupItem key={o.value} value={o.value} disabled={o.disabled} className="px-2.5 text-xs data-[state=on]:bg-raise data-[state=on]:text-foreground">
          {o.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

export function LoadingRows({ rows = 3 }: { rows?: number }) {
  return (
    <div className="grid gap-2" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => <div key={i} className="h-9 animate-pulse bg-raise" />)}
    </div>
  )
}

export function ErrorNote({ error }: { error: unknown }) {
  return (
    <div role="alert" className="border border-danger/30 bg-danger/5 p-3 text-sm text-danger">
      {error instanceof Error ? error.message : 'Something went wrong while loading the data.'}
    </div>
  )
}
