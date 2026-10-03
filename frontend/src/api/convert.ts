// Maps protobuf messages (src/gen) to the UI's types (types.ts): bigint to number, Timestamp to
// ISO strings, unset optionals to null.

import { create } from '@bufbuild/protobuf'
import type { Timestamp } from '@bufbuild/protobuf/wkt'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import type * as cat from '@/gen/aicompare/v1/catalog_pb'
import type * as cmp from '@/gen/aicompare/v1/comparison_pb'
import { LimitsSchema, SideConfigSchema } from '@/gen/aicompare/v1/comparison_pb'
import type * as prj from '@/gen/aicompare/v1/project_pb'
import type * as pre from '@/gen/aicompare/v1/preset_pb'
import type * as rep from '@/gen/aicompare/v1/report_pb'
import type * as set from '@/gen/aicompare/v1/settings_pb'
import { SettingsSchema } from '@/gen/aicompare/v1/settings_pb'
import type {
  Catalog,
  Cli,
  Comparison,
  FileChange,
  FolderListing,
  HarnessChoice,
  Limits,
  LogEntry,
  Preset,
  PresetRoot,
  Price,
  ProjectInspection,
  ProjectProfile,
  ProviderId,
  Report,
  ReportStatus,
  Settings,
  SideConfig,
  SideDiff,
  SideKey,
  SideRun,
  SideStatus,
  TestOutput,
  TestRun,
  Tests,
  Timeline,
  Usage,
} from './types'

const iso = (t: Timestamp | undefined) => (t ? timestampDate(t).toISOString() : '')
const num = (v: bigint | undefined | null) => (v == null ? null : Number(v))
const opt = (v: number | undefined) => (v == null ? null : v)

/* ── Catalogue and projects ───────────────────────────────── */

export function priceFromProto(p: cat.Price | undefined): Price | null {
  if (!p) return null
  return { input: p.input, cacheRead: opt(p.cacheRead), cacheWrite: opt(p.cacheWrite), output: p.output }
}

export function catalogFromProto(c: cat.Catalog | undefined): Catalog {
  if (!c) throw new Error('The catalogue response is empty')
  return {
    source: 'models.dev',
    fetchedAt: iso(c.fetchedAt),
    fromCache: c.fromCache,
    warning: c.warning || undefined,
    models: c.models.map(m => ({
      id: m.id,
      name: m.name,
      provider: m.provider as ProviderId,
      family: m.family || undefined,
      releaseDate: m.releaseDate || undefined,
      deprecated: m.deprecated,
      toolCall: m.toolCall,
      textOutput: m.textOutput,
      efforts: m.efforts,
      contextK: m.contextK,
      price: priceFromProto(m.price),
      longContext: m.longContext?.price ? { aboveTokens: m.longContext.aboveTokens, price: priceFromProto(m.longContext.price)! } : undefined,
    })),
  }
}

const profileFromProto = (p: prj.ProjectProfile | undefined): ProjectProfile => ({
  runtime: p?.runtime ?? '',
  setup: p?.setup ?? '',
  test: p?.test ?? '',
  hiddenTestsPath: p?.hiddenTestsPath ?? '',
  previewCommand: p?.previewCommand ?? '',
  previewPort: p?.previewPort || null,
})

export function inspectionFromProto(i: prj.ProjectInspection | undefined): ProjectInspection {
  if (!i) throw new Error('The inspection response is empty')
  return {
    path: i.path,
    name: i.name,
    isGit: i.isGit,
    fileCount: Number(i.fileCount),
    sizeBytes: Number(i.sizeBytes),
    harnessFiles: i.harnessFiles.map(f => ({ path: f.path, readBy: f.readBy as Cli[] })),
    excluded: i.excluded,
    profile: profileFromProto(i.profile),
  }
}

export const foldersFromProto = (r: prj.ListFoldersResponse): FolderListing => ({
  path: r.path,
  parent: r.parent,
  folders: r.folders.map(f => ({ name: f.name, path: f.path, isGit: f.isGit })),
})

/* ── Comparisons ──────────────────────────────────────────── */

const limitsFromProto = (l: cmp.Limits | undefined): Limits => ({
  timeoutMin: opt(l?.timeoutMin),
  maxTokensK: opt(l?.maxTokensK),
  maxCostUsd: opt(l?.maxCostUsd),
})

export const limitsToProto = (l: Limits) =>
  create(LimitsSchema, { timeoutMin: l.timeoutMin ?? undefined, maxTokensK: l.maxTokensK ?? undefined, maxCostUsd: l.maxCostUsd ?? undefined })

