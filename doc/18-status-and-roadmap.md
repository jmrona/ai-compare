# 18. Status and roadmap

State as of 3 October 2026: **phase 1 is complete**, except the manual check on macOS.

## What works

- `docker compose up -d` brings up the whole app; no host toolchain.
- Real comparisons with **opencode** and **OpenAI** models: project inspection (or an empty folder), a folder browser, copy without secrets, optional hidden tests, one image per side, both sides in parallel, autonomous and interactive modes. Agents run as an unprivileged user.
- Live terminals in the browser with replay on reconnect, the final screen in the history, and **timed replay** of asciicast recordings.
- Inference proxy with per-side tokens, usage from Chat Completions, Responses and Anthropic Messages (JSON and SSE), provider errors (including inside streams), cost with long-context tiers, optional token, cost and time limits.
- Live metrics: tokens, cost, requests, errors, tokens per second, preparation and verification time per phase, human wait for interactive sides.
- **Verification** when a side's agent ends: result image, solution and harness diffs, files and lines changed, opencode's session (Events tab, usage cross-check), the profile's tests and hidden tests in fresh containers without network. Agent and infrastructure failures are told apart.
- **Live diff** while a side runs.
- **Reports:** blind review and analysis per side, comparative judgement with verdicts, warnings; through the proxy with the report model (`gpt-6-luna` by default), cost measured apart; automatic or on demand.
- Finish, Cancel (also while a side is being prepared), zip download per side (from the artefacts once ended), deleting a comparison.
- **Everything on Connect** (six services) with connect-query in the frontend, and an **event stream** (`EventService.Watch`) instead of polling.
- models.dev catalogue on the Pricing page and in the model dropdowns, newest first, with per-model efforts; cached and refreshed with ETags.
- PostgreSQL persistence: history, metrics, logs, requests, results, reports, settings and terminal output survive restarts; **running sides are reattached** after an `api` restart.
- **Settings** page: report model, automatic report, default and suggested limits, resources per side, retention, clean-up, disk use, CLI versions.
- **Retention** after 2 days of the containers, images and staging copies ai-compare created, and nothing else; artefacts and reports are kept.
- Network isolation of agents, verified.
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

## Phase 1 checklist

The full list, all ticked, is in [PLAN.md → Phase 1](../PLAN.md#phase-1--comparing-openai-models-with-the-projects-harness).

| Item | State |
|---|---|
| Foundations: Connect for every route, event stream with connect-query, reattachment after a restart, non-root agents, optional project and folder browser | Done |
| Changes tab: solution diff without harness files, harness diff apart, files and lines changed | Done |
| Verification: tests in a fresh container, hidden tests, real Tests tab | Done |
| Report: blind reviewer, analyst, judge; early per-side stages; cost apart; self-preference warning; real report page | Done |
| History: timed recordings, Events from opencode's sessions, human wait, infrastructure errors apart, results kept as artefacts | Done |
| Settings and retention: real `/settings`, 2-day retention of ai-compare's own objects, mocks removed | Done |
| End-to-end check on Windows with real OpenAI runs | Done |
| macOS check | **Pending** |

## Known limitations

- **Tests run without network**, as the user `agent`. A suite that downloads dependencies, calls a service or needs root fails.
- **Test status comes from the exit code only:** passed, failed or error, with the output. There are no per-test counts.
- **Report quality depends on the report model.** Its findings and verdicts are a model's opinion, based on a diff cut at 120,000 characters and on facts summarised for it.
- **One run per side.** Results vary between runs; the report says so. Repetitions are phase 2.
- **Live diff only while a side runs**; once it has ended, the saved diff is shown. Between the agent stopping and the result being collected there is no diff to show.
- **Human wait is a heuristic** (input inside gaps between model requests).
- The **Preview** tab is disabled (phase 2); `/harnesses` is a preview on sample presets (phase 2).
- The runtime must contain Node.js (opencode is installed with npm).
- Starting a second comparison while one runs is only prevented by the UI.
- The event stream does not replay missed events by sequence number; a reconnecting client refetches instead.
- The terminal WebSocket does not reconnect on its own; switching tabs or reloading does.
- Very long sessions keep only their last 2 MB of terminal output for the final screen (the recording has everything).

## Where the code differs from PLAN.md

| PLAN.md says | The code does | Reason |
|---|---|---|
| Frontend embedded in the Go binary with `embed.FS` | Served from `/app/web` in the image | Simpler Docker build; same effect in practice |
| `air` for hot reload | Not set up; rebuild with `docker compose up -d --build` | Not needed yet |
| Images built with BuildKit | Classic builder (`BuilderV1`) | Works through the plain Engine API; revisit if needed |
| Agents on the `ai-compare` network | A separate `ai-compare-agents` network with netguard | Stronger isolation (no route to Postgres) |
| `pnpm gen` via `npx @bufbuild/buf` | A pinned Docker image | No host toolchain, identical output |
| Package `store/` with tables `projects`, `side_transitions`, `requests`, `price_snapshots`, `reports`, `findings` | Package `db/` with `comparisons`, `comparison_sides` and `settings`; requests, price snapshots, results and reports as JSONB; large artefacts as files | Smallest schema that persists what exists; grows with the features |
| Non-root user in side images, with `ripgrep` | Non-root user `agent`; no ripgrep | ripgrep not added yet |
| Tests in a new container from the side's image with the diff applied | Tests in a new container of an image committed from the side's container, without network | An exact copy of what the agent left, binary files included; see [Decisions D28](17-decisions.md#d28-verification-in-fresh-containers-without-network-from-a-committed-image) |
| The blind reviewer receives both diffs labelled A and B in random order | One reviewer call per side, with no model, CLI or side identity | Lets each side's review start as soon as it ends; blind without shuffling |
| Event stream: live metrics every ~2 s; a reconnecting client asks for events since the last sequence number | Running comparisons republished every second; on reconnect the client receives the live comparisons and refetches its queries | Simpler, nothing to keep on the server; see [Decisions D24](17-decisions.md#d24-an-event-stream-instead-of-polling) |
| The terminal WebSocket reconnects on its own, with retries | A new connection when the tab is shown again or the page reloads | Not needed so far |
| The report page has comparison bars for cost, tokens and duration | Tables of results and configuration, verdicts, conclusions, analyses, findings | Kept simple for phase 1 |

## Next

- **macOS check by hand:** copy from `/Users`, the "not shared" error (for example a path under `/Volumes`), terminals (typing, resizing, Ctrl+C), and a full comparison with verification and a report.

## Phase 2 (in progress, opencode only)

The checklist lives in [PLAN.md → Phase 2](../PLAN.md#phase-2).

- **Done:** Anthropic as a second provider for opencode; each side picks its own.
- Presets (`/harnesses`), "no harness", and excluding the project's harness.
- App previews per side through subdomains (`a-<id>.localhost`), relaunched from stopped containers.
- N repetitions per side with aggregates; cost versus quality chart.
- Preset adviser and preset cards.

## Phase 3

- Local models for opencode: model list from `/v1/models`, "local" cost, shared-GPU warning and optional sequential runs.
- `claude` and `codex` adapters, with the warning that they read different harness files; translation of preset cards between CLIs.
