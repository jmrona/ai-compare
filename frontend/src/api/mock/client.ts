// In-memory fake backend. It mimics what the real backend will do: per-side states,
// metrics that grow while the agent works, terminals that keep their output (so a
// reconnect rebuilds the screen) and an interactive mode that waits for your answer.

import type { ApiClient } from '@/api/client'
import type { Comparison, SideKey, SideRun, SideStatus, TerminalSource, Usage } from '@/api/types'
import { TERMINAL_STATUSES } from '@/api/types'
import { estimateCost } from '@/lib/format'
import {
  AGENTS_MD,
  CATALOG,
  DEFAULT_PROFILE,
  DIFFS,
  HISTORY,
  LOGS,
  NOW,
  PRESETS,
  PROJECT_PATH,
  PROMPT_0142,
  REPORTS,
  SETTINGS,
  TEST_OUTPUT,
  TIMELINE,
  autonomousScript,
  interactiveScript,
  makeSide,
  prepLine,
} from './fixtures'

const delay = (ms = 220) => new Promise(r => setTimeout(r, ms))
const clone = <T>(v: T): T => structuredClone(v)
const SEC = 1000

/* ── Simulating one side ───────────────────────────────── */

interface Sim {
  createdAt: number
  runStartedAt: number | null
  /** When the agent stopped requesting tokens (waiting for input). */
  pausedAt: number | null
  endedAt: number | null
  humanWaitMs: number
  /** Tokens per second of agent work. */
  rate: Usage
  buffer: string
  listeners: Set<(chunk: string) => void>
  awaitingAnswer: boolean
  answer: string
  timers: number[]
}

const sims = new Map<string, Sim>()
const comparisons = new Map<string, Comparison>()
let activeId: string | null = null
let nextId = 143

const simKey = (id: string, side: SideKey) => `${id}:${side}`

function newSim(createdAt: number, rate: Usage): Sim {
  return { createdAt, runStartedAt: null, pausedAt: null, endedAt: null, humanWaitMs: 0, rate, buffer: '', listeners: new Set(), awaitingAnswer: false, answer: '', timers: [] }
}

function write(sim: Sim, text: string) {
  sim.buffer += text
  sim.listeners.forEach(l => l(text))
}
const writeLines = (sim: Sim, lines: string[]) => write(sim, lines.map(l => l + '\r\n').join(''))

/** Writes lines one at a time, as if they came from the CLI. The last line keeps the cursor on it. */
function play(sim: Sim, lines: string[], every: number, done?: () => void) {
  lines.forEach((line, i) => {
    sim.timers.push(window.setTimeout(() => write(sim, (i > 0 ? '\r\n' : '') + line), every * (i + 1)))
  })
  if (done) sim.timers.push(window.setTimeout(done, every * (lines.length + 1)))
}

function setStatus(id: string, side: SideKey, status: SideStatus, endReason?: string) {
  const c = comparisons.get(id)
  if (!c) return
  c.sides[side].status = status
  if (endReason) c.sides[side].endReason = endReason
  const sim = sims.get(simKey(id, side))
  if (!sim) return
  const now = Date.now()
  if (status === 'running' && sim.runStartedAt == null) sim.runStartedAt = now
  if (status === 'running' && sim.pausedAt != null && sim.endedAt == null) {
    sim.humanWaitMs += now - sim.pausedAt
    sim.pausedAt = null
  }
  if (status === 'waiting_input') sim.pausedAt = now
  if (TERMINAL_STATUSES.includes(status)) {
    if (sim.pausedAt != null) sim.humanWaitMs += now - sim.pausedAt
    sim.pausedAt = null
    sim.endedAt = now
    sim.timers.forEach(clearTimeout)
    sim.awaitingAnswer = false
  }
  if (TERMINAL_STATUSES.includes(c.sides.A.status) && TERMINAL_STATUSES.includes(c.sides.B.status) && activeId === id) {
    activeId = null
  }
}

