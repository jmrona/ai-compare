// Sample data for building the UI without a backend (VITE_USE_MOCKS=true).
// Dates are relative to "now" so Today/Yesterday always have content.
// Prices are sample values, not real models.dev prices.

import type {
  Catalog,
  Comparison,
  DiffLine,
  Limits,
  LogEntry,
  Preset,
  Report,
  Settings,
  SideConfig,
  SideMetrics,
  SideRun,
  TimelineEvent,
} from '@/api/types'
import { estimateCost } from '@/lib/format'

export const NOW = Date.now()
const MIN = 60_000
const DAY = 24 * 60 * MIN

/** ISO date `days` days ago at the given time. */
function at(days: number, hhmm: string): string {
  const d = new Date(NOW - days * DAY)
  const [h, m] = hhmm.split(':').map(Number)
  d.setHours(h, m, 0, 0)
  return d.toISOString()
}

export const CATALOG: Catalog = {
  source: 'models.dev',
  fetchedAt: new Date(NOW - 38 * MIN).toISOString(),
  fromCache: true,
  models: [
    { id: 'gpt-5.5', provider: 'openai', contextK: 400, price: { input: 5, cacheRead: 0.5, cacheWrite: null, output: 30 } },
    { id: 'gpt-5.5-mini', provider: 'openai', contextK: 400, price: { input: 0.75, cacheRead: 0.075, cacheWrite: null, output: 4.5 } },
    { id: 'gpt-5.5-nano', provider: 'openai', contextK: 400, price: { input: 0.2, cacheRead: 0.02, cacheWrite: null, output: 1.25 } },
    { id: 'gpt-5.5-codex', provider: 'openai', contextK: 400, price: null },
  ],
}

export const priceOf = (model: string) => CATALOG.models.find(m => m.id === model)?.price ?? null

export const DEFAULT_PROFILE = { runtime: 'node:22', setup: 'npm ci', test: 'npm test', hiddenTestsPath: '' }

export const PROJECT_PATH = 'C:\\Users\\Jose\\Desktop\\projects\\invoices-web'

export const PROMPT_0142 =
  'Add cursor-based pagination to the GET /api/invoices endpoint. Keep the existing page parameter working, add tests for the new behaviour and update the client in src/lib/api.ts.'

const noLimits: Limits = { timeoutMin: null, maxTokensK: null, maxCostUsd: null }

/** Partial side used to build fixtures; metrics can be partial too. */
export type SidePatch = Omit<Partial<SideRun>, "metrics"> & { metrics?: Partial<SideMetrics> }

export function makeSide(key: 'A' | 'B', config: SideConfig, partial: SidePatch): SideRun {
  const usage = partial.metrics?.usage ?? { input: 0, cacheRead: 0, cacheWrite: null, output: 0 }
  const price = priceOf(config.model)
  return {
    key,
    config,
    cliVersion: '1.9.2',
    status: 'pending',
    files: [],
    priceSnapshot: { price, fetchedAt: CATALOG.fetchedAt },
    ...partial,
    metrics: {
      elapsedSec: 0,
      agentSec: 0,
      humanWaitSec: config.mode === 'interactive' ? 0 : null,
      prepSec: 32,
      costConfirmedUsd: null,
      requests: 0,
      retries: 0,
      errors: 0,
      tokensPerSec: null,
      ...partial.metrics,
      usage,
      costUsd: estimateCost(usage, price),
    },
  }
}

function finished(
  id: string,
  createdAt: string,
  project: string,
  prompt: string,
  a: [SideConfig, SidePatch],
  b: [SideConfig, SidePatch],
  report: Comparison['report'],
): Comparison {
  return {
    id,
    createdAt,
    projectPath: 'C:\\Users\\Jose\\Desktop\\projects\\' + project,
    projectName: project,
    prompt,
    harness: "project's AGENTS.md",
    sides: { A: makeSide('A', a[0], { status: 'finished', ...a[1] }), B: makeSide('B', b[0], { status: 'finished', ...b[1] }) },
    report,
  }
}