export const sideConfigToProto = (c: SideConfig) =>
  create(SideConfigSchema, {
    cli: c.cli, provider: c.provider, model: c.model, effort: c.effort, mode: c.mode, limits: limitsToProto(c.limits),
    harness: { kind: c.harness.kind, preset: c.harness.kind === 'preset' ? c.harness.preset : '' },
  })

const harnessFromProto = (h: cmp.Harness | undefined): HarnessChoice => ({
  kind: h?.kind === 'preset' || h?.kind === 'none' ? h.kind : 'project',
  preset: h?.preset ?? '',
  title: h?.title ?? '',
  hash: h?.hash ?? '',
})

export const presetFromProto = (p: pre.Preset | undefined): Preset => {
  if (!p) throw new Error('The preset response is empty')
  return {
    slug: p.slug,
    title: p.title,
    description: p.description,
    clis: p.clis as Cli[],
    notes: p.notes,
    files: p.files.map(f => ({ root: f.root as PresetRoot, path: f.path, size: Number(f.size), category: f.category })),
    updatedAt: iso(p.updatedAt),
    uses: p.uses,
    hash: p.hash,
  }
}

export const usageFromProto = (u: cmp.Usage | undefined): Usage => ({
  input: Number(u?.input ?? 0n),
  cacheRead: Number(u?.cacheRead ?? 0n),
  cacheWrite: num(u?.cacheWrite),
  output: Number(u?.output ?? 0n),
})

const filesFromProto = (fs: cmp.FileChange[]): FileChange[] => fs.map(f => ({ path: f.path, added: f.added, removed: f.removed }))

const testRunFromProto = (t: cmp.TestRun | undefined): TestRun | null =>
  t ? { status: t.status as TestRun['status'], exitCode: t.exitCode, durationSec: t.durationSec } : null

export const testsFromProto = (t: cmp.Tests | undefined): Tests => ({
  command: t?.command ?? '',
  visible: testRunFromProto(t?.visible),
  hidden: testRunFromProto(t?.hidden),
  skippedReason: t?.skippedReason ?? '',
})

function sideFromProto(s: cmp.Side | undefined, key: SideKey): SideRun {
  const c = s?.config
  const m = s?.metrics
  return {
    key,
    config: {
      cli: (c?.cli ?? 'opencode') as Cli,
      provider: (c?.provider ?? 'openai') as ProviderId,
      model: c?.model ?? '',
      effort: c?.effort ?? '',
      mode: c?.mode === 'interactive' ? 'interactive' : 'autonomous',
      limits: limitsFromProto(c?.limits),
      harness: harnessFromProto(c?.harness),
    },
    cliVersion: s?.cliVersion ?? '',
    status: (s?.status ?? 'pending') as SideStatus,
    endReason: s?.endReason ?? '',
    failure: (s?.failure ?? '') as SideRun['failure'],
    metrics: {
      elapsedSec: m?.elapsedSec ?? 0,
      agentSec: m?.agentSec ?? 0,
      humanWaitSec: opt(m?.humanWaitSec),
      prepSec: m?.prepSec ?? 0,
      phases: {
        copySec: opt(m?.phases?.copySec),
        buildSec: opt(m?.phases?.buildSec),
        startSec: opt(m?.phases?.startSec),
        verifySec: opt(m?.phases?.verifySec),
      },
      usage: usageFromProto(m?.usage),
      costUsd: opt(m?.costUsd),
      costConfirmedUsd: opt(m?.costConfirmedUsd),
      requests: m?.requests ?? 0,
      retries: m?.retries ?? 0,
      errors: m?.errors ?? 0,
      tokensPerSec: opt(m?.tokensPerSec),
    },
    files: filesFromProto(s?.files ?? []),
    harnessFiles: filesFromProto(s?.harnessFiles ?? []),
    tests: testsFromProto(s?.tests),
    priceSnapshot: { price: priceFromProto(s?.priceSnapshot?.price), fetchedAt: iso(s?.priceSnapshot?.fetchedAt) },
    hasResult: s?.hasResult ?? false,
    hasRecording: s?.hasRecording ?? false,
  }
}

