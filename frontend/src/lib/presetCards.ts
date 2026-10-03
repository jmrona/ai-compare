// Preset cards: ready-made blocks of instructions to build a preset's AGENTS.md by picking them.
// They are written for AGENTS.md, which opencode reads; translating them to each CLI's own files
// (CLAUDE.md, .claude/…) comes with the other CLIs in phase 3.

export interface PresetCard {
  id: string
  title: string
  description: string
  /** The Markdown section added to AGENTS.md. */
  body: string
}

export const PRESET_CARDS: PresetCard[] = [
  {
    id: 'small-changes',
    title: 'Small, focused changes',
    description: 'Change only what the task needs; no drive-by refactors.',
    body: `## Scope
- Change only what the task needs. Do not refactor, rename or reformat unrelated code.
- Keep public interfaces as they are unless the task asks to change them.
- Prefer the smallest change that solves the problem; explain any larger change in your final message.`,
  },
  {
    id: 'tests-first',
    title: 'Tests with every change',
    description: 'Add or update tests and run them before finishing.',
    body: `## Tests
- Every behaviour you add or change gets a test; fix the code, not the test, when a test fails.
- Run the project's test command before you finish and report the result.
- Cover edge cases: empty input, invalid input, boundaries.`,
  },
  {
    id: 'read-before-writing',
    title: 'Read before writing',
    description: 'Explore the project and follow its conventions.',
    body: `## Working in this project
- Before writing code, read the files you will touch and the tests next to them.
- Follow the project's existing patterns, naming and structure; do not introduce new libraries without need.
- Look for an existing helper before writing a new one.`,
  },
  {
    id: 'error-handling',
    title: 'Explicit error handling',
    description: 'No swallowed errors; clear messages.',
    body: `## Errors
- Never swallow errors silently; handle them or pass them up with context.
- Validate input at the boundaries (requests, files, user input) and fail with a clear message.
- Error messages say what went wrong and what to do about it.`,
  },
  {
    id: 'security',
    title: 'Security basics',
    description: 'No secrets in code, safe input handling.',
    body: `## Security
- Never write secrets, tokens or keys into code or config; read them from the environment.
- Treat all external input as untrusted: validate it, escape it in HTML, and use parameterised queries.
- Do not weaken authentication, authorisation or TLS checks to make something work.`,
  },
  {
    id: 'typescript-strict',
    title: 'Strict TypeScript',
    description: 'Types over any; no non-null assertions.',
    body: `## TypeScript
- Keep strict mode on; do not use \`any\` or \`@ts-ignore\`. Prefer precise types and unions.
- Avoid non-null assertions (\`!\`); handle the null case.
- Export types that other modules need instead of redefining them.`,
  },
  {
    id: 'docs',
    title: 'Document what changes',
    description: 'Comments for the why; README for behaviour changes.',
    body: `## Documentation
- Comments explain why, not what; keep them short and up to date.
- When behaviour, configuration or commands change, update the README or the relevant docs in the same change.`,
  },
  {
    id: 'accessibility',
    title: 'Accessible UI',
    description: 'Semantic HTML, labels, keyboard support.',
    body: `## Accessibility
- Use semantic HTML (buttons for actions, links for navigation, headings in order).
- Every form control has a label; images have alt text.
- Everything works with the keyboard and has a visible focus style.`,
  },
  {
    id: 'performance',
    title: 'Mind performance',
    description: 'Avoid needless work in hot paths.',
    body: `## Performance
- Avoid repeated work in loops and render paths; cache or hoist what does not change.
- Do not load or send more data than needed.
- Measure before optimising anything that is not obviously slow.`,
  },
  {
    id: 'final-summary',
    title: 'Finish with a summary',
    description: 'End with what changed and how it was checked.',
    body: `## When you finish
- End with a short summary: the files you changed, what you did in each, and how you checked it (tests run and their result).
- Mention anything you could not do or are unsure about.`,
  },
]

/** Builds AGENTS.md from cards, or appends them to an existing one. */
export function agentsMdWith(cards: PresetCard[], existing = ''): string {
  const sections = cards.map(c => c.body.trim())
  const head = existing.trim() || '# Instructions'
  return [head, ...sections].join('\n\n') + '\n'
}
