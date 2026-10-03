// Data hooks. Queries use connect-query (cache keys come from the generated method descriptors);
// mutations call the clients. Nothing polls: the event stream (events.ts) puts every change of a
// comparison into the cache and invalidates what depends on it.

import { create } from '@bufbuild/protobuf'
import { createConnectQueryKey, useQuery } from '@connectrpc/connect-query'
import type { Query, QueryClient } from '@tanstack/react-query'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CatalogService, GetCatalogResponseSchema } from '@/gen/aicompare/v1/catalog_pb'
import { ComparisonService } from '@/gen/aicompare/v1/comparison_pb'
import { ProjectService } from '@/gen/aicompare/v1/project_pb'
import { ReportService } from '@/gen/aicompare/v1/report_pb'
import { GetSettingsResponseSchema, SettingsService } from '@/gen/aicompare/v1/settings_pb'
import {
  catalogFromProto,
  comparisonFromProto,
  diffFromProto,
  foldersFromProto,
  inspectionFromProto,
  logsFromProto,
  reportFromProto,
  settingsFromProto,
  settingsToProto,
  sideConfigToProto,
  testOutputFromProto,
  timelineFromProto,
} from './convert'
import { clients, transport } from './transport'
import type { Comparison, NewComparison, Settings, SideKey } from './types'
import { TERMINAL_STATUSES } from './types'

export const isLive = (c: Comparison | null | undefined) =>
  !!c && (!TERMINAL_STATUSES.includes(c.sides.A.status) || !TERMINAL_STATUSES.includes(c.sides.B.status))

/** Matches connect-query keys of one method, optionally for one comparison id. */
export function methodKey(method: string, id?: string) {
  return (q: Query) => {
    const [tag, k] = q.queryKey as [string, { methodName?: string; input?: { id?: string; comparisonId?: string } }]
    return tag === 'connect-query' && k?.methodName === method && (id == null || k.input?.id === id || k.input?.comparisonId === id)
  }
}

export const comparisonKey = (id: string) =>
  createConnectQueryKey({ schema: ComparisonService.method.getComparison, input: { id }, transport, cardinality: 'finite' })

/* ── Catalogue and projects ───────────────────────────────── */

export const useCatalog = () =>
  useQuery(CatalogService.method.getCatalog, {}, { select: r => catalogFromProto(r.catalog), staleTime: 5 * 60_000 })

export function useRefreshCatalog() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => clients.catalog.refreshCatalog({}),
    onSuccess: r => qc.setQueryData(
      createConnectQueryKey({ schema: CatalogService.method.getCatalog, input: {}, transport, cardinality: 'finite' }),
      create(GetCatalogResponseSchema, { catalog: r.catalog }),
    ),
  })
}

export const useInspectProject = () =>
  useMutation({ mutationFn: async (path: string) => inspectionFromProto((await clients.projects.inspectProject({ path })).inspection) })

export const useFolders = (path: string) =>
  useQuery(ProjectService.method.listFolders, { path }, { select: foldersFromProto, staleTime: 30_000, retry: false })

/* ── Comparisons ──────────────────────────────────────────── */

export const useActiveComparison = () =>
  useQuery(ComparisonService.method.getActiveComparison, {}, {
    select: r => (r.comparison ? comparisonFromProto(r.comparison) : null),
  })

export const useComparison = (id: string) =>
  useQuery(ComparisonService.method.getComparison, { id }, { select: r => comparisonFromProto(r.comparison) })

/** Ended comparisons, newest first. */
export const useHistory = () =>
  useQuery(ComparisonService.method.listComparisons, {}, {
    select: r => r.comparisons.map(comparisonFromProto).filter(c => !isLive(c)),
  })

export function useStartComparison() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: NewComparison) => clients.comparisons.startComparison({
      projectPath: input.projectPath,
      profile: input.profile,
      prompt: input.prompt,
      a: sideConfigToProto(input.sides.A),
      b: sideConfigToProto(input.sides.B),
    }),
    onSuccess: () => qc.invalidateQueries({ predicate: methodKey('GetActiveComparison') }),
  })
}

export function useSideAction(id: string) {
  return useMutation({
    mutationFn: async ({ side, action }: { side: SideKey; action: 'finish' | 'cancel' }) => {
      if (action === 'finish') await clients.comparisons.finishSide({ id, side })
      else await clients.comparisons.cancelSide({ id, side })
    },
  })
}

export function useDeleteComparison() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => clients.comparisons.deleteComparison({ id }),
    onSuccess: () => qc.invalidateQueries({ predicate: methodKey('ListComparisons') }),
  })
}

export const useLogs = (id: string, side: SideKey) =>
  useQuery(ComparisonService.method.getLogs, { id, side }, { select: logsFromProto })

export const useDiff = (id: string, side: SideKey, kind: 'solution' | 'harness') =>
  useQuery(ComparisonService.method.getDiff, { id, side, kind }, { select: diffFromProto })

export const useTimeline = (id: string, side: SideKey) =>
  useQuery(ComparisonService.method.getTimeline, { id, side }, { select: timelineFromProto })

export const useTestOutput = (id: string, side: SideKey) =>
  useQuery(ComparisonService.method.getTests, { id, side }, { select: testOutputFromProto })

/* ── Report ───────────────────────────────────────────────── */

export const useReport = (id: string) =>
  useQuery(ReportService.method.getReport, { comparisonId: id }, { select: r => reportFromProto(r.report) })

export function useGenerateReport(id: string) {
  return useMutation({ mutationFn: () => clients.reports.generateReport({ comparisonId: id }) })
}

/* ── Settings ─────────────────────────────────────────────── */

const settingsKey = () =>
  createConnectQueryKey({ schema: SettingsService.method.getSettings, input: {}, transport, cardinality: 'finite' })

export const useSettings = () =>
  useQuery(SettingsService.method.getSettings, {}, { select: r => settingsFromProto(r.settings) })

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (patch: Partial<Settings>) => {
      const current = qc.getQueryData(settingsKey())
      const next = { ...settingsFromProto(current?.settings), ...patch }
      return clients.settings.updateSettings({ settings: settingsToProto(next) })
    },
    onSuccess: r => qc.setQueryData(settingsKey(), create(GetSettingsResponseSchema, { settings: r.settings })),
  })
}

export function useCleanUp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => clients.settings.cleanUp({}),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKey() }),
  })
}

/** Invalidates every query about one comparison except the comparison itself. */
export function invalidateComparisonDetails(qc: QueryClient, id: string, methods: string[]) {
  for (const m of methods) qc.invalidateQueries({ predicate: methodKey(m, id) })
}
