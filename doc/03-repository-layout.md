# 3. Repository layout

A pnpm workspace with a Go module. One `.env` at the root configures everything.

```
ai-compare/
├── compose.yaml                 Entry point for `docker compose up`; includes infra/docker-compose.yml
├── .env.example                 Every setting with its default (copy to .env; .env is git-ignored)
├── package.json                 Root scripts: dev, build, lint, gen, test, infra:*, db:up, backend:*
├── pnpm-workspace.yaml          The workspace (frontend)
├── buf.yaml / buf.gen.yaml      protobuf module and code generation config
├── .gitattributes               Forces LF line endings in the repo
├── PLAN.md                      Original product and technical plan
├── README.md                    Quick start
├── AGENTS.md                    Instructions for AI coding agents working on this repo
├── CLAUDE.md                    Imports AGENTS.md for Claude Code
├── doc/                         This documentation
├── mockups/                     Design mockups of every page
│
├── proto/aicompare/v1/          API contract (protobuf)
│   ├── catalog.proto            CatalogService
│   ├── project.proto            ProjectService (inspection, folder browser)
│   ├── preset.proto             PresetService (harness presets)
│   ├── comparison.proto         ComparisonService (start, follow, finish/cancel, delete, logs, diff, tests, timeline)
│   ├── events.proto             EventService (the live event stream)
│   ├── report.proto             ReportService
│   └── settings.proto           SettingsService (settings, clean-up)
│
├── infra/
│   ├── docker-compose.yml       Services api, postgres, gen and test; networks; volumes
│   └── docker/
│       ├── api.Dockerfile       Builds the frontend (Node) and the backend (Go) into one Alpine image
│       └── gen.Dockerfile       Pinned code generators: buf, protoc-gen-go, protoc-gen-connect-go, protoc-gen-es, sqlc
│
├── backend/                     Go module `ai-compare/backend`
│   ├── go.mod / go.sum
│   ├── sqlc.yaml                sqlc configuration
│   ├── cmd/
│   │   ├── server/              The api binary
│   │   │   ├── main.go          Wiring, health check, Connect mount, SPA handler, two HTTP servers
│   │   │   ├── comparisons.go   Plain HTTP routes: terminal WebSocket, download, recording
│   │   │   ├── spike_proxy.go   Phase 0 routes to create and inspect proxy sessions by hand
│   │   │   └── zip.go           Converts a side's tar stream (saved workspace.tar or the running container) into a zip
│   │   └── spike/               Phase 0 command-line checks (prepare, proxy-check), shipped as /app/spike
│   └── internal/
│       ├── config/              Environment and .env loading
│       ├── catalog/             models.dev client, parser, cache
│       ├── proxy/               Inference proxy (proxy.go) and usage parsing (usage.go)
│       ├── workspace/           Docker work: copy, inspect, build (workspace.go); commit, collect, tests, live diff, clean-up, disk use (result.go)
│       │   ├── collect-result.sh    Collects a side's result into the artefacts volume (embedded)
│       │   └── copier/          The helper image: Dockerfile, copy-project.sh, inspect-project.sh, list-folders.sh, copy-paths.sh (embedded)
│       ├── comparison/          Orchestrator: types and actions (comparison.go), preparing and following a side (run.go),
│       │                        verification (verify.go), persistence and restarts (store.go), event bus (events.go),
│       │                        CLI sessions (timeline.go), human wait (human.go), retention and deletion (retention.go),
│       │                        opencode adapter (agent.go); tests with testdata/session.json
│       ├── report/              Report: service and per-side stages (report.go), prompts and schemas (stages.go), proxy caller (caller.go)
│       ├── settings/            Editable settings, saved in Postgres
│       ├── presets/             Harness presets on disk (harnesses/<slug>/)
│       ├── terminal/            Attach and WebSocket bridge (terminal.go), per-side hub (hub.go), asciicast recorder (cast.go)
│       ├── netguard/            Blocks the agent network from the app port
│       ├── rpc/                 Connect service implementations (catalogue, projects, comparisons, events, reports, settings)
│       ├── db/                  postgres.go (open + migrate), migrations/ (00001_comparisons, 00002_phase1), queries/ and sqlc output
│       └── gen/                 Generated protobuf and Connect code (committed, do not edit)
│
└── frontend/                    Vite + React + TypeScript
    ├── package.json
    ├── vite.config.ts           Reads the root .env, proxies /api to the backend in dev
    ├── components.json          shadcn configuration
    ├── index.html
    └── src/
        ├── main.tsx             React root, QueryClient, connect-query TransportProvider, router
        ├── router.tsx           Code-based TanStack Router routes
        ├── index.css            Tailwind v4 and the "Consola" design tokens
        ├── api/
        │   ├── types.ts         Plain types the UI works with
        │   ├── transport.ts     Connect transport and one client per service
        │   ├── convert.ts       Protobuf messages to UI types and back
        │   ├── queries.ts       connect-query hooks and mutations (nothing polls)
        │   ├── events.ts        The event stream hook: EventService.Watch into the query cache
        │   └── http.ts          The WebSocket terminal source; download and recording URLs
        ├── gen/                 Generated TypeScript from proto/ (committed, do not edit)
        ├── lib/                 Formatting (en-GB), catalogue helpers, cn()
        ├── components/
        │   ├── app/AppShell.tsx Navigation, layout and the status bar (event stream state, active run)
        │   ├── compare/         SideForm (one side's settings), FolderBrowser, artifacts (the per-side tabs)
        │   ├── terminal/        TerminalView (xterm.js), RecordingPlayer (timed replay of asciicast recordings)
        │   ├── common/          Small shared pieces (Metric, Dot, ErrorNote…)
        │   └── ui/              shadcn components
        └── pages/               One file per page; harnesses/samples.ts holds the sample presets of the phase 2 preview
```

## Conventions

- **Language.** All code, comments, UI text and documentation are in **British English** (colour, behaviour, catalogue, initialise). Nothing in the repository is in Spanish.
- **Generated code is committed** (`backend/internal/gen`, `backend/internal/db/*.go` except `postgres.go` and `fs.go`, `frontend/src/gen`). Regenerate with `pnpm gen`; never edit it by hand.
- **Line endings.** LF everywhere (`.gitattributes`). Shell scripts embedded in Go are normalised to LF before use, because a CRLF script fails in Linux.
- **One `.env`.** Compose, the Go backend (when run on the host) and Vite all read the root `.env`. Only `VITE_*` variables reach the browser.
- **Labels on Docker objects.** Everything ai-compare creates carries `ai-compare.*` labels (`comparison`, `side`, `role`), used for retention, deletion and finding leftovers. Roles: `copier`, `copy-project`, `inspect-project`, `list-folders`, `side`, `agent`, `result`, `collect`, `test`, `test-hidden`.
- **Commits** never carry AI attribution trailers.

## Where to start reading the code

1. `backend/cmd/server/main.go` — how everything is wired.
2. `backend/internal/comparison/comparison.go` — `Start`, `Finish`/`Cancel` and the views; then `run.go` (`run`, `runSide`, `follow`) and `verify.go` (`verify`, `collect`) are the whole lifecycle.
3. `backend/internal/proxy/proxy.go` — `ServeHTTP` is the proxy in one function.
4. `backend/internal/rpc/` — how each Connect method maps onto the services.
5. `frontend/src/api/events.ts` and `queries.ts` — how the UI keeps its data current.
6. `frontend/src/pages/RunPage.tsx` — the live run screen.
