// Client for the Go backend. With VITE_USE_MOCKS=true only the parts the backend implements
// are used (see client.ts).

import type { ApiClient } from './client'
import type { TerminalSource } from './types'
import { getCatalog, refreshCatalog } from './rpc'

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

/**
 * Terminal over WebSocket: binary frames carry TTY output and keystrokes, text frames carry
 * JSON control messages (resize). Output that arrives before anyone subscribes is kept, and
 * the latest size is sent as soon as the socket opens.
 */
export function websocketTerminal(path: string): TerminalSource {
  const url = new URL(BASE + path, window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  const ws = new WebSocket(url)
  ws.binaryType = 'arraybuffer'
  // stream: true keeps multi-byte characters split across frames intact.
  const decoder = new TextDecoder()
  const encoder = new TextEncoder()
  const listeners = new Set<(chunk: string) => void>()
  let pending = ''
  let size: { cols: number; rows: number } | null = null

  const emit = (chunk: string) => {
    if (listeners.size === 0) pending += chunk
    else listeners.forEach(l => l(chunk))
  }
  ws.addEventListener('message', e => emit(typeof e.data === 'string' ? e.data : decoder.decode(e.data, { stream: true })))
  ws.addEventListener('open', () => {
    if (size) ws.send(JSON.stringify({ type: 'resize', ...size }))
  })
  ws.addEventListener('close', e => emit(`\r\n\x1b[90m[connection closed${e.reason ? ': ' + e.reason : ''}]\x1b[0m\r\n`))
  ws.addEventListener('error', () => emit('\r\n\x1b[91m[could not connect to the terminal]\x1b[0m\r\n'))

  return {
    subscribe(onData) {
      if (pending) {
        onData(pending)
        pending = ''
      }
      listeners.add(onData)
      return () => {
        listeners.delete(onData)
        if (listeners.size === 0) ws.close()
      }
    },
    send(data) {
      if (ws.readyState === WebSocket.OPEN) ws.send(encoder.encode(data))
    },
    resize(cols, rows) {
      size = { cols, rows }
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    },
  }
}

export const httpClient: ApiClient = {
  // Already on Connect (see rpc.ts); the rest move there service by service.
  getCatalog,
  refreshCatalog,
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
