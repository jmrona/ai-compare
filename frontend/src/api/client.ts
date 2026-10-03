import type {
  Catalog,
  Comparison,
  LogEntry,
  NewComparison,
  Preset,
  ProjectInspection,
  Report,
  Settings,
  SideDiff,
  SideKey,
  TerminalSource,
  TimelineEvent,
} from './types'
import { httpClient } from './http'
import { mockClient } from './mock/client'

/** Everything the UI asks the backend for. There is an HTTP implementation and an in-memory one. */
export interface ApiClient {
  getCatalog(): Promise<Catalog>
  refreshCatalog(): Promise<Catalog>
  inspectProject(path: string): Promise<ProjectInspection>
  startComparison(input: NewComparison): Promise<{ id: string }>
  getActiveComparison(): Promise<Comparison | null>
  getComparison(id: string): Promise<Comparison>
  finishSide(id: string, side: SideKey): Promise<void>
  cancelSide(id: string, side: SideKey): Promise<void>
  getLogs(id: string, side: SideKey): Promise<LogEntry[]>
  getDiff(id: string, side: SideKey): Promise<SideDiff>
  getTimeline(id: string, side: SideKey): Promise<TimelineEvent[]>
  getTestOutput(id: string, side: SideKey): Promise<string[]>
  listHistory(): Promise<Comparison[]>
  deleteComparison(id: string): Promise<void>
  getReport(id: string): Promise<Report | null>
  generateReport(id: string): Promise<void>
  listPresets(): Promise<Preset[]>
  getPreset(slug: string): Promise<Preset>
  getPresetFile(slug: string, path: string): Promise<string>
  getSettings(): Promise<Settings>
  updateSettings(patch: Partial<Settings>): Promise<Settings>
  /** Live during a run; in the history it replays the recording. */
  openTerminal(id: string, side: SideKey): TerminalSource
}

export const useMocks = import.meta.env.VITE_USE_MOCKS !== 'false'

/** Comparisons run by the backend have ids starting with "r"; the sample data uses numbers. */
export const isRealComparison = (id: string) => id.startsWith('r')

const notYet = (what: string) => () => Promise.reject(new Error(`${what} is not available for real runs yet.`))

/**
 * With VITE_USE_MOCKS=true the backend serves what it already implements (catalogue, project
 * inspection, starting and following comparisons, terminals) and the sample data fills the
 * rest (history samples, reports, presets, settings). Each piece moves to the backend as it
 * is built.
 */
const hybridClient: ApiClient = {
  ...mockClient,
  getCatalog: httpClient.getCatalog,
  refreshCatalog: httpClient.refreshCatalog,
  inspectProject: httpClient.inspectProject,
  startComparison: httpClient.startComparison,
  getActiveComparison: httpClient.getActiveComparison,
  getComparison: id => (isRealComparison(id) ? httpClient.getComparison(id) : mockClient.getComparison(id)),
  finishSide: (id, side) => (isRealComparison(id) ? httpClient.finishSide(id, side) : mockClient.finishSide(id, side)),
  cancelSide: (id, side) => (isRealComparison(id) ? httpClient.cancelSide(id, side) : mockClient.cancelSide(id, side)),
  getLogs: (id, side) => (isRealComparison(id) ? httpClient.getLogs(id, side) : mockClient.getLogs(id, side)),
  getDiff: (id, side) => (isRealComparison(id) ? notYet('The diff')() : mockClient.getDiff(id, side)),
  getTimeline: (id, side) => (isRealComparison(id) ? notYet('The event timeline')() : mockClient.getTimeline(id, side)),
  getTestOutput: (id, side) => (isRealComparison(id) ? notYet('Test results')() : mockClient.getTestOutput(id, side)),
  openTerminal: (id, side) => (isRealComparison(id) ? httpClient.openTerminal(id, side) : mockClient.openTerminal(id, side)),
  async listHistory() {
    const [real, samples] = await Promise.all([httpClient.listHistory().catch(() => []), mockClient.listHistory()])
    const done = real.filter(c => ['finished', 'error', 'cancelled', 'limit_reached'].includes(c.sides.A.status) && ['finished', 'error', 'cancelled', 'limit_reached'].includes(c.sides.B.status))
    return [...done, ...samples].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  },
}

export const api: ApiClient = useMocks ? hybridClient : httpClient