export function comparisonFromProto(c: cmp.Comparison | undefined): Comparison {
  if (!c) throw new Error('The comparison response is empty')
  return {
    id: c.id,
    createdAt: iso(c.createdAt),
    projectPath: c.projectPath,
    projectName: c.projectName,
    prompt: c.prompt,
    harness: c.harness,
    profile: profileFromProto(c.profile),
    sides: { A: sideFromProto(c.a, 'A'), B: sideFromProto(c.b, 'B') },
    report: (c.report || 'none') as ReportStatus,
    seriesId: c.seriesId,
    attempt: c.attempt,
    seriesSize: c.seriesSize,
    seriesStopped: c.seriesStopped,
  }
}

export const logsFromProto = (r: cmp.GetLogsResponse): LogEntry[] =>
  r.entries.map(e => ({ at: iso(e.at), level: e.level as LogEntry['level'], source: e.source as LogEntry['source'], message: e.message }))

export const diffFromProto = (r: cmp.GetDiffResponse): SideDiff => ({
  files: filesFromProto(r.files),
  lines: r.lines.map(l => ({ kind: l.kind as SideDiff['lines'][number]['kind'], text: l.text })),
  truncated: r.truncated,
  ready: r.ready,
  dependencyFiles: r.dependencyFiles,
})

export const testOutputFromProto = (r: cmp.GetTestsResponse): TestOutput => ({
  tests: testsFromProto(r.tests),
  visibleOutput: r.visibleOutput,
  hiddenOutput: r.hiddenOutput,
})

export const timelineFromProto = (r: cmp.GetTimelineResponse): Timeline => ({
  events: r.events.map(e => ({ at: iso(e.at), kind: e.kind, detail: e.detail })),
  sessionUsage: r.sessionUsage ? usageFromProto(r.sessionUsage) : null,
  sessionCostUsd: opt(r.sessionCostUsd),
  ready: r.ready,
})

/* ── Report ───────────────────────────────────────────────── */

const sideKey = (s: string): SideKey | null => (s === 'A' || s === 'B' ? s : null)

export function reportFromProto(r: rep.Report | undefined): Report | null {
  if (!r) return null
  return {
    comparisonId: r.comparisonId,
    status: r.status as Report['status'],
    error: r.error,
    model: r.model,
    costUsd: opt(r.costUsd),
    verdicts: r.verdicts.map(v => ({ label: v.label, side: sideKey(v.side) })),
    conclusions: r.conclusions,
    perSide: { A: r.analysisA, B: r.analysisB },
    findings: r.findings.map(f => ({
      severity: (['high', 'medium', 'low'].includes(f.severity) ? f.severity : 'low') as 'high' | 'medium' | 'low',
      side: sideKey(f.side) ?? 'A',
      title: f.title,
      impact: f.impact,
      location: f.location,
    })),
    warnings: r.warnings,
    harnessAdvice: r.harnessAdvice
      ? { differences: r.harnessAdvice.differences.map(d => ({ difference: d.difference, influence: d.influence })), suggestions: r.harnessAdvice.suggestions }
      : null,
  }
}

/* ── Settings ─────────────────────────────────────────────── */

export function settingsFromProto(s: set.Settings | undefined): Settings {
  if (!s) throw new Error('The settings response is empty')
  const suggested = limitsFromProto(s.suggestedLimits)
  return {
    keys: { openai: s.keys?.openai ?? false, anthropic: s.keys?.anthropic ?? false },
    defaultLimits: limitsFromProto(s.defaultLimits),
    suggestedLimits: { timeoutMin: suggested.timeoutMin ?? 30, maxTokensK: suggested.maxTokensK ?? 2000, maxCostUsd: suggested.maxCostUsd ?? 2 },
    reportModel: s.reportModel,
    autoReport: s.autoReport,
    resources: { cpus: s.resources?.cpus ?? 2, memoryGb: s.resources?.memoryGb ?? 4 },
    localBaseUrl: s.localBaseUrl,
    cliVersions: s.cliVersions.map(v => ({ cli: v.cli as Cli, pinned: v.pinned || null, latest: v.latest || null })),
    retentionDays: s.retentionDays,
    disk: s.disk.map(d => ({ label: d.label, bytes: Number(d.bytes) })),
  }
}

/** Only the editable fields matter; the backend ignores the read-only ones. */
export const settingsToProto = (s: Settings) =>
  create(SettingsSchema, {
    defaultLimits: limitsToProto(s.defaultLimits),
    reportModel: s.reportModel,
    autoReport: s.autoReport,
    resources: { cpus: s.resources.cpus, memoryGb: s.resources.memoryGb },
    retentionDays: s.retentionDays,
  })
