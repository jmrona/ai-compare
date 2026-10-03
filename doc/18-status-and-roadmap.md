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
- Protobuf contract with Connect (`CatalogService`), generated Go and TypeScript clients.
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
- Most API routes are still JSON; only the catalogue is on Connect.
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
| UI protected by a session token | No token; localhost binding only | Single-user local app; may be added |
| `pnpm gen` via `npx @bufbuild/buf` | A pinned Docker image | No host toolchain, identical output |
| Package `store/` with tables `projects`, `side_transitions`, `requests`, `price_snapshots`, `reports`, `findings` | Package `db/` with `comparisons` and `comparison_sides` (JSONB) | Smallest schema that persists what exists; grows with the features |
| Non-root user in side images, with `ripgrep` | Root, no ripgrep | Not done yet |
| Recordings in asciicast v2 | Raw output | Not done yet |

## Next (phase 1)

**1a — Launch and watch**

- Reattach to live containers after an `api` restart instead of closing them.
- Move the remaining routes to Connect (`ComparisonService`, `ProjectService`) and add `EventService.Watch` to replace polling; adopt `connect-query`.

**1b — Measure and compare**

- Changes tab: diff of each side's `/workspace` against its `baseline` commit, excluding harness files.
- Test verification in a fresh container from the side's result, with the profile's test command and optional hidden tests.
- Timed terminal recordings for replay.
- Reports: blind reviewer, analyst and judge.
- Settings page on the backend (defaults, local model URL, retention).
- Retention and cleanup of images, containers and staging copies.

## Later (phase 2)

- Anthropic and local models for opencode; local model list from `/v1/models`; shared-GPU warning and optional sequential runs.
- `claude` and `codex` adapters, with the warning that they read different harness files.
- Presets (`/harnesses`), "no harness", and excluding the project's harness.
- App previews per side through subdomains (`a-<id>.localhost`).
- N repetitions per side with aggregates; cost versus quality chart.
- Preset adviser.
