// The live connection to the backend: EventService.Watch, a Connect server stream. Each event
// carries a comparison as it is now; it goes straight into the cache, and the queries that
// depend on what changed are invalidated. On reconnect the backend sends the live comparisons
// again, and everything else about comparisons is refetched, so nothing missed is lost.

import { create } from '@bufbuild/protobuf'
import { createConnectQueryKey } from '@connectrpc/connect-query'
import type { QueryClient } from '@tanstack/react-query'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import type { Comparison as ComparisonMsg } from '@/gen/aicompare/v1/comparison_pb'
import { ComparisonService, GetActiveComparisonResponseSchema, GetComparisonResponseSchema } from '@/gen/aicompare/v1/comparison_pb'
import { comparisonKey, invalidateComparisonDetails, methodKey } from './queries'
import { clients, transport } from './transport'

export type StreamState = 'connecting' | 'live' | 'reconnecting'

const TERMINAL = ['finished', 'error', 'cancelled', 'limit_reached']
const live = (c: ComparisonMsg) => !TERMINAL.includes(c.a?.status ?? '') || !TERMINAL.includes(c.b?.status ?? '')

const activeKey = () =>
  createConnectQueryKey({ schema: ComparisonService.method.getActiveComparison, input: {}, transport, cardinality: 'finite' })

/** Keeps the event stream open while the app is mounted and reports its state. */
export function useEventStream(): StreamState {
  const qc = useQueryClient()
  const [state, setState] = useState<StreamState>('connecting')

  useEffect(() => {
    const abort = new AbortController()
    let attempt = 0
    ;(async () => {
      while (!abort.signal.aborted) {
        try {
          const stream = clients.events.watch({}, {
            signal: abort.signal,
            timeoutMs: 0,
            // The server sends headers as soon as the stream is open, before any event.
            onHeader: () => {
              const reconnected = attempt > 0
              attempt = 0
              setState('live')
              // Anything that changed while disconnected is refetched.
              if (reconnected) qc.invalidateQueries({ predicate: q => (q.queryKey as unknown[])[0] === 'connect-query' })
            },
          })
          for await (const res of stream) {
            if (res.event.case === 'comparison') apply(qc, res.event.value)
            if (res.event.case === 'deletedId') removed(qc, res.event.value)
          }
        } catch {
          if (abort.signal.aborted) return
        }
        setState('reconnecting')
        attempt++
        await new Promise(r => setTimeout(r, Math.min(1000 * 2 ** Math.min(attempt, 4), 10_000)))
      }
    })()
    return () => abort.abort()
  }, [qc])

  return state
}

function apply(qc: QueryClient, c: ComparisonMsg) {
  const key = comparisonKey(c.id)
  const before = qc.getQueryData(key)?.comparison
  qc.setQueryData(key, create(GetComparisonResponseSchema, { comparison: c }))

  // The active comparison follows the newest live one.
  const active = qc.getQueryData(activeKey())?.comparison
  if (live(c)) {
    if (!active || active.id === c.id || c.createdAt!.seconds >= active.createdAt!.seconds) {
      qc.setQueryData(activeKey(), create(GetActiveComparisonResponseSchema, { comparison: c }))
    }
  } else if (active?.id === c.id) {
    qc.setQueryData(activeKey(), create(GetActiveComparisonResponseSchema, {}))
  }

  const statusChanged = before?.a?.status !== c.a?.status || before?.b?.status !== c.b?.status
  if (live(c)) {
    // New log lines and proxy requests arrive all the time while a side runs.
    invalidateComparisonDetails(qc, c.id, ['GetLogs'])
  }
  if (statusChanged) {
    invalidateComparisonDetails(qc, c.id, ['GetLogs', 'GetDiff', 'GetTests', 'GetTimeline'])
    qc.invalidateQueries({ predicate: methodKey('ListComparisons') })
  }
  if (before?.report !== c.report) {
    invalidateComparisonDetails(qc, c.id, ['GetReport'])
    qc.invalidateQueries({ predicate: methodKey('ListComparisons') })
  }
}

function removed(qc: QueryClient, id: string) {
  qc.removeQueries({ predicate: methodKey('GetComparison', id) })
  qc.invalidateQueries({ predicate: methodKey('ListComparisons') })
}
