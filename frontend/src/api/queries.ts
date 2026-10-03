import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import type { Comparison, NewComparison, Settings, SideKey } from './types'
import { TERMINAL_STATUSES } from './types'

export const keys = {
  catalog: ['catalog'] as const,
  active: ['comparisons', 'active'] as const,
  comparison: (id: string) => ['comparisons', id] as const,
  history: ['comparisons', 'history'] as const,
  logs: (id: string, side: SideKey) => ['comparisons', id, side, 'logs'] as const,
  diff: (id: string, side: SideKey) => ['comparisons', id, side, 'diff'] as const,
  timeline: (id: string, side: SideKey) => ['comparisons', id, side, 'timeline'] as const,
  tests: (id: string, side: SideKey) => ['comparisons', id, side, 'tests'] as const,
  report: (id: string) => ['comparisons', id, 'report'] as const,
  presets: ['presets'] as const,
  preset: (slug: string) => ['presets', slug] as const,
  presetFile: (slug: string, path: string) => ['presets', slug, 'file', path] as const,
  settings: ['settings'] as const,
}

export const isLive = (c: Comparison | null | undefined) =>
  !!c && (!TERMINAL_STATUSES.includes(c.sides.A.status) || !TERMINAL_STATUSES.includes(c.sides.B.status))

// With the real backend, changes will arrive over SSE and invalidate these queries.
// Until then, a running comparison is refetched every second.
const LIVE_REFRESH_MS = 1000

export const useCatalog = () => useQuery({ queryKey: keys.catalog, queryFn: api.getCatalog, staleTime: 5 * 60_000 })

export function useRefreshCatalog() {
  const qc = useQueryClient()
  return useMutation({ mutationFn: api.refreshCatalog, onSuccess: data => qc.setQueryData(keys.catalog, data) })
}

export const useInspectProject = () => useMutation({ mutationFn: (path: string) => api.inspectProject(path) })

// Polled only while a run is live. With nothing running there is nothing to poll for: starting a
// comparison invalidates this query, and returning to the tab refetches it (for runs started elsewhere).
export const useActiveComparison = () =>
  useQuery({ queryKey: keys.active, queryFn: api.getActiveComparison, refetchInterval: q => (isLive(q.state.data) ? LIVE_REFRESH_MS : false) })

export const useComparison = (id: string) =>
  useQuery({
    queryKey: keys.comparison(id),
    queryFn: () => api.getComparison(id),
    // Keep polling while the run is live or its report is being generated.
    refetchInterval: q => (isLive(q.state.data) || q.state.data?.report === 'generating' ? LIVE_REFRESH_MS : false),
  })

export function useStartComparison() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: NewComparison) => api.startComparison(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.active }),
  })
}

export function useSideAction(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ side, action }: { side: SideKey; action: 'finish' | 'cancel' }) =>
      action === 'finish' ? api.finishSide(id, side) : api.cancelSide(id, side),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['comparisons'] }),
  })
}

export const useLogs = (id: string, side: SideKey) => useQuery({ queryKey: keys.logs(id, side), queryFn: () => api.getLogs(id, side) })
export const useDiff = (id: string, side: SideKey) => useQuery({ queryKey: keys.diff(id, side), queryFn: () => api.getDiff(id, side) })
export const useTimeline = (id: string, side: SideKey) => useQuery({ queryKey: keys.timeline(id, side), queryFn: () => api.getTimeline(id, side) })
export const useTestOutput = (id: string, side: SideKey) => useQuery({ queryKey: keys.tests(id, side), queryFn: () => api.getTestOutput(id, side) })

export const useHistory = () => useQuery({ queryKey: keys.history, queryFn: api.listHistory })

export function useDeleteComparison() {
  const qc = useQueryClient()
  return useMutation({ mutationFn: (id: string) => api.deleteComparison(id), onSuccess: () => qc.invalidateQueries({ queryKey: keys.history }) })
}

export const useReport = (id: string, generating: boolean) =>
  useQuery({ queryKey: keys.report(id), queryFn: () => api.getReport(id), refetchInterval: generating ? 1000 : false })

export function useGenerateReport(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.generateReport(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.comparison(id) })
      qc.invalidateQueries({ queryKey: keys.report(id) })
    },
  })
}

export const usePresets = () => useQuery({ queryKey: keys.presets, queryFn: api.listPresets })
export const usePreset = (slug: string) => useQuery({ queryKey: keys.preset(slug), queryFn: () => api.getPreset(slug) })
export const usePresetFile = (slug: string, path: string) =>
  useQuery({ queryKey: keys.presetFile(slug, path), queryFn: () => api.getPresetFile(slug, path) })

export const useSettings = () => useQuery({ queryKey: keys.settings, queryFn: api.getSettings })

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (patch: Partial<Settings>) => api.updateSettings(patch),
    onSuccess: data => qc.setQueryData(keys.settings, data),
  })
}
