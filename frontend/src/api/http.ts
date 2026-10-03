// Client for the Go backend. Routes follow the plan; the backend does not implement them yet
// (phase 0 only has /api/health), so the mock client is the default.

import type { ApiClient } from './client'
import type { TerminalSource } from './types'

const BASE = import.meta.env.VITE_API_BASE_URL || '/api'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`${method} ${path} returned ${res.status}${text ? ': ' + text : ''}`)
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T)
}

const get = <T>(path: string) => request<T>('GET', path)
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {})

function websocketTerminal(path: string): TerminalSource {
  const url = new URL(BASE + path, window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  const ws = new WebSocket(url)
  ws.binaryType = 'arraybuffer'
  const decoder = new TextDecoder()
  return {
    subscribe(onData) {
      const handler = (e: MessageEvent) => onData(typeof e.data === 'string' ? e.data : decoder.decode(e.data))
      ws.addEventListener('message', handler)
      return () => {
        ws.removeEventListener('message', handler)
        ws.close()
      }
    },
    send(data) {
      if (ws.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(data))
    },
    resize(cols, rows) {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    },
  }
}

export const httpClient: ApiClient = {
  getCatalog: () => get('/catalog'),
  refreshCatalog: () => post('/catalog/refresh'),
  inspectProject: path => post('/projects/inspect', { path }),
  startComparison: input => post('/comparisons', input),
  getActiveComparison: () => get('/comparisons/active'),
  getComparison: id => get(`/comparisons/${id}`),
  finishSide: (id, side) => post(`/comparisons/${id}/sides/${side}/finish`),
  cancelSide: (id, side) => post(`/comparisons/${id}/sides/${side}/cancel`),
  getLogs: (id, side) => get(`/comparisons/${id}/sides/${side}/logs`),
  getDiff: (id, side) => get(`/comparisons/${id}/sides/${side}/diff`),
  getTimeline: (id, side) => get(`/comparisons/${id}/sides/${side}/events`),
  getTestOutput: (id, side) => get(`/comparisons/${id}/sides/${side}/tests`),
  listHistory: () => get('/comparisons'),
  deleteComparison: id => request('DELETE', `/comparisons/${id}`),
  getReport: id => get(`/comparisons/${id}/report`),
  generateReport: id => post(`/comparisons/${id}/report`),
  listPresets: () => get('/presets'),
  getPreset: slug => get(`/presets/${slug}`),
  getPresetFile: (slug, path) => get(`/presets/${slug}/files?path=${encodeURIComponent(path)}`),
  getSettings: () => get('/settings'),
  updateSettings: patch => request('PATCH', '/settings', patch),
  openTerminal: (id, side) => websocketTerminal(`/comparisons/${id}/sides/${side}/terminal`),
}