/** Current metrics of a side, derived from its simulation. */
function liveSide(id: string, run: SideRun): SideRun {
  const sim = sims.get(simKey(id, run.key))
  if (!sim) return run
  const now = Date.now()
  const workEnd = sim.endedAt ?? sim.pausedAt ?? now
  const waiting = sim.pausedAt != null && sim.endedAt == null ? now - sim.pausedAt : 0
  const agentMs = sim.runStartedAt == null ? 0 : Math.max(0, workEnd - sim.runStartedAt - sim.humanWaitMs)
  const s = agentMs / SEC
  const usage: Usage = {
    input: Math.round(sim.rate.input * s),
    cacheRead: Math.round(sim.rate.cacheRead * s),
    cacheWrite: null,
    output: Math.round(sim.rate.output * s),
  }
  return {
    ...run,
    metrics: {
      ...run.metrics,
      elapsedSec: ((sim.endedAt ?? now) - sim.createdAt) / SEC,
      agentSec: s,
      humanWaitSec: run.config.mode === 'interactive' ? (sim.humanWaitMs + waiting) / SEC : null,
      usage,
      costUsd: estimateCost(usage, run.priceSnapshot.price),
      requests: Math.round(s / 22),
      tokensPerSec: s > 5 ? Math.round(sim.rate.output * 1.6) : null,
    },
  }
}

function snapshot(id: string): Comparison {
  const c = comparisons.get(id)
  if (!c) throw new Error(`Comparison #${id} does not exist`)
  return { ...clone(c), sides: { A: liveSide(id, clone(c.sides.A)), B: liveSide(id, clone(c.sides.B)) } }
}

/* ── Starting a new side ───────────────────────────────── */

function runSide(c: Comparison, side: SideKey) {
  const sim = sims.get(simKey(c.id, side))!
  const cfg = c.sides[side].config
  const steps: [SideStatus, string, number][] = [
    ['copying', 'copying project (read-only)…', 900],
    ['building', 'building image: node:22 + npm ci + opencode 1.9.2…', 1800],
    ['starting', 'starting container · 2 CPUs · 4 GB memory', 900],
  ]
  let t = 0
  for (const [status, text, ms] of steps) {
    sim.timers.push(window.setTimeout(() => {
      setStatus(c.id, side, status)
      writeLines(sim, [prepLine(text)])
    }, t))
    t += ms
  }
  sim.timers.push(window.setTimeout(() => {
    write(sim, '\x1b[2J\x1b[H')
    setStatus(c.id, side, 'running')
    if (cfg.mode === 'interactive') {
      const script = interactiveScript(cfg.model, cfg.effort, c.prompt)
      play(sim, script.beforeQuestion, 450, () => {
        setStatus(c.id, side, 'waiting_input')
        sim.awaitingAnswer = true
      })
    } else {
      play(sim, autonomousScript(cfg.model, c.prompt), 650, () => setStatus(c.id, side, 'finished', 'The CLI exited (code 0)'))
    }
  }, t))
}

function answer(id: string, side: SideKey) {
  const c = comparisons.get(id)!
  const sim = sims.get(simKey(id, side))!
  sim.awaitingAnswer = false
  setStatus(id, side, 'running')
  const cfg = c.sides[side].config
  play(sim, interactiveScript(cfg.model, cfg.effort, c.prompt).afterAnswer, 500, () =>
    setStatus(id, side, 'finished', 'The agent ended the session'))
}

/* ── Initial state: #0142 running + history ────────────── */