const cfg = (model: string, effort: SideConfig['effort'], mode: SideConfig['mode'], limits: Limits = noLimits): SideConfig => ({
  cli: 'opencode',
  provider: 'openai',
  model,
  effort,
  mode,
  limits,
})

function m(elapsedSec: number, agentSec: number, input: number, cacheRead: number, output: number, tps: number, humanWaitSec?: number) {
  return {
    elapsedSec,
    agentSec,
    humanWaitSec: humanWaitSec ?? null,
    usage: { input, cacheRead, cacheWrite: null, output },
    requests: Math.round(agentSec / 25),
    retries: 0,
    tokensPerSec: tps,
  }
}

export const HISTORY: Comparison[] = [
  finished('0141', at(0, '08:12'), 'stock-api', 'Fix the reserved stock calculation when an order is cancelled and add a regression test',
    [cfg('gpt-5.5', 'high', 'autonomous'), { metrics: m(860, 720, 141_000, 402_000, 21_000, 38), tests: { passed: 42, total: 42 } }],
    [cfg('gpt-5.5', 'medium', 'autonomous'), { metrics: m(725, 725, 118_000, 344_000, 16_500, 41), tests: { passed: 41, total: 42 } }],
    'none'),
  finished('0140', at(1, '18:30'), 'invoices-web', 'Migrate the new-customer form to react-hook-form with zod validation',
    [cfg('gpt-5.5-mini', 'high', 'interactive'), { metrics: m(722, 610, 160_000, 512_000, 24_000, 88, 64), tests: { passed: 29, total: 31 } }],
    [cfg('gpt-5.5', 'high', 'interactive'), { metrics: m(947, 830, 96_000, 300_000, 31_000, 52, 117), tests: { passed: 31, total: 31 } }],
    'ready'),
  finished('0139', at(1, '11:04'), 'invoices-web', 'Add CSV export to the invoice list, honouring the active filters',
    [cfg('gpt-5.5', 'high', 'autonomous', { timeoutMin: 20, maxTokensK: null, maxCostUsd: null }), { status: 'limit_reached', endReason: '20 min timeout', metrics: m(1200, 1200, 210_000, 690_000, 40_000, 44) }],
    [cfg('gpt-5.5-mini', 'medium', 'autonomous'), { metrics: m(511, 511, 88_000, 230_000, 12_000, 97), tests: { passed: 12, total: 14 } }],
    'none'),
  finished('0138', at(2, '16:55'), 'stock-api', 'Refactor the notifications module to use a queue with retries',
    [cfg('gpt-5.5', 'high', 'autonomous'), { metrics: m(1155, 1155, 180_000, 610_000, 36_000, 47), tests: { passed: 55, total: 55 } }],
    [cfg('gpt-5.5-codex', 'high', 'autonomous'), { metrics: m(1060, 1060, 170_000, 560_000, 33_000, 51), tests: { passed: 53, total: 55 } }],
    'ready'),
]

export const LOGS: LogEntry[] = [
  { at: '09:41:02', level: 'info', source: 'copy', message: 'Copying project from the given path (read-only)' },
  { at: '09:41:05', level: 'info', source: 'copy', message: '1,284 files · 18.4 MB · .env excluded · harness copied as is' },
  { at: '09:41:06', level: 'info', source: 'build', message: 'Project layer: node:22 + npm ci (cached)' },
  { at: '09:41:31', level: 'info', source: 'build', message: 'Side layer: opencode 1.9.2 + Git baseline a1f93c2' },
  { at: '09:41:34', level: 'info', source: 'run', message: 'Container started · 2 CPUs · 4 GB memory' },
  { at: '09:41:35', level: 'info', source: 'proxy', message: 'POST /openai/v1/responses 200 · 1.9 s · in 12.4k · out 0.8k' },
  { at: '09:44:10', level: 'warn', source: 'proxy', message: '429 rate limit · the CLI retries in 2 s' },
  { at: '09:44:12', level: 'info', source: 'proxy', message: 'POST /openai/v1/responses 200 · 4.1 s · in 48.2k · out 2.1k' },
  { at: '09:53:47', level: 'info', source: 'run', message: 'No requests in flight · the terminal is waiting for input' },
]

