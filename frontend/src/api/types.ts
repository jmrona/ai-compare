// Domain types shared by the UI and the API client.
// Once the OpenAPI contract exists, these will be generated from it.

export type Cli = 'opencode' | 'codex' | 'claude'
export type ProviderId = 'openai' | 'anthropic' | 'local'
export type Mode = 'interactive' | 'autonomous'
/** Reasoning effort as the model names it on models.dev (none, low, medium, high, xhigh…). */
export type Effort = string
export type SideKey = 'A' | 'B'

/** null = no limit. Every limit is optional. */
export interface Limits {
  timeoutMin: number | null
  maxTokensK: number | null
  maxCostUsd: number | null
}

export interface SideConfig {
  cli: Cli
  provider: ProviderId
  model: string
  effort: Effort
  mode: Mode
  limits: Limits
}

export type SideStatus =
  | 'pending'
  | 'copying'
  | 'building'
  | 'starting'
  | 'running'
  | 'waiting_input'
  | 'verifying'
  | 'finished'
  | 'error'
  | 'cancelled'
  | 'limit_reached'

export const TERMINAL_STATUSES: SideStatus[] = ['finished', 'error', 'cancelled', 'limit_reached']

/** Tokens per category. null = the provider does not report it (never shown as 0). */
export interface Usage {
  input: number
  cacheRead: number
  cacheWrite: number | null
  output: number
}

/** USD per million tokens, as published by models.dev. */
export interface Price {
  input: number
  cacheRead: number | null
  cacheWrite: number | null
  output: number
}

export interface SideMetrics {
  elapsedSec: number
  agentSec: number
  humanWaitSec: number | null
  prepSec: number
  /** prepSec split by step; each is null until it finishes. Both sides share the copy. */
  phases?: { copySec: number | null; buildSec: number | null; startSec: number | null }
  usage: Usage
  /** null = cannot be calculated (model has no price on models.dev). */
  costUsd: number | null
  costConfirmedUsd: number | null
  requests: number
  retries: number
  errors: number
  tokensPerSec: number | null
}

export interface FileChange {
  path: string
  added: number
  removed: number
}

export interface TestResult {
  passed: number
  total: number
  hiddenPassed?: number
  hiddenTotal?: number
}

export interface SideRun {
  key: SideKey
  config: SideConfig
  cliVersion: string
  status: SideStatus
  endReason?: string
  metrics: SideMetrics
  files: FileChange[]
  tests?: TestResult
  /** Snapshot of the models.dev price taken when the comparison started. */
  priceSnapshot: { price: Price | null; fetchedAt: string }
}

export interface Comparison {
  id: string
  createdAt: string
  projectPath: string
  projectName: string
  prompt: string
  harness: string
  sides: Record<SideKey, SideRun>
  report: 'none' | 'generating' | 'ready'
}

export interface HarnessFile {
  path: string
  readBy: Cli[]
}

export interface ProjectProfile {
  runtime: string
  setup: string
  test: string
  hiddenTestsPath: string
}

/** One folder of the folder browser. */
export interface FolderListing {
  path: string
  /** Empty at the top-level folder (C:\, /Users…), above which Docker cannot see. */
  parent: string
  folders: { name: string; path: string; isGit: boolean }[]
}

export interface ProjectInspection {
  path: string
  name: string
  isGit: boolean
  fileCount: number
  sizeBytes: number
  harnessFiles: HarnessFile[]
  excluded: string[]
  profile: ProjectProfile
}

export interface ModelInfo {
  id: string
  name: string
  provider: ProviderId
  family?: string
  /** YYYY-MM-DD. The catalogue is sorted by this, newest first. */
  releaseDate?: string
  deprecated: boolean
  /** Coding agents need tool calling and text output. */
  toolCall: boolean
  textOutput: boolean
  /** Effort values the model accepts; empty when it has no effort setting. */
  efforts: string[]
  contextK: number
  price: Price | null
  /** Price once the prompt exceeds aboveTokens, when the model has one. */
  longContext?: { aboveTokens: number; price: Price }
}

export interface Catalog {
  source: 'models.dev'
  /** Last time models.dev confirmed this data. */
  fetchedAt: string
  /** true when models.dev could not be reached and this is the saved copy. */
  fromCache: boolean
  warning?: string
  models: ModelInfo[]
}

export interface NewComparison {
  projectPath: string
  profile: ProjectProfile
  prompt: string
  sides: Record<SideKey, SideConfig>
}

export interface LogEntry {
  at: string
  level: 'info' | 'warn' | 'error'
  source: 'copy' | 'build' | 'run' | 'proxy'
  message: string
}

export interface DiffLine {
  kind: '+' | '-' | ' ' | '@@'
  text: string
}

export interface SideDiff {
  files: FileChange[]
  lines: DiffLine[]
}

export interface TimelineEvent {
  at: string
  kind: string
  detail: string
}

export interface Finding {
  severity: 'high' | 'medium' | 'low'
  side: SideKey
  title: string
  impact: string
  location: string
}

export interface Report {
  comparisonId: string
  model: string
  costUsd: number
  verdicts: { label: string; side: SideKey | null }[]
  conclusions: string[]
  perSide: Record<SideKey, string>
  findings: Finding[]
}

export interface Preset {
  slug: string
  title: string
  description: string
  clis: Cli[]
  uses: number
  files: { root: 'project' | 'home'; path: string; category: string }[]
}

export interface Settings {
  keys: Record<'openai' | 'anthropic', boolean>
  defaultLimits: Limits
  suggestedLimits: { timeoutMin: number; maxTokensK: number; maxCostUsd: number }
  reportModel: string
  autoReport: boolean
  resources: { cpus: number; memoryGb: number }
  localBaseUrl: string
  cliVersions: { cli: Cli; pinned: string | null; latest: string | null }[]
  retention: { keepStopped: boolean; containersDays: number; imagesDays: number; recordingsDays: number }
  disk: { label: string; gb: number }[]
}

/**
 * A terminal channel. In production it is a WebSocket; with mocks, an in-memory player.
 * subscribe() first delivers the accumulated output, so a reconnect rebuilds the screen.
 */
export interface TerminalSource {
  subscribe(onData: (chunk: string) => void): () => void
  send(data: string): void
  resize(cols: number, rows: number): void
}
