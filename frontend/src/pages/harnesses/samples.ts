// Sample presets for the /harnesses page, a preview of phase 2 (presets are not built yet).

import type { Preset } from '@/api/types'

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