function seed() {
  for (const h of HISTORY) comparisons.set(h.id, clone(h))

  const createdAt = NOW - 12 * 60 * SEC
  const noLimits = { timeoutMin: null, maxTokensK: null, maxCostUsd: null }
  const c: Comparison = {
    id: '0142',
    createdAt: new Date(createdAt).toISOString(),
    projectPath: PROJECT_PATH,
    projectName: 'invoices-web',
    prompt: PROMPT_0142,
    harness: "project's AGENTS.md",
    sides: {
      A: makeSide('A', { cli: 'opencode', provider: 'openai', model: 'gpt-5.5', effort: 'high', mode: 'interactive', limits: noLimits }, { status: 'waiting_input', files: DIFFS.A.files }),
      B: makeSide('B', { cli: 'opencode', provider: 'openai', model: 'gpt-5.5-mini', effort: 'medium', mode: 'autonomous', limits: { ...noLimits, timeoutMin: 30 } }, {
        status: 'finished',
        endReason: 'The CLI exited (code 0)',
        files: DIFFS.B.files,
        tests: { passed: 16, total: 16 },
      }),
    },
    report: 'none',
  }
  comparisons.set(c.id, c)
  activeId = c.id

  const script = interactiveScript('gpt-5.5', 'high', PROMPT_0142).beforeQuestion
  const simA = newSim(createdAt, { input: 150, cacheRead: 440, cacheWrite: null, output: 52 })
  simA.runStartedAt = createdAt + 32 * SEC
  simA.pausedAt = NOW - 108 * SEC
  writeLines(simA, script.slice(0, -1))
  write(simA, script.at(-1)!)
  simA.awaitingAnswer = true
  sims.set(simKey(c.id, 'A'), simA)

  const simB = newSim(createdAt, { input: 220, cacheRead: 470, cacheWrite: null, output: 58 })
  simB.runStartedAt = createdAt + 30 * SEC
  simB.endedAt = createdAt + 30 * SEC + 552 * SEC
  writeLines(simB, autonomousScript('gpt-5.5-mini', PROMPT_0142))
  sims.set(simKey(c.id, 'B'), simB)
}
seed()

/* ── In-memory terminal ────────────────────────────────── */

function mockTerminal(id: string, side: SideKey): TerminalSource {
  const sim = sims.get(simKey(id, side))
  return {
    subscribe(onData) {
      if (!sim) {
        onData(historyRecording(id, side))
        return () => {}
      }
      // Accumulated output first: a reopened tab sees the same screen as before.
      onData(sim.buffer)
      sim.listeners.add(onData)
      return () => {
        sim.listeners.delete(onData)
      }
    },
    send(data) {
      if (!sim || !sim.awaitingAnswer) return
      for (const ch of data) {
        if (ch === '\r') {
          write(sim, '\r\n')
          answer(id, side)
          return
        }
        if (ch === '\x7f') {
          if (sim.answer.length) {
            sim.answer = sim.answer.slice(0, -1)
            write(sim, '\b \b')
          }
          continue
        }
        if (ch >= ' ') {
          sim.answer += ch
          write(sim, ch)
        }
      }
    },
    resize() {},
  }
}

function historyRecording(id: string, side: SideKey): string {
  const c = comparisons.get(id)
  if (!c) return ''
  const cfg = c.sides[side].config
  if (cfg.mode !== 'interactive') return autonomousScript(cfg.model, c.prompt).join('\r\n')
  const s = interactiveScript(cfg.model, cfg.effort, c.prompt)
  return [...s.beforeQuestion.slice(0, -1), ' ❯ 1', ...s.afterAnswer].join('\r\n')
}

/* ── Client ────────────────────────────────────────────── */

let settings = clone(SETTINGS)
const isAbsolute = (p: string) => /^[a-zA-Z]:[\\/]/.test(p) || p.startsWith('/')