const DIFF_LINES: DiffLine[] = [
  { kind: '@@', text: '@@ -18,12 +18,31 @@ router.get("/api/invoices", async (req, res) => {' },
  { kind: ' ', text: '   const limit = clamp(Number(req.query.limit ?? 50), 1, 200);' },
  { kind: '-', text: '-  const page = Number(req.query.page ?? 1);' },
  { kind: '-', text: '-  const rows = await repo.list({ offset: (page - 1) * limit, limit });' },
  { kind: '+', text: '+  if (req.query.page !== undefined) {' },
  { kind: '+', text: '+    const page = Number(req.query.page);' },
  { kind: '+', text: '+    return res.json(await repo.list({ offset: (page - 1) * limit, limit }));' },
  { kind: '+', text: '+  }' },
  { kind: '+', text: '+  const cursor = req.query.cursor ? decodeCursor(req.query.cursor) : null;' },
  { kind: '+', text: '+  const rows = await repo.listAfter({ cursor, limit: limit + 1 });' },
  { kind: '+', text: '+  const items = rows.slice(0, limit);' },
  { kind: '+', text: '+  res.json({ items, nextCursor: rows.length > limit ? encodeCursor(items.at(-1).issuedAt) : null });' },
  { kind: ' ', text: ' });' },
]

export const DIFFS = {
  A: { files: [{ path: 'src/routes/invoices.ts', added: 46, removed: 9 }, { path: 'src/routes/__tests__/invoices.cursor.test.ts', added: 88, removed: 0 }], lines: DIFF_LINES },
  B: { files: [{ path: 'src/routes/invoices.ts', added: 38, removed: 6 }, { path: 'src/db/invoices.repo.ts', added: 21, removed: 2 }, { path: 'src/lib/api.ts', added: 12, removed: 3 }], lines: DIFF_LINES },
}

export const TIMELINE: TimelineEvent[] = [
  { at: '00:00', kind: 'start', detail: 'CLI started with the prompt' },
  { at: '00:14', kind: 'read', detail: 'src/routes/invoices.ts, src/lib/api.ts' },
  { at: '03:02', kind: 'edit', detail: 'src/routes/invoices.ts +46 −9' },
  { at: '05:40', kind: 'command', detail: 'npm test -- invoices · exit 0' },
  { at: '08:10', kind: 'question', detail: 'scope of the view migration' },
  { at: '09:58', kind: 'answer', detail: 'user: option 1' },
  { at: '10:53', kind: 'end', detail: 'finished by the user' },
]

export const TEST_OUTPUT = [
  '\x1b[90mnpm test · fresh container with the diff applied\x1b[0m',
  ' PASS  src/routes/__tests__/invoices.test.ts (14)',
  ' PASS  src/routes/__tests__/invoices.cursor.test.ts (4)',
  '\x1b[91m FAIL  hidden/invoices.duplicate-dates.test.ts\x1b[0m',
  '\x1b[91m   ✕ does not skip invoices with the same date (38 ms)\x1b[0m',
  '\x1b[90m     Expected: 51 items   Received: 50 items\x1b[0m',
  '',
  ' Tests: \x1b[91m1 failed\x1b[0m, \x1b[92m23 passed\x1b[0m, 24 total',
]

