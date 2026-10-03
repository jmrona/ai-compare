import type { Catalog, ModelInfo, ProviderId } from '@/api/types'

/** Models a coding agent can use: current, with tool calling and text output. Keeps the catalogue order (newest first). */
export function agentModels(catalog: Catalog | undefined, provider: ProviderId): ModelInfo[] {
  return catalog?.models.filter(m => m.provider === provider && !m.deprecated && m.toolCall && m.textOutput) ?? []
}

/** The effort to preselect for a model: keep the current one if the model accepts it, otherwise prefer "medium". */
export function pickEffort(model: ModelInfo | undefined, current?: string): string {
  const efforts = model?.efforts ?? []
  if (efforts.length === 0) return ''
  if (current && efforts.includes(current)) return current
  if (efforts.includes('medium')) return 'medium'
  return efforts[Math.floor(efforts.length / 2)]
}

const releaseFmt = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })

/** "2026-09-29" → "29 Sept 2026" */
export function formatRelease(date: string | undefined): string {
  if (!date) return '—'
  const d = new Date(date + 'T00:00:00Z')
  return Number.isNaN(d.getTime()) ? date : releaseFmt.format(d)
}
