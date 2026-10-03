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
│   └── project.proto            ProjectService (inspection, folder browser)
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
│   │   │   ├── comparisons.go   JSON routes: comparisons, finish/cancel, logs, download, terminal
│   │   │   ├── spike_proxy.go   Phase 0 routes to create and inspect proxy sessions by hand
│   │   │   └── zip.go           Converts the container's tar stream into a zip
│   │   └── spike/               Phase 0 command-line checks (prepare, proxy-check), shipped as /app/spike
│   └── internal/
│       ├── config/              Environment and .env loading
│       ├── catalog/             models.dev client, parser, cache
│       ├── proxy/               Inference proxy (proxy.go) and usage parsing (usage.go)
│       ├── workspace/           Docker work: copy, inspect, build
│       │   └── copier/          The helper image: Dockerfile, copy-project.sh, inspect-project.sh, list-folders.sh (embedded)
│       ├── comparison/          Orchestrator (comparison.go), opencode adapter (agent.go), persistence (store.go)
│       ├── terminal/            Attach and WebSocket bridge (terminal.go), per-side hub (hub.go)
│       ├── netguard/            Blocks the agent network from the app port
│       ├── rpc/                 Connect service implementations (catalogue, projects)
│       ├── db/                  postgres.go (open + migrate), migrations/, queries/ and sqlc output
│       └── gen/                 Generated protobuf and Connect code (committed, do not edit)
│
└── frontend/                    Vite + React + TypeScript
    ├── package.json
    ├── vite.config.ts           Reads the root .env, proxies /api to the backend in dev
    ├── components.json          shadcn configuration
    ├── index.html
    └── src/
        ├── main.tsx             React root, QueryClient, router
        ├── router.tsx           Code-based TanStack Router routes
        ├── index.css            Tailwind v4 and the "Consola" design tokens
        ├── api/
        │   ├── types.ts         Types shared by the UI (mirror the backend JSON)
        │   ├── client.ts        ApiClient interface and the hybrid client (real + mocks)
        │   ├── http.ts          Real HTTP client and the WebSocket terminal source
        │   ├── rpc.ts           Connect clients and mapping from protobuf messages
        │   ├── queries.ts       TanStack Query hooks and polling rules
        │   └── mock/            In-memory client and fixtures (sample data)
        ├── gen/                 Generated TypeScript from proto/ (committed, do not edit)
        ├── lib/                 Formatting (en-GB), catalogue helpers, cn()
        ├── components/
        │   ├── app/AppShell.tsx Navigation bar and layout
        │   ├── compare/         SideForm (one side's settings), FolderBrowser, artifacts (the per-side tabs)
        │   ├── terminal/        TerminalView (xterm.js)
        │   ├── common/          Small shared pieces (Metric, Dot, ErrorNote…)
        │   └── ui/              shadcn components
        └── pages/               One file per page
```

## Conventions

- **Language.** All code, comments, UI text and documentation are in **British English** (colour, behaviour, catalogue, initialise). Nothing in the repository is in Spanish.
- **Generated code is committed** (`backend/internal/gen`, `backend/internal/db/*.go` except `postgres.go` and `fs.go`, `frontend/src/gen`). Regenerate with `pnpm gen`; never edit it by hand.
- **Line endings.** LF everywhere (`.gitattributes`). Shell scripts embedded in Go are normalised to LF before use, because a CRLF script fails in Linux.
- **One `.env`.** Compose, the Go backend (when run on the host) and Vite all read the root `.env`. Only `VITE_*` variables reach the browser.
- **Labels on Docker objects.** Everything ai-compare creates carries `ai-compare.*` labels (`comparison`, `side`, `role`), used for cleanup and for finding leftovers.
- **Commits** never carry AI attribution trailers.

## Where to start reading the code

1. `backend/cmd/server/main.go` — how everything is wired.
2. `backend/internal/comparison/comparison.go` — `Start`, `run` and `runSide` are the whole lifecycle.
3. `backend/internal/proxy/proxy.go` — `ServeHTTP` is the proxy in one function.
4. `frontend/src/api/client.ts` — which calls are real and which are samples.
5. `frontend/src/pages/RunPage.tsx` — the live run screen.