export const REPORTS: Record<string, Report> = {
  '0142': {
    comparisonId: '0142',
    model: 'gpt-5.5',
    costUsd: 0.22,
    verdicts: [
      { label: 'lower cost', side: 'B' },
      { label: 'shorter run', side: 'B' },
      { label: 'fewer issues', side: 'A' },
      { label: '1 run per side · low confidence', side: null },
    ],
    conclusions: [
      'B cost far less and finished sooner, but added no tests for the new behaviour and returns 500 on a malformed cursor. A covered the change with 4 new tests and followed the error rule in AGENTS.md, although its cursor can skip invoices that share a date.',
      'On the hidden tests, A passed 5 of 6 and B 4 of 6. Both fail the duplicate-date case; B also fails the invalid-cursor case. For this task, A leaves a base closer to something you would merge. B is worth it if cost matters most and the tests are added afterwards.',
    ],
    perSide: {
      A: 'Facts: read 6 files, ran the tests 3 times and asked once about scope. Inference: the AGENTS.md rule "every new route gets tests" explains the 4 added tests.',
      B: 'Facts: 24 requests, no questions, 3 files changed. Inference: it prioritised client compatibility over test coverage, despite having the same rule.',
    },
    findings: [
      { severity: 'high', side: 'A', title: 'The cursor only uses the date and can skip invoices', location: 'src/routes/invoices.ts:58', impact: 'Two invoices with the same date at a page boundary: the second is never returned. The id is missing as a tie-breaker.' },
      { severity: 'medium', side: 'B', title: 'A malformed cursor returns 500', location: 'src/routes/invoices.ts:41', impact: 'decodeCursor throws an uncaught exception. The project rules ask for 400 with { error, field }.' },
      { severity: 'low', side: 'B', title: 'No tests for cursor mode', location: 'src/routes/__tests__/', impact: 'The 16 passing tests are the existing ones; the new behaviour is not covered.' },
    ],
  },
}

export const PRESETS: Preset[] = [
  {
    slug: 'strict-backend',
    title: 'Strict backend',
    description: 'REST API rules, mandatory tests and skills for migrations and API tests.',
    clis: ['opencode', 'codex', 'claude'],
    uses: 6,
    files: [
      { root: 'project', path: 'AGENTS.md', category: 'instructions' },
      { root: 'project', path: 'CLAUDE.md', category: 'instructions' },
      { root: 'project', path: '.claude/skills/migrations/SKILL.md', category: 'skills' },
      { root: 'project', path: '.claude/skills/api-tests/SKILL.md', category: 'skills' },
      { root: 'project', path: '.claude/agents/reviewer.md', category: 'agents' },
      { root: 'project', path: '.agents/rules/rest-api.md', category: 'rules' },
      { root: 'project', path: '.agents/rules/errors.md', category: 'rules' },
      { root: 'project', path: '.mcp.json', category: 'mcp' },
      { root: 'home', path: '.codex/config.toml', category: 'mcp' },
    ],
  },
  {
    slug: 'react-frontend',
    title: 'React frontend',
    description: 'Component conventions, Tailwind and accessibility. Includes a UI reviewer subagent.',
    clis: ['claude'],
    uses: 3,
    files: [{ root: 'project', path: 'CLAUDE.md', category: 'instructions' }],
  },
  {
    slug: 'minimal-agents',
    title: 'Minimal AGENTS.md',
    description: 'Only AGENTS.md with the build, test and lint commands.',
    clis: ['opencode', 'codex'],
    uses: 9,
    files: [{ root: 'project', path: 'AGENTS.md', category: 'instructions' }],
  },
]

export const AGENTS_MD = `# invoices-web · instructions

## Commands
- Install: npm ci
- Tests: npm test
- Lint: npm run lint

## Rules
- Every new API route gets integration tests.
- Do not change public signatures in src/lib/api.ts without keeping compatibility.
- Validation errors return 400 with { error, field }.`

export const SETTINGS: Settings = {
  keys: { openai: true, anthropic: false },
  defaultLimits: noLimits,
  suggestedLimits: { timeoutMin: 30, maxTokensK: 2000, maxCostUsd: 5 },
  reportModel: 'gpt-5.5',
  autoReport: false,
  resources: { cpus: 2, memoryGb: 4 },
  localBaseUrl: 'http://host.docker.internal:11434/v1',
  cliVersions: [
    { cli: 'opencode', pinned: '1.9.2', latest: '1.10.0' },
    { cli: 'codex', pinned: null, latest: null },
    { cli: 'claude', pinned: null, latest: null },
  ],
  retention: { keepStopped: true, containersDays: 7, imagesDays: 14, recordingsDays: 90 },
  disk: [
    { label: 'images', gb: 14.2 },
    { label: 'stopped containers', gb: 7.1 },
    { label: 'artefacts and reports', gb: 2.9 },
    { label: 'database', gb: 0.4 },
  ],
}

