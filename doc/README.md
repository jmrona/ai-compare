# ai-compare documentation

This folder explains how ai-compare works and why it is built the way it is. It is written for engineers and AI agents who need to understand, change or extend the project without having been part of the conversations that shaped it.

[`PLAN.md`](../PLAN.md) at the repo root is the original product and technical plan. It describes the full vision, including what has not been built yet. These documents describe **what exists in the code today** and mark planned work explicitly.

## Reading order

If you are new, read the first four in order. The rest are reference.

| # | Document | What it covers |
|---|---|---|
| 1 | [Product](01-product.md) | What ai-compare is for, the main flow, scope by phase and what is out of scope |
| 2 | [Architecture](02-architecture.md) | The pillars (browser, api, proxy, Docker, agent containers, Postgres, models.dev) and how they talk to each other |
| 3 | [Repository layout](03-repository-layout.md) | Every folder and the important files in it |
| 4 | [Comparison lifecycle](04-comparison-lifecycle.md) | What happens from "Run comparison" to the end of both sides: statuses, verification, timings, downloads, restarts |
| 5 | [Project copy and side images](05-project-copy-and-images.md) | How the user's folder is copied safely, turned into one image per side, and how results are collected, tested and cleaned up |
| 6 | [Inference proxy](06-inference-proxy.md) | How model traffic is routed, measured, priced and limited |
| 7 | [Terminals](07-terminals.md) | Live TTYs in the browser: attach, hub, WebSocket protocol, sizing, timed recordings and replay |
| 8 | [Networking and security](08-networking-and-security.md) | Networks, ports, isolation of agents, secrets, threat model |
| 9 | [Models and pricing](09-models-and-pricing.md) | The models.dev catalogue, caching, filtering, price snapshots |
| 10 | [API and contracts](10-api-and-contracts.md) | Connect services, the event stream, the plain HTTP routes (terminal, download, recording), code generation |
| 11 | [Database](11-database.md) | PostgreSQL schema, migrations (goose), queries (sqlc), what is persisted |
| 12 | [Frontend](12-frontend.md) | React app structure, routing, data fetching with connect-query and the event stream, styling |
| 13 | [Agent CLIs](13-agent-clis.md) | How opencode is installed, configured and started; how codex and claude will fit |
| 14 | [Tools and libraries](14-tools.md) | Every tool and library in use, what it does here and why it was chosen |
| 15 | [Development](15-development.md) | Running, configuration, scripts, tests, code generation, debugging, spike commands |
| 16 | [Platforms](16-platforms.md) | macOS, Windows and Linux differences and how each is handled |
| 17 | [Decisions](17-decisions.md) | Decision log: each significant choice, the alternatives and the reasoning |
| 18 | [Status and roadmap](18-status-and-roadmap.md) | What works, known limitations, gaps between plan and code, next steps |
| 19 | [Harnesses and presets](19-harnesses-and-presets.md) | What a harness is, the three choices per side, presets, where they are stored and how to see them with Docker, what each comparison keeps |
| 20 | [Reports](20-reports.md) | Acceptance criteria, gates, the 0–100 score, the judge, harness cost and audit, subagents, session figures, export |

## Glossary

| Term | Meaning |
|---|---|
| **Comparison** | One run of the same prompt on the same project with two configurations. Ids start with `r` followed by six hex characters (for example `ra3f80e`). |
| **Side** | One of the two configurations, `A` or `B`. Each side has its own image, container, proxy session and terminal. |
| **Configuration** | Per side: CLI, provider, model, effort, mode and optional limits. |
| **CLI / agent** | The coding agent that runs inside a side container: `opencode` today, `codex` and `claude` later. It runs as the unprivileged user `agent`. |
| **Harness** | Files that instruct an agent: `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.opencode/`, `opencode.json`, `.mcp.json` and so on. Each side runs with the project's own, a preset or none; see [Harnesses and presets](19-harnesses-and-presets.md). |
| **Preset** | A reusable harness stored by ai-compare (phase 2). |
| **Profile** | How to run a project: base image (runtime), setup command, test command and an optional hidden tests folder. Detected from the project and editable. |
| **Hidden tests** | A host folder of tests the agent never sees. It is copied next to the project copy and added to the side's result only to verify it. |
| **Staging volume** | The Docker volume `ai-compare_staging` where the project copy (and the hidden tests) live before they are built into images. |
| **Baseline** | The Git commit made inside each side image right after the copy, so the diff shows only what the agent changed. |
| **Verifying** | The status between the end of the agent and a side's final status: the result is collected and the tests run. |
| **Outcome** | The final status decided when the agent stops (CLI exit, Finish, Cancel, timeout, limit). The first decision wins; the side gets it once verification ends. |
| **Failure kind** | For sides that end in `error`: `agent` (the CLI exited with a non-zero code) or `infrastructure` (copy, build, start, a lost container or a restart during preparation). |
| **Result image** | `ai-compare/result:<id>-<side>`: the side's stopped container committed as an image. Collection and test containers start from it. |
| **Artefacts** | Files kept per side in the volume `ai-compare_artifacts` under `<id>/<side>`: `workspace.tar`, the diffs, the CLI session, test output and the terminal recording. Retention never removes them. |
| **Recording** | `terminal.cast`, the side's terminal in asciicast v2 format (output and resizes with their timing), replayed in the history. |
| **Human wait** | Interactive sides only: time the agent spent waiting for the user, estimated from when the user typed and the gaps between model requests. Not counted as agent time. |
| **Report** | Blind review and analysis of each side, then a comparative judgement, written by the report model from settings (`gpt-6-luna` by default). |
| **Retention** | The hourly clean-up of the containers, images and staging copies of comparisons that ended more than N days ago (2 by default). Only objects ai-compare created are touched. |
| **Inference proxy** | The part of `api` on port 4701 that agent containers (and the report) call instead of the provider. |
| **Side token** | A random `aic_…` string that a side uses as its API key. The proxy swaps it for the real key. |
| **Mode** | `autonomous` (the CLI runs to completion on its own) or `interactive` (the TUI stays open and the user answers in the browser terminal). |
| **Hub** | The in-memory object that owns a side's terminal: buffers its output, fans it out to every connected browser, records it and notes when the user types. |
| **Event stream** | `EventService.Watch`, the Connect server stream that pushes every change of a comparison to the browser. Nothing polls. |
| **models.dev** | The public catalogue of models and prices that ai-compare uses as its only price source. |
| **Phase 0 / 1 / 2 / 3** | Project stages: 0 is stack and spike, 1 is comparing OpenAI models with opencode (both done, macOS still to be checked by hand), 2 keeps opencode and adds Anthropic, presets, previews and repetitions, 3 adds local models and the `claude` and `codex` CLIs. |
