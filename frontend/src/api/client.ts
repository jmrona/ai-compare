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

// The models.dev catalogue is always real (the backend serves it); the rest is mocked
// until the backend implements it.
export const api: ApiClient = useMocks
  ? { ...mockClient, getCatalog: httpClient.getCatalog, refreshCatalog: httpClient.refreshCatalog }
  : httpClient
