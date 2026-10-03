# ai-compare documentation

This folder explains how ai-compare works and why it is built the way it is. It is written for engineers and AI agents who need to understand, change or extend the project without having been part of the conversations that shaped it.

[`PLAN.md`](../PLAN.md) at the repo root is the original product and technical plan (in Spanish). It describes the full vision, including what has not been built yet. These documents describe **what exists in the code today** and mark planned work explicitly.

## Reading order

If you are new, read the first four in order. The rest are reference.

| # | Document | What it covers |
|---|---|---|
| 1 | [Product](01-product.md) | What ai-compare is for, the main flow, scope by phase and what is out of scope |
| 2 | [Architecture](02-architecture.md) | The pillars (browser, api, proxy, Docker, agent containers, Postgres, models.dev) and how they talk to each other |
| 3 | [Repository layout](03-repository-layout.md) | Every folder and the important files in it |
| 4 | [Comparison lifecycle](04-comparison-lifecycle.md) | What happens from "Run comparison" to the end of both sides, statuses, timings, restarts |
| 5 | [Project copy and side images](05-project-copy-and-images.md) | How the user's folder is copied safely and turned into one image per side |
| 6 | [Inference proxy](06-inference-proxy.md) | How model traffic is routed, measured, priced and limited |
| 7 | [Terminals](07-terminals.md) | Live TTYs in the browser: attach, hub, WebSocket protocol, sizing, replay |
| 8 | [Networking and security](08-networking-and-security.md) | Networks, ports, isolation of agents, secrets, threat model |
| 9 | [Models and pricing](09-models-and-pricing.md) | The models.dev catalogue, caching, filtering, price snapshots |
| 10 | [API and contracts](10-api-and-contracts.md) | Connect services, the remaining JSON routes, the terminal WebSocket, code generation |
| 11 | [Database](11-database.md) | PostgreSQL schema, migrations (goose), queries (sqlc), what is persisted |
| 12 | [Frontend](12-frontend.md) | React app structure, routing, data fetching, the hybrid mock client, styling |
| 13 | [Agent CLIs](13-agent-clis.md) | How opencode is installed, configured and started; how codex and claude will fit |
| 14 | [Tools and libraries](14-tools.md) | Every tool and library in use, what it does here and why it was chosen |
| 15 | [Development](15-development.md) | Running, configuration, scripts, tests, code generation, debugging, spike commands |
| 16 | [Platforms](16-platforms.md) | macOS, Windows and Linux differences and how each is handled |
| 17 | [Decisions](17-decisions.md) | Decision log: each significant choice, the alternatives and the reasoning |
| 18 | [Status and roadmap](18-status-and-roadmap.md) | What works, known limitations, gaps between plan and code, next steps |

## Glossary

| Term | Meaning |
|---|---|
| **Comparison** | One run of the same prompt on the same project with two configurations. Ids of real comparisons start with `r` (for example `ra3f80e`); sample data uses numbers. |
| **Side** | One of the two configurations, `A` or `B`. Each side has its own image, container, proxy session and terminal. |
| **Configuration** | Per side: CLI, provider, model, effort, mode and optional limits. |
| **CLI / agent** | The coding agent that runs inside a side container: `opencode` today, `codex` and `claude` later. |
| **Harness** | Files that instruct an agent: `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.opencode/`, `opencode.json`, `.mcp.json` and so on. Phase 1 uses the project's own harness as it is. |
| **Preset** | A reusable harness stored by ai-compare (phase 2). |
| **Profile** | How to run a project: base image (runtime), setup command, test command. Detected from the project and editable. |
| **Staging volume** | The Docker volume `ai-compare_staging` where the project copy lives before it is built into images. |
| **Baseline** | The Git commit made inside each side image right after the copy, so the diff shows only what the agent changed. |
| **Inference proxy** | The part of `api` on port 4701 that agent containers call instead of the provider. |
| **Side token** | A random `aic_…` string that a side uses as its API key. The proxy swaps it for the real key. |
| **Mode** | `autonomous` (the CLI runs to completion on its own) or `interactive` (the TUI stays open and the user answers in the browser terminal). |
| **Hub** | The in-memory object that owns a side's terminal: buffers its output and fans it out to every connected browser. |
| **models.dev** | The public catalogue of models and prices that ai-compare uses as its only price source. |
| **Phase 0 / 1 / 2** | Project stages: 0 is stack and spike (done), 1 is comparing OpenAI models with opencode, 2 adds Anthropic, local models, more CLIs and presets. |