/* ── Terminal scripts (ANSI) ────────────────────────────── */

const C = { dim: '\x1b[90m', acc: '\x1b[94m', ok: '\x1b[92m', warn: '\x1b[93m', mag: '\x1b[95m', red: '\x1b[91m', x: '\x1b[0m' }

export function interactiveScript(model: string, effort: string, prompt: string) {
  const short = prompt.length > 62 ? prompt.slice(0, 62) + '…' : prompt
  return {
    beforeQuestion: [
      `${C.dim} opencode · openai/${model} · ${effort}                    /workspace${C.x}`,
      '',
      `${C.acc} > ${short}${C.x}`,
      '',
      ' ⚙ read   src/routes/invoices.ts',
      ' ⚙ read   src/lib/api.ts',
      ' ⚙ grep   "page=" src',
      `${C.dim}   7 matches in 4 files${C.x}`,
      ` ⚙ edit   src/routes/invoices.ts                ${C.ok}+46${C.x} ${C.red}−9${C.x}`,
      ' ⚙ write  src/routes/__tests__/invoices.cursor.test.ts',
      ' ⚙ bash   npm test -- invoices',
      `${C.ok}   ✓ 18 passed${C.x}`,
      '',
      `${C.warn} ? The client uses \`page\` in 3 views (List, Export, Dashboard).${C.x}`,
      `${C.warn}   Should I migrate those views to cursors too, or only the client?${C.x}`,
      '   1. Only the client; the views keep using page',
      `${C.dim}   2. Migrate the 3 views as well${C.x}`,
      '',
      `${C.acc} ❯ ${C.x}`,
    ],
    afterAnswer: [
      '',
      ` ⚙ edit   src/lib/api.ts                        ${C.ok}+12${C.x} ${C.red}−3${C.x}`,
      ' ⚙ bash   npm test',
      `${C.ok}   ✓ 22 passed${C.x}`,
      '',
      ' Done: cursor and limit on GET /api/invoices; page still works.',
      ` The client accepts both parameters. The views were not touched.${C.x}`,
      '',
      `${C.ok} [session ended]${C.x}`,
    ],
  }
}

export function autonomousScript(model: string, prompt: string) {
  const short = prompt.length > 40 ? prompt.slice(0, 40) + '…' : prompt
  return [
    `${C.dim} $ opencode run --model openai/${model} "${short}"${C.x}`,
    '',
    `${C.mag} ⚙ grep   "page" src/routes src/lib${C.x}`,
    `${C.dim}   src/routes/invoices.ts:22  const page = Number(req.query.page ?? 1)${C.x}`,
    `${C.mag} ⚙ read   src/db/invoices.repo.ts${C.x}`,
    `${C.mag} ⚙ edit   src/routes/invoices.ts                ${C.ok}+38${C.x} ${C.red}−6${C.x}`,
    `${C.mag} ⚙ edit   src/db/invoices.repo.ts               ${C.ok}+21${C.x} ${C.red}−2${C.x}`,
    `${C.mag} ⚙ edit   src/lib/api.ts                        ${C.ok}+12${C.x} ${C.red}−3${C.x}`,
    `${C.mag} ⚙ bash   npm test${C.x}`,
    `${C.ok}   ✓ 16 passed${C.x}`,
    '',
    ' Added cursor and limit to GET /api/invoices. When page is sent,',
    ' the previous behaviour is kept. The client accepts both.',
    '',
    `${C.ok} [finished · exit 0]${C.x}`,
  ]
}

export const prepLine = (step: string) => `${C.dim}[ai-compare] ${step}${C.x}`
