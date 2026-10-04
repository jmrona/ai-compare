# 17. Decisions

Each entry: the decision, the context, the alternatives and why. Newest considerations do not erase older ones; when a decision changes, a new entry says so.

---

### D1. A local web app driven by Docker

**Decision.** ai-compare is a web UI served from a local container, not a desktop app or a CLI.
**Why.** Two live terminals side by side, tabs, diffs and reports need a rich UI; a browser gives that on every platform. Docker gives isolation for agents and the "one command" install.
**Alternatives.** Electron or Tauri app (packaging per platform, still needs Docker for isolation); a pure CLI (no side-by-side terminals and reports).

### D2. Docker-out-of-Docker

**Decision.** `api` mounts the host's Docker socket and creates sibling containers.
**Why.** Agents need real containers; Docker-in-Docker would need a privileged container, run slower and keep a separate image cache. Bind mounts resolved by the host daemon are what let `api` read `C:\…` or `/Users/…`.
**Accepted risk.** Whoever controls the socket controls the host. Only `api` mounts it, and its port is bound to localhost.

### D3. Copy the project once; mount the top-level folder read-only

**Decision.** A short-lived helper container mounts the top-level folder (`C:\`, `/Users`, `/home`) read-only and copies the project into a volume. Both sides build from that copy.
**Why.** The original is never touched. Agents never work on slow shared folders. Mounting the project folder directly was rejected after finding that **Docker Desktop on Windows creates missing bind sources**: a typo would create an empty folder on the user's disk. Mounting a folder that always exists and checking inside gives a clear error.
**Detail.** Git projects are copied as `git ls-files --cached --others --exclude-standard` (the working tree, uncommitted changes included, ignored files excluded). `.env` files are never copied; templates are.

### D4. A Git baseline per side

**Decision.** Each side image commits the copied project as `baseline` before the agent runs, with `core.autocrlf=false`; the CLI configuration is added after it.
**Why.** The Changes tab can show exactly what the agent did, without setup output or line-ending noise and without ai-compare's own files.

### D5. One image per side, layered for caching

**Decision.** Runtime → Git → CLI → project → setup → baseline → CLI config.
**Why.** Everything that does not depend on the project is cached across comparisons; only the first build pays for installing the CLI.

### D6. Our own inference proxy in Go; no LiteLLM

**Decision.** A reverse proxy inside `api` on its own port, with per-side tokens.
**Why.** It measures every request in every mode, keeps real keys out of containers and enforces budgets. LiteLLM's main value is translating between APIs, which is unnecessary because each CLI uses its provider's native API; it would add a Python service and a translation layer that could change behaviour between sides.
**Details.** Requests and responses pass unchanged; usage is read on the side. Limits answer **403**, not 429, because CLIs retry 429s.

### D7. Each CLI with its native providers only

**Decision.** `claude` only with Anthropic, `codex` only with OpenAI, `opencode` with both and with local models. Phase 1 uses only opencode with OpenAI.
**Why.** Translating APIs would make the comparison about the translator. opencode is the only CLI that covers all providers, so phase 2 only adds a provider (Anthropic), not adapters; the other CLIs and local models are phase 3.

### D8. models.dev as the only price source, with snapshots

**Decision.** No price table. Prices come from models.dev, cached on disk; each side stores a snapshot of the price it ran with.
**Why.** Nothing to maintain by hand; past comparisons keep their prices. models.dev has no per-provider endpoint, so the whole file (~380 KB compressed) is fetched with ETags and filtered.
**Also.** The catalogue provides release dates (newest models first in dropdowns) and the efforts each model accepts.

### D9. No Temporal; an in-process orchestrator with state in Postgres

**Decision.** A Go orchestrator with a state machine per side, timeouts via `context`, and state saved to Postgres.
**Why.** Temporal adds a server, workers and a deterministic-workflow programming model. The longest part of a comparison is a live interactive container, which Temporal cannot resume after a crash anyway; reconciling with Docker is needed regardless. The load is one user and one or two comparisons. Temporal could pay off later for queued repetitions or long reports.

### D10. Protobuf + Connect for the API; WebSocket for terminals

**Decision.** `connect-go` v2, `connect-es`, `protobuf-es`, `buf`, and later `connect-query`.
**Requirements.** One contract for Go and TypeScript; works in the browser without extra pieces; server-to-browser streaming for live events; TanStack Query integration; an active project.
**Alternatives evaluated** (October 2026): gRPC-Web (needs Envoy or an adapter), Twirp (inactive since 2022, no streaming), OpenAPI contract-first (`oapi-codegen`/`ogen` + `orval`/Hey API: close, but streaming stays outside the contract), OpenAPI code-first (Huma), TypeSpec (another layer), GraphQL (excessive), tRPC (TypeScript backends only).
**Why Connect.** The only option meeting all five without extra pieces; maintained by Buf, in the CNCF; also speaks JSON, so `curl` works.
**Notes.** connect-go v2 was a release candidate when adopted; v1.21 is the stable fallback with a migration tool. Terminals stay on WebSocket because browsers cannot do Connect bidirectional streaming. Migration is incremental: `CatalogService` first, JSON routes move one service at a time. (Completed in phase 1, see D23.)

### D11. Generated code is committed; generators run in Docker

**Decision.** `backend/internal/gen`, `frontend/src/gen` and sqlc output are in Git; `pnpm gen` runs pinned generators in a container.
**Why.** Building never needs buf or sqlc, and generation gives identical output on every platform with nothing installed. (The plan first proposed `npx @bufbuild/buf` with local plugins, which would have needed Go on the host for the Go plugins.)

### D12. PostgreSQL with pgx, goose and sqlc; JSONB for evolving shapes

**Decision.** Migrations embedded and applied at startup; typed queries generated from SQL; side details stored as JSONB.
**Why.** No migration step for the user; SQL stays reviewable; JSONB keeps the schema stable while API shapes are still changing. The app degrades to in-memory if the database is unavailable.

### D13. Agents on their own network; the app port refuses them

**Decision.** Network `ai-compare-agents` with only `api` on it; netguard returns 403 on the app port for that subnet; agents keep internet access.
**Why.** Agents must reach the proxy but must not reach the control plane (which can create containers) or the database. They need the internet to install packages.

### D14. Terminals owned by a server-side hub

**Decision.** One hub per side buffers output (2 MB), fans it out to any number of viewers, accepts input from any of them, remembers the size, and is saved when the side ends.
**Why.** Closing a tab must not lose anything; several tabs must show the same thing; the TUI must start at the browser's size (it used to draw at 80×24 until a tab switch); the history needs the output.

### D15. Autonomous by default

**Decision.** Both sides default to autonomous mode (`opencode run --auto`).
**Why.** Autonomous sides finish on their own; interactive sides wait for the user and their time includes the user's. Interactive remains available for follow-up conversations.

### D16. Equal resources per side

**Decision.** 2 CPUs and 4 GB of memory per side; both sides start together.
**Why.** Fairness. Running sides one after the other would be slower and expose them to different provider load. The GPU of local models cannot be split; that case gets a warning and an optional sequential mode in phase 2.

### D17. Optional limits

**Decision.** Timeout, token and cost limits are all optional per side.
**Why.** Sometimes the point is to see how far a model goes; sometimes a budget matters. Without limits the proxy only measures.

### D18. Results are throwaway; downloads by zip

**Decision.** Nothing is pushed anywhere; each side's `/workspace` can be downloaded as `<model>-<id>-<side>.zip` (without `.git` and `node_modules`).
**Why.** The tool compares; it does not ship code. A zip named after the model makes it obvious which result is which.

### D19. Preparation time recorded but not counted

**Decision.** Copy, build and start times are recorded per phase and shown, but the agent's time starts when its container runs.
**Why.** Image builds depend on cache state, not on the model; counting them would make the first side to build look slower.

### D20. A hybrid mock client while the backend grows

**Decision.** The UI was built first on in-memory sample data; it now calls the real backend for everything implemented and samples for the rest, deciding per call (real ids start with `r`).
**Why.** The whole product could be designed and reviewed before the backend existed, and each piece moves over without breaking the others.
**Superseded** by D23: the mock client was removed in phase 1.

### D40. Repetitions as a series of whole comparisons

**Decision.** N repetitions run as N complete comparisons that share a series id, one after another, all from attempt 1's project copy and preset snapshots, instead of N attempts inside one comparison.
**Why.** Every attempt reuses everything a comparison already has (containers, terminals, verification, reports, history, downloads) without changing the side model; running them in sequence keeps the load and the provider's rate limits as for one comparison. The series page adds what repetitions are for: aggregates and the cost versus quality chart.
**Trade-off.** A series of N takes N times as long as one comparison; parallel attempts can come later if needed.

### D45. ai-compare's CLI settings live in the managed configuration

**Decision.** opencode's settings that ai-compare must control (model, small model, build agent, provider base URLs) go to `/etc/opencode/opencode.json`, owned by root; the agent's global config folder belongs to the preset. Side tokens work for every provider, and the proxy prices each request with the model it names. Plugin dependencies are installed while the side image is built.
**Why.** opencode merges its config files and the project's win over the global one, so a harness `opencode.json` could change a side's model or bypass the proxy unnoticed; the managed file is the one layer that wins over all others. Leaving the global folder to presets lets them carry plugins and agents. Routing plugins send subagents to other models, so per-request pricing keeps costs right. Agents have no network, and opencode waits forever for plugin dependencies it cannot download.

### D44. Autonomous sides are told not to ask

**Decision.** In autonomous mode the prompt ends with a fixed note: nobody will answer, so decide, state the assumptions and finish. Interactive sides get the prompt as typed.
**Why.** An autonomous run that ends on a question produces no changes and wastes the comparison; the note makes both sides finish the task. It is the same text for every autonomous side, so it does not bias one side against the other, and the report still judges against the user's prompt.

### D43. Retention the user can tune down to nothing

**Decision.** Retention days go from 0 to 365, and the user selects what retention removes: containers, images, project copies and, optionally, artefacts. 0 removes the selection from every comparison that is not running, and the page warns about it; selecting artefacts also shows a warning.
**Why.** Disk is the user's to manage: some want everything gone as soon as a comparison ends, others want the downloads forever. The defaults keep the earlier behaviour (artefacts kept), and reports and the history are never removed so a comparison can always be read.

### D42. A Harness tab instead of a harness diff

**Decision.** Each side has a **Harness** tab next to Changes that shows the harness files it ran with, read-only. Their diff appears there only when the agent changed them. The project's own harness files are copied into the side's artefacts (`<id>/<side>/harness/`) when the project is copied, as preset snapshots already were.
**Why.** Agents are not meant to change the harness, so a diff of it was almost always empty, while what matters when reading a comparison is which instructions each side had. Keeping them in the artefacts means they survive retention, which removes the staging copy.

### D41. A harness adviser in the report, and cards without translation

**Decision.** When the two sides ran with different harnesses (kind, preset or preset hash), the report adds a stage that reads both harnesses' text with each side's facts and the judgement, and lists the differences that may have mattered and what to try. Preset cards write sections of `AGENTS.md` only.
**Why.** Comparing presets is what phase 2 adds; the adviser turns a comparison into a next step. Its answers are framed as inferences because one run per side cannot prove cause (repetitions make them firmer). Cards stay in AGENTS.md because opencode is the only CLI until phase 3, where translating them to each CLI's files belongs.

### D21. British English everywhere in the code

**Decision.** Code, comments, UI and documentation in British English, the plan included (it was first written in Spanish and translated).
**Why.** The owner's preference, applied consistently so contributors and AI agents follow one convention.

### D22. macOS first, Windows and Linux supported

**Decision.** Design for macOS; verify on Windows (where development happened) and Linux.
**Why.** The owner's main machine is a Mac. Platform differences are handled in one place each (see [Platforms](16-platforms.md)).

---

The entries below were taken while implementing phase 1 (October 2026).

### D23. All comparison routes on Connect; mocks removed

**Decision.** Every JSON route moved to Connect (`ComparisonService`, `EventService`, `ReportService`, `SettingsService`); only the terminal WebSocket, the download and the recording stay plain HTTP. The hybrid mock client and `VITE_USE_MOCKS` were removed.
**Why.** This completes the incremental migration of D10 and ends D20: with every page on the backend, sample data could only hide gaps. Downloads and recordings are files the browser fetches by URL, and a terminal needs bidirectional streaming, so they stay outside Connect. Only `/harnesses` keeps sample presets, clearly marked as a phase 2 preview.

### D24. An event stream instead of polling

**Decision.** One `EventService.Watch` server stream per browser pushes every change of a comparison (coalesced every 250 ms, running comparisons republished every second, deletions). The frontend writes each comparison into the connect-query cache and invalidates the queries that depend on what changed; nothing polls. On reconnect the server sends the live comparisons again and the client refetches all its queries.
**Why.** Polling every second for every open view wasted requests and still lagged. Sending whole comparisons (small) instead of fine-grained events keeps the client simple: the cache entry is replaced, not patched. Replaying missed events by sequence number (the plan's idea) was not needed: refetching on reconnect gives the same result with no history kept on the server.
**Alternatives.** Keep polling; WebSocket or SSE outside the contract (loses the typed contract D10 chose Connect for).

### D25. A first message and heartbeats on Watch

**Decision.** `Watch` sends an empty message as soon as it opens, and another every 20 s.
**Why.** In connect-go v2 RC1, `SendHeaders` does not flush on the server, so a client could not tell the stream was open until the first change, which may never come when nothing is running. The heartbeat keeps idle connections from being closed along the way. An empty message is cheap and needs no new message type.

### D26. connect-query for every query

**Decision.** Queries use `@connectrpc/connect-query` with keys from the generated method descriptors; mutations call the clients through TanStack Query.
**Why.** Keys derived from the contract let the event stream update or invalidate exactly the right cache entries, and remove hand-written fetch functions and key tables.

### D27. Agents run as a non-root user

**Decision.** Side images create the user `agent` (home `/home/agent`), give it `/workspace` after the baseline commit, copy the CLI configuration with `--chown` and switch to it with `USER agent`. The setup command still runs as root at build time.
**Why.** Defence in depth: the container stays the security boundary (D13), but an agent no longer has root inside it. Setup often needs root (system packages) and runs before any model output exists.
**Consequence.** ai-compare's own collection step runs as root in a separate container and must tell Git (and opencode) that a repository owned by `agent` is safe (`safe.directory=*`).

### D28. Verification in fresh containers, without network, from a committed image

**Decision.** When the agent stops, its container is committed to `ai-compare/result:<id>-<side>`. The result is collected, and the profile's tests run, in short-lived containers of that image with `NetworkMode: none`: the tests as `agent`, with a 10-minute timeout; hidden tests in a second run that copies them in first. Test status comes from the exit code.
**Why.** Tests must see exactly what the agent left, but nothing it left running, and never in the container the agent worked in. Hidden tests never enter an image the agent ran in. Without network a test suite cannot reach the proxy, leak the project or depend on a live service, and both sides are tested under the same conditions. Committing an image is the cheapest exact copy of the container's file system, and it outlives the container.
**Alternatives.** Applying the diff to a fresh side image (the plan's wording: needs a diff that applies cleanly, binary files included); running the tests in the agent's container (affected by its processes and state).
**Accepted cost.** Suites that need network or root fail; per-test counts would need a parser per test runner, so only pass or fail is reported.

### D29. Results kept as artefacts

**Decision.** Each side's files (`workspace.tar`), diffs, CLI session, test output and terminal recording are written to the `ai-compare_artifacts` volume under `<id>/<side>/`, and the database only says whether they exist. Downloads, diffs, tests, events and replays of ended sides read them from there.
**Why.** Retention removes containers and images after two days; the history must stay complete and downloadable after that. Files of this size do not belong in Postgres rows (D12).
**Changes D18.** The zip download no longer reads the stopped container once the side has ended.

### D30. Reattaching to running sides after a restart

**Decision.** A running side's proxy token is saved with the side, and the side is saved every 10 s. On startup, sides whose container still runs are reattached: same token (`proxy.RestoreSession` with the saved snapshot), screen rebuilt from `docker logs`, recording completed with the output since the last save. Sides whose agent ended meanwhile are verified; sides being prepared end as infrastructure errors.
**Why.** An `api` restart (an update, a crash) should not throw away a long interactive session. Docker keeps the container and its TTY log, so only `api`'s in-memory state had to be made recoverable, as D9 anticipated by keeping state in Postgres and reconciling with Docker.

### D31. Agent and infrastructure failures told apart

**Decision.** A side that ends in `error` carries a failure kind: `agent` (the CLI exited with a non-zero code) or `infrastructure` (copy, build, start, a lost container, a restart during preparation).
**Why.** A comparison should only count against a model what the model did. The report and the UI can say "the setup broke" instead of blaming the agent.

### D32. Human wait as a heuristic

**Decision.** For interactive sides, the hub records when the user typed (at most once per second). Human wait is the sum of the gaps between merged proxy request intervals that contain user input; agent time excludes it.
**Why.** Interactive sides would otherwise count the user's reading and thinking as agent time. The proxy already knows when the model was working and the terminal knows when the user acted, so no CLI cooperation is needed.
**Limit.** It cannot tell a user reading the screen from an agent working locally without calling the model; gaps without input stay agent time.

### D33. The report in stages: per side early, then a judge

**Decision.** Three stages: a blind reviewer per side (task, test line, files and diff; no model, CLI or side), an analyst per side (facts, events, log warnings, test output) and a comparative judge (both sides' facts, findings and analyses; verdicts Cheaper, Faster, Fewer problems, Overall and Tests, each A, B or tie). With automatic reports on, a side's reviewer and analyst run as soon as it ends; otherwise everything runs when the user asks.
**Why.** Per-side stages are independent, so running them early leaves only the judge when the second side ends. A reviewer that sees one side at a time and no identity is blind more simply than one given shuffled, labelled diffs. Strict JSON schema outputs make the report parseable without fragile text parsing. Warnings are always shown: one run per side; the report model being one of the compared models (self-preference); an interactive side.
**Automatic reports are off by default**, because a report costs money and the user may not want one for every run.

### D34. Reports through the inference proxy

**Decision.** The report service calls the OpenAI Responses API at `http://127.0.0.1:<PROXY_PORT>/openai/v1/responses` with a proxy session per stage group, not the provider directly.
**Why.** The proxy already prices requests with models.dev and keeps the key in one place, so the report's cost is measured exactly as the comparison's, and apart from it.

### D35. `gpt-6-luna` as the default report model, configurable

**Decision.** The report model is a setting, `gpt-6-luna` by default, chosen from the usable OpenAI models; reasoning effort is `low` when the model supports it.
**Why.** A capable, recent model gives useful reviews; low effort keeps a report cheap. It is configurable because the best choice changes quickly, and because it should differ from the compared models where possible.

### D36. Retention of labelled objects only, after 2 days

**Decision.** Every hour, comparisons whose sides all ended more than `retentionDays` ago (2 by default, 1 to 365) lose their containers (by the label `ai-compare.comparison=<id>`), their side and result images (by exact name) and their staging folder. Artefacts, reports and the history stay. **Clean up now** applies the same rule; deleting a comparison also removes its artefacts and rows.
**Why.** Side and result images add up quickly. ai-compare runs on the user's own Docker, so it never prunes or matches by wildcard: it removes only what it can prove it created for that comparison. Two days leave time to inspect a container by hand.

### D37. Settings in the database; resources configurable

**Decision.** Report model, automatic report, default limits, CPUs and memory per side and retention are edited in `/settings` and saved as one JSON row in Postgres. Facts the app cannot change (keys present, local model URL, CLI versions, disk use, suggested limits) are shown read-only.
**Why.** These are preferences, not deployment configuration, and should not need editing `.env` and restarting. D16 still holds: both sides always get the same resources (2 CPUs and 4 GB by default).

### D38. Suggested limits

**Decision.** Limits stay optional and off by default (D17). When the user switches one on, it is pre-filled with a suggestion: 30 min, 2,000k tokens, $2 per side.
**Why.** A sensible starting value saves typing and avoids accidental tiny or huge budgets, without imposing limits on runs where the point is to see how far a model goes.

### D39. No UI session token for now

**Decision.** The app has no login or session token.
**Why.** It is local, single-user and bound to `127.0.0.1`, and the terminal WebSocket only accepts same-origin connections. A Jupyter-style token will be added if the app is ever exposed beyond the local machine.
