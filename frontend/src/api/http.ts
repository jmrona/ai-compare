// What the backend serves outside Connect: terminals (WebSocket), recordings and downloads.

import type { SideKey, TerminalSource } from './types'
import { API_BASE as BASE } from './transport'

/** The side's files as a zip named after its model. */
export const downloadUrl = (id: string, side: SideKey) => `${BASE}/comparisons/${id}/sides/${side}/download`

/** The side's terminal recording (asciicast v2). */
export const recordingUrl = (id: string, side: SideKey) => `${BASE}/comparisons/${id}/sides/${side}/recording`

/** The live terminal of a side, or its final screen once it has ended. */
export const sideTerminal = (id: string, side: SideKey) => websocketTerminal(`/comparisons/${id}/sides/${side}/terminal`)

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