export const mockClient: ApiClient = {
  async getCatalog() {
    await delay()
    return clone(CATALOG)
  },
  async refreshCatalog() {
    await delay(700)
    CATALOG.fetchedAt = new Date().toISOString()
    CATALOG.fromCache = false
    return clone(CATALOG)
  },
  async inspectProject(path) {
    await delay(500)
    if (!isAbsolute(path)) throw new Error('The path must be absolute, for example C:\\Users\\Jose\\projects\\my-app.')
    return {
      path,
      name: path.split(/[\\/]/).filter(Boolean).at(-1) ?? path,
      isGit: true,
      fileCount: 1284,
      sizeBytes: 19_300_000,
      harnessFiles: [
        { path: 'AGENTS.md', readBy: ['opencode', 'codex'] },
        { path: '.claude/', readBy: ['claude'] },
      ],
      excluded: ['.env', 'node_modules/ (.gitignore)', 'dist/ (.gitignore)'],
      profile: { ...DEFAULT_PROFILE },
    }
  },
  async startComparison(input) {
    await delay(400)
    const id = String(nextId++).padStart(4, '0')
    const now = Date.now()
    const c: Comparison = {
      id,
      createdAt: new Date(now).toISOString(),
      projectPath: input.projectPath,
      projectName: input.projectPath.split(/[\\/]/).filter(Boolean).at(-1) ?? input.projectPath,
      prompt: input.prompt,
      harness: "project's harness",
      sides: { A: makeSide('A', input.sides.A, { files: DIFFS.A.files }), B: makeSide('B', input.sides.B, { files: DIFFS.B.files }) },
      report: 'none',
    }
    comparisons.set(id, c)
    activeId = id
    sims.set(simKey(id, 'A'), newSim(now, { input: 150, cacheRead: 440, cacheWrite: null, output: 52 }))
    sims.set(simKey(id, 'B'), newSim(now, { input: 210, cacheRead: 470, cacheWrite: null, output: 60 }))
    runSide(c, 'A')
    runSide(c, 'B')
    return { id }
  },
  async getActiveComparison() {
    await delay(80)
    return activeId ? snapshot(activeId) : null
  },
  async getComparison(id) {
    await delay(80)
    return snapshot(id)
  },
  async finishSide(id, side) {
    await delay(150)
    setStatus(id, side, 'finished', 'Finished by the user')
  },
  async cancelSide(id, side) {
    await delay(150)
    setStatus(id, side, 'cancelled', 'Cancelled by the user')
  },
  async getLogs() {
    await delay()
    return clone(LOGS)
  },
  async getDiff(_id, side) {
    await delay()
    return clone(DIFFS[side])
  },
  async getTimeline() {
    await delay()
    return clone(TIMELINE)
  },
  async getTestOutput() {
    await delay()
    return [...TEST_OUTPUT]
  },
  async listHistory() {
    await delay()
    return [...comparisons.keys()]
      .map(snapshot)
      .filter(c => TERMINAL_STATUSES.includes(c.sides.A.status) && TERMINAL_STATUSES.includes(c.sides.B.status))
      .sort((x, y) => y.createdAt.localeCompare(x.createdAt))
  },
  async deleteComparison(id) {
    await delay()
    comparisons.delete(id)
  },
  async getReport(id) {
    await delay(300)
    const c = comparisons.get(id)
    if (!c || c.report !== 'ready') return null
    return clone(REPORTS[id] ?? { ...REPORTS['0142'], comparisonId: id })
  },
  async generateReport(id) {
    const c = comparisons.get(id)
    if (!c) return
    c.report = 'generating'
    window.setTimeout(() => {
      c.report = 'ready'
    }, 2500)
  },
  async listPresets() {
    await delay()
    return clone(PRESETS)
  },
  async getPreset(slug) {
    await delay()
    const p = PRESETS.find(x => x.slug === slug)
    if (!p) throw new Error(`Preset "${slug}" does not exist`)
    return clone(p)
  },
  async getPresetFile(_slug, path) {
    await delay(120)
    return path.endsWith('AGENTS.md') ? AGENTS_MD : `# ${path}\n\nSample content.`
  },
  async getSettings() {
    await delay()
    return clone(settings)
  },
  async updateSettings(patch) {
    await delay()
    settings = { ...settings, ...clone(patch) }
    return clone(settings)
  },
  openTerminal: mockTerminal,
}
