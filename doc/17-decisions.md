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
**Why.** Translating APIs would make the comparison about the translator. opencode is the only CLI that covers all providers, so phase 2 only adds providers, not adapters.

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
**Notes.** connect-go v2 was a release candidate when adopted; v1.21 is the stable fallback with a migration tool. Terminals stay on WebSocket because browsers cannot do Connect bidirectional streaming. Migration is incremental: `CatalogService` first, JSON routes move one service at a time.

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

### D21. British English everywhere in the code

**Decision.** Code, comments, UI and documentation in British English; the original plan stays in Spanish.
**Why.** The owner's preference, applied consistently so contributors and AI agents follow one convention.

### D22. macOS first, Windows and Linux supported

**Decision.** Design for macOS; verify on Windows (where development happened) and Linux.
**Why.** The owner's main machine is a Mac. Platform differences are handled in one place each (see [Platforms](16-platforms.md)).
