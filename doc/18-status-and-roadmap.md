# 18. Status and roadmap

State as of October 2026.

## What works

- `docker compose up -d` brings up the whole app; no host toolchain.
- Real comparisons with **opencode** and **OpenAI** models: project inspection, copy without secrets, one image per side, both sides in parallel, autonomous and interactive modes.
- Live terminals in the browser with replay on reconnect and read-only replay in the history.
- Inference proxy with per-side tokens, usage from Chat Completions, Responses and Anthropic Messages (JSON and SSE), provider errors (including inside streams), cost with long-context tiers, optional token, cost and time limits.
- Live metrics: tokens, cost, requests, errors, tokens per second, preparation time per phase.
- Finish, Cancel and zip download per side.
- models.dev catalogue on the Pricing page and in the model dropdowns, newest first, with per-model efforts; cached and refreshed with ETags.
- PostgreSQL persistence: history, metrics, logs, requests and terminal output survive restarts; interrupted sides are closed cleanly.
- Network isolation of agents, verified.
- Protobuf contract with Connect (`CatalogService`, `ProjectService`), generated Go and TypeScript clients.
- Comparisons with or without a project; a folder browser to pick the project folder.
- `pnpm gen` and `pnpm test` in Docker.

## Phase 0 checklist

| Item | State |
|---|---|
| Compose stack with `api` and `postgres` | Done |
| Go server with health check and migrations | Done |
| Vite frontend with shadcn and navigation | Done |
| Protobuf contract with generated Go and TypeScript, plus sqlc | Done |
| pnpm scripts `dev`, `gen`, `build`, `lint`, `test` | Done |
| Spike 1: copy a host path from a helper container | Done |
| Spike 2: build the side image from the copy | Done |
| Spike 3: agent network reaches the proxy and nothing else | Done |
| Spike 4: TTY over WebSocket with typing, resize and Ctrl+C | Done |
| Spike 5: opencode through the proxy, interactive and autonomous | Done, wired to the UI |
| Spike 6: usage and cost from responses, streaming or not | Done |
| Spike 7: reaching the host for local models | Done (with a stand-in server) |
| macOS check | **Pending** |

## Known limitations

- Sample data still fills the Changes, Tests and Events tabs, reports, presets and settings.
- Most comparison routes are still JSON; the catalogue and projects are on Connect.
- Live views poll every second; the event stream is not built.
- After an `api` restart, running sides are closed instead of reattached.
- Terminal recordings are raw output (final screen), not timed asciicast.
- Agents run as root in their containers.
- No cleanup of old images, containers and staging copies.
- `humanWaitSec` is not measured.
- The runtime must contain Node.js (opencode is installed with npm).
- Starting a second comparison while one runs is only prevented by the UI.

## Where the code differs from PLAN.md

| PLAN.md says | The code does | Reason |
|---|---|---|
| Frontend embedded in the Go binary with `embed.FS` | Served from `/app/web` in the image | Simpler Docker build; same effect in practice |
| `air` for hot reload | Not set up; rebuild with `docker compose up -d --build` | Not needed yet |
| Images built with BuildKit | Classic builder (`BuilderV1`) | Works through the plain Engine API; revisit if needed |
| Agents on the `ai-compare` network | A separate `ai-compare-agents` network with netguard | Stronger isolation (no route to Postgres) |
| `pnpm gen` via `npx @bufbuild/buf` | A pinned Docker image | No host toolchain, identical output |
| Package `store/` with tables `projects`, `side_transitions`, `requests`, `price_snapshots`, `reports`, `findings` | Package `db/` with `comparisons` and `comparison_sides` (JSONB) | Smallest schema that persists what exists; grows with the features |
| Non-root user in side images, with `ripgrep` | Root, no ripgrep | Not done yet |
| Recordings in asciicast v2 | Raw output | Not done yet |

## Next (phase 1)

The checklist lives in [PLAN.md → Phase 1](../PLAN.md#phase-1--comparing-openai-models-with-the-projects-harness). In order:

1. **Foundations:** remaining routes to Connect, `EventService.Watch` instead of polling (with `connect-query`), reattaching to live containers after a restart, non-root agents, optional project and a folder browser on the New comparison page.
2. **Changes tab:** solution diff against `baseline` without harness files, harness diff apart, files and lines changed.
3. **Verification:** the profile's tests in a fresh container, optional hidden tests, real Tests tab.
4. **Report:** blind reviewer, per-side analyst and judge with `gpt-6-luna` by default; real report page.
5. **History:** timed recordings, Events from opencode's session files, human wait time, each side's result kept as an artefact so it can be downloaded after retention.
6. **Settings and retention:** real `/settings` (suggested limits 30 min, 2M tokens, $2); retention after 2 days of the containers, images and staging copies **created by ai-compare only**; artefacts and reports kept; mocks removed.

Decided for phase 1: no UI session token for now (local, single-user, bound to 127.0.0.1).

## Later (phase 2)

- Anthropic and local models for opencode; local model list from `/v1/models`; shared-GPU warning and optional sequential runs.
- `claude` and `codex` adapters, with the warning that they read different harness files.
- Presets (`/harnesses`), "no harness", and excluding the project's harness.
- App previews per side through subdomains (`a-<id>.localhost`).
- N repetitions per side with aggregates; cost versus quality chart.
- Preset adviser.
