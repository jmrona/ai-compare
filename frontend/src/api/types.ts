// Types the UI works with. The wire format is the protobuf contract (proto/aicompare/v1, code in
// src/gen); convert.ts maps its messages to these, so pages never deal with protobuf details
// such as bigint or unset optionals.

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
  /** Each step is null until it finishes. Both sides share the copy. */
  phases: { copySec: number | null; buildSec: number | null; startSec: number | null; verifySec: number | null }
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

export interface TestRun {
  status: 'passed' | 'failed' | 'error'
  exitCode: number
  durationSec: number
}

export interface Tests {
  command: string
  /** null until the tests have run, or when they do not run. */
  visible: TestRun | null
  /** The same command with the hidden tests added; null without hidden tests. */
  hidden: TestRun | null
  skippedReason: string
}

export interface SideRun {
  key: SideKey
  config: SideConfig
  cliVersion: string
  status: SideStatus
  endReason: string
  /** For status "error": whether the agent or ai-compare/Docker failed. */
  failure: '' | 'agent' | 'infrastructure'
  metrics: SideMetrics
  /** Files the agent changed, harness files excluded. */
  files: FileChange[]
  harnessFiles: FileChange[]
  tests: Tests
  /** Snapshot of the models.dev price taken when the side started. */
  priceSnapshot: { price: Price | null; fetchedAt: string }
  /** The side's files are saved and can be downloaded. */
  hasResult: boolean
  /** A timed terminal recording exists. */
  hasRecording: boolean
}

export type ReportStatus = 'none' | 'generating' | 'ready' | 'error'

export interface Comparison {
  id: string
  createdAt: string
  /** Empty for comparisons that start from an empty folder. */
  projectPath: string
  projectName: string
  prompt: string
  harness: string
  profile: ProjectProfile
  sides: Record<SideKey, SideRun>
  report: ReportStatus
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
  source: 'copy' | 'build' | 'run' | 'verify' | 'proxy'
  message: string
}

export interface DiffLine {
  kind: '+' | '-' | ' ' | '@@' | 'file'
  text: string
}

export interface SideDiff {
  files: FileChange[]
  lines: DiffLine[]
  truncated: boolean
  /** false while the changes cannot be read yet (the side has not started). */
  ready: boolean
  /** Changed files in dependency folders (node_modules, .venv…), left out: not the agent's work. */
  dependencyFiles: number
}

export interface TimelineEvent {
  at: string
  kind: 'prompt' | 'message' | 'tool' | 'patch' | 'error' | string
  detail: string
}

export interface Timeline {
  events: TimelineEvent[]
  /** The CLI's own token count, to cross-check the proxy's. */
  sessionUsage: Usage | null
  sessionCostUsd: number | null
  /** false until the side has ended and its CLI session was read. */
  ready: boolean
}

export interface TestOutput {
  tests: Tests
  visibleOutput: string
  hiddenOutput: string
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
  status: 'generating' | 'ready' | 'error'
  error: string
  model: string
  costUsd: number | null
  verdicts: { label: string; side: SideKey | null }[]
  conclusions: string[]
  perSide: Record<SideKey, string>
  findings: Finding[]
  warnings: string[]
}

/** Phase 2 preview: presets are sample data for now. */
export interface Preset {
  slug: string
  title: string
  description: string
  clis: Cli[]
  uses: number
  files: { root: 'project' | 'home'; path: string; category: string }[]
}

export interface Settings {
  /** Read-only: which keys are set in .env. */
  keys: Record<'openai' | 'anthropic', boolean>
  defaultLimits: Limits
  /** Read-only: offered when a limit is switched on. */
  suggestedLimits: { timeoutMin: number; maxTokensK: number; maxCostUsd: number }
  reportModel: string
  autoReport: boolean
  resources: { cpus: number; memoryGb: number }
  /** Read-only (phase 2). */
  localBaseUrl: string
  /** Read-only. */
  cliVersions: { cli: Cli; pinned: string | null; latest: string | null }[]
  /** Days after which ai-compare's containers, images and copies are removed; artefacts stay. */
  retentionDays: number
  /** Read-only: disk used by what ai-compare created. */
  disk: { label: string; bytes: number }[]
}

/**
 * A terminal channel (a WebSocket, or a recording being replayed). subscribe() first delivers
 * the accumulated output, so a reconnect rebuilds the screen.
 */
export interface TerminalSource {
  subscribe(onData: (chunk: string) => void): () => void
  send(data: string): void
  resize(cols: number, rows: number): void
}
