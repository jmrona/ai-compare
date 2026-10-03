import type { Mode, Price, SideStatus, Usage } from '@/api/types'

const LOCALE = 'en-GB'
const intFmt = new Intl.NumberFormat(LOCALE)
const compactFmt = new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 1 })
const usdFmt = new Intl.NumberFormat(LOCALE, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const priceFmt = new Intl.NumberFormat(LOCALE, { minimumFractionDigits: 2, maximumFractionDigits: 3 })

export const formatInt = (n: number) => intFmt.format(n)

/** 412300 → "412.3k" */
export function formatTokens(n: number): string {
  if (n >= 1_000_000) return compactFmt.format(n / 1_000_000) + 'M'
  if (n >= 1_000) return compactFmt.format(n / 1_000) + 'k'
  return intFmt.format(n)
}

const smallUsdFmt = new Intl.NumberFormat(LOCALE, { maximumSignificantDigits: 2 })

/** $1.84; amounts under a cent keep two significant digits ($0.0048) instead of showing $0.00. */
export function formatUsd(n: number | null): string {
  if (n == null) return 'n/a'
  if (n > 0 && n < 0.01) return '$' + smallUsdFmt.format(n)
  return '$' + usdFmt.format(n)
}

/** 75.86 → "76"; null → "—" */
export const formatRate = (n: number | null) => (n == null ? '—' : String(Math.round(n)))

export const formatPrice = (n: number) => priceFmt.format(n)

/** Short durations with decimals: 0.42 → "0.4 s", 54.3 → "54 s", 75 → "1:15"; null → "—". */
export function formatSeconds(sec: number | null | undefined): string {
  if (sec == null) return '—'
  if (sec < 10) return `${sec.toFixed(1)} s`
  if (sec < 60) return `${Math.round(sec)} s`
  return formatDuration(sec)
}

/** 761 → "12:41" */
export function formatDuration(sec: number | null): string {
  if (sec == null) return '—'
  const s = Math.max(0, Math.round(sec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const r = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${r}` : `${m}:${r}`
}

export function formatBytes(bytes: number): string {
  if (bytes >= 1024 ** 3) return compactFmt.format(bytes / 1024 ** 3) + ' GB'
  if (bytes >= 1024 ** 2) return compactFmt.format(bytes / 1024 ** 2) + ' MB'
  return compactFmt.format(bytes / 1024) + ' KB'
}

const timeFmt = new Intl.DateTimeFormat(LOCALE, { hour: '2-digit', minute: '2-digit' })
const dayFmt = new Intl.DateTimeFormat(LOCALE, { weekday: 'long', day: 'numeric', month: 'short', year: 'numeric' })
const dateTimeFmt = new Intl.DateTimeFormat(LOCALE, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })

export const formatTime = (iso: string) => timeFmt.format(new Date(iso))
export const formatDay = (iso: string) => dayFmt.format(new Date(iso))
export const formatDateTime = (iso: string) => dateTimeFmt.format(new Date(iso))

export function totalTokens(u: Usage): number {
  return u.input + u.cacheRead + (u.cacheWrite ?? 0) + u.output
}

/** Estimated cost in USD from a models.dev price (USD per million). null without a price. */
export function estimateCost(u: Usage, price: Price | null): number | null {
  if (!price) return null
  const perM = (tokens: number, rate: number | null) => (rate == null ? 0 : (tokens * rate) / 1_000_000)
  return perM(u.input, price.input) + perM(u.cacheRead, price.cacheRead ?? price.input) + perM(u.cacheWrite ?? 0, price.cacheWrite) + perM(u.output, price.output)
}

export const modeLabel = (m: Mode) => (m === 'interactive' ? 'interactive' : 'autonomous')

export const STATUS_LABEL: Record<SideStatus, string> = {
  pending: 'pending',
  copying: 'copying project',
  building: 'building image',
  starting: 'starting',
  running: 'running',
  waiting_input: 'waiting for input',
  verifying: 'verifying',
  finished: 'finished',
  error: 'error',
  cancelled: 'cancelled',
  limit_reached: 'limit reached',
}

export type Tone = 'a' | 'b' | 'ok' | 'warn' | 'danger' | 'dim'

export const STATUS_TONE: Record<SideStatus, Tone> = {
  pending: 'dim',
  copying: 'dim',
  building: 'dim',
  starting: 'dim',
  running: 'a',
  waiting_input: 'warn',
  verifying: 'a',
  finished: 'ok',
  error: 'danger',
  cancelled: 'dim',
  limit_reached: 'warn',
}
