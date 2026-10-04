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
  CostPart,
  Finding,
  Criterion,
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
  ScorePart,
  SideReport,
  Judgement,
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
  lint: p?.lint ?? '',
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
    otherEntries: i.otherEntries,
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
  lintCommand: t?.lintCommand ?? '',
  lint: testRunFromProto(t?.lint),
  baseline: t?.baseline ? { tests: testRunFromProto(t.baseline.tests), lint: testRunFromProto(t.baseline.lint) } : null,
})

export const criteriaFromProto = (c: { text: string; required: boolean }[]): Criterion[] => c.map(x => ({ text: x.text, required: x.required }))

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
    criteria: criteriaFromProto(c.criteria),
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

const n = (v: bigint | number) => Number(v)
const parts = (p: { label: string; tokens: bigint }[]): CostPart[] => p.map(x => ({ label: x.label, tokens: n(x.tokens) }))
const sideOrNull = (s: string): SideKey | null => (s === 'A' || s === 'B' ? s : null)

function sideReportFromProto(s: rep.SideReport | undefined): SideReport {
  const ss = s?.session
  return {
    gates: (s?.gates ?? []).map(g => ({ key: g.key, label: g.label, passed: g.passed, reason: g.reason })),
    criteria: (s?.criteria ?? []).map(c => ({ index: c.index, status: c.status as SideReport['criteria'][number]['status'], method: c.method as SideReport['criteria'][number]['method'], evidence: c.evidence })),
    review: {
      problems: (s?.review?.problems ?? []).map(f => ({
        severity: (['high', 'medium', 'low'].includes(f.severity) ? f.severity : 'low') as Finding['severity'],
        title: f.title, impact: f.impact, location: f.location,
      })),
      strengths: (s?.review?.strengths ?? []).map(x => ({ title: x.title, location: x.location })),
      notReviewed: s?.review?.notReviewed ?? [],
    },
    analysis: s?.analysis ?? '',
    score: {
      total: s?.score?.total ?? 0,
      parts: (s?.score?.parts ?? []).map(p => ({ key: p.key as ScorePart['key'], label: p.label, points: p.points, max: p.max, lines: p.lines.map(l => ({ label: l.label, points: l.points, max: l.max, detail: l.detail })) })),
    },
    notVerified: s?.notVerified ?? [],
    harness: s?.harness
      ? {
        firstRequestTokens: n(s.harness.firstRequestTokens), parts: parts(s.harness.parts), perRequest: n(s.harness.perRequest),
        requests: s.harness.requests, total: n(s.harness.total), cacheShare: s.harness.cacheShare, costUsd: opt(s.harness.costUsd),
        shareOfSide: opt(s.harness.shareOfSide), files: parts(s.harness.files), skills: parts(s.harness.skills), skillsLoaded: parts(s.harness.skillsLoaded),
      }
      : null,
    audit: s?.audit
      ? {
        strengths: s.audit.strengths.map(x => ({ title: x.title, evidence: x.evidence })),
        gaps: s.audit.gaps.map(x => ({ title: x.title, evidence: x.evidence })),
        suggestions: s.audit.suggestions.map(x => ({ kind: x.kind, file: x.file, change: x.change, evidence: x.evidence, tokensSaved: n(x.tokensSaved) })),
      }
      : null,
    subagents: (s?.subagents ?? []).map(a => ({ type: a.type, description: a.description, model: a.model, status: a.status, durationSec: a.durationSec, tokens: n(a.tokens), costUsd: a.costUsd, tools: { ...a.tools } })),
    session: {
      requests: ss?.requests ?? 0, cacheShare: ss?.cacheShare ?? 0, reasoningSteps: ss?.reasoningSteps ?? 0, reasoningTokens: n(ss?.reasoningTokens ?? 0),
      firstEditSec: opt(ss?.firstEditSec), tools: { ...(ss?.tools ?? {}) }, toolCalls: ss?.toolCalls ?? 0, toolFailures: ss?.toolFailures ?? 0,
      failedCommands: (ss?.failedCommands ?? []).map(c => ({ command: c.command, exitCode: c.exitCode, fixed: c.fixed, agent: c.agent })),
      endsWithQuestion: ss?.endsWithQuestion ?? false, longContextRequests: ss?.longContextRequests ?? 0, providerErrors: ss?.providerErrors ?? 0,
      rateLimited: ss?.rateLimited ?? 0,
      requestPoints: (ss?.requestPoints ?? []).map(p => ({ atSec: p.atSec, context: n(p.context), costUsd: opt(p.costUsd) })),
      reasoningPoints: (ss?.reasoningPoints ?? []).map(n),
    },
  }
}

export function reportFromProto(r: rep.Report | undefined): Report | null {
  if (!r) return null
  const j = r.judge
  return {
    version: r.version,
    comparisonId: r.comparisonId,
    status: r.status as Report['status'],
    error: r.error,
    model: r.model,
    judgeModel: r.judgeModel,
    costUsd: opt(r.costUsd),
    headline: r.headline,
    criteria: criteriaFromProto(r.criteria),
    criteriaBy: r.criteriaBy as Report['criteriaBy'],
    sides: r.a && r.b ? { A: sideReportFromProto(r.a), B: sideReportFromProto(r.b) } : null,
    judge: j
      ? {
        winner: sideOrNull(j.winner),
        confidence: (['high', 'medium', 'low'].includes(j.confidence) ? j.confidence : 'low') as Judgement['confidence'],
        reasons: j.reasons,
        ship: Object.fromEntries(Object.entries(j.ship).map(([k, v]) => [k, { yes: v.yes, reason: v.reason }])),
        labels: j.labels.map(l => ({ label: l.label, side: sideOrNull(l.side) })),
        disagreements: j.disagreements,
        passesAgree: j.passesAgree,
        passes: j.passes.map(sideOrNull),
      }
      : null,
    warnings: r.warnings,
    userVerdict: r.userVerdict?.verdict ? { verdict: r.userVerdict.verdict as 'agree' | 'other' | 'tie', note: r.userVerdict.note } : null,
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
    judgeModel: s.judgeModel,
    autoReport: s.autoReport,
    resources: { cpus: s.resources?.cpus ?? 2, memoryGb: s.resources?.memoryGb ?? 4 },
    localBaseUrl: s.localBaseUrl,
    cliVersions: s.cliVersions.map(v => ({ cli: v.cli as Cli, pinned: v.pinned || null, latest: v.latest || null })),
    retentionDays: s.retentionDays,
    retentionHours: s.retentionHours,
    retention: {
      containers: s.retention?.containers ?? true,
      images: s.retention?.images ?? true,
      projectCopies: s.retention?.projectCopies ?? true,
      artefacts: s.retention?.artefacts ?? false,
    },
    disk: s.disk.map(d => ({ label: d.label, bytes: Number(d.bytes) })),
  }
}

/** Only the editable fields matter; the backend ignores the read-only ones. */
export const settingsToProto = (s: Settings) =>
  create(SettingsSchema, {
    defaultLimits: limitsToProto(s.defaultLimits),
    reportModel: s.reportModel,
    judgeModel: s.judgeModel,
    autoReport: s.autoReport,
    resources: { cpus: s.resources.cpus, memoryGb: s.resources.memoryGb },
    retentionDays: s.retentionDays,
    retentionHours: s.retentionHours,
    retention: s.retention,
  })
