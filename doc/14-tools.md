# 14. Tools and libraries

Every tool in the stack, what it does in ai-compare and why it was picked. Versions are the ones pinned or installed at the time of writing; check `go.mod`, `frontend/package.json` and the Dockerfiles for the current ones.

## Runtime and infrastructure

| Tool | Role here | Why |
|---|---|---|
| **Docker Engine / Docker Desktop** | Runs everything: the stack, the copy helper, one container per side | Isolation for agents that run arbitrary commands, identical environments for both sides, and a single requirement on the host |
| **Docker Compose** | Declares `api`, `postgres`, networks and volumes; `docker compose up -d` is the whole install. `compose.yaml` at the root includes `infra/docker-compose.yml`. Profile `tools` holds `gen` and `test` | One command on every platform; no host toolchain |
| **Docker-out-of-Docker** (the host socket mounted into `api`) | `api` creates sibling containers and builds images on the host daemon | Faster and simpler than Docker-in-Docker, shares the image cache, needs no privileged container. See [Decisions](17-decisions.md) |
| **Docker classic builder** (`BuilderV1`) | Builds the copy helper and side images from tar contexts sent by `api` | Works through the plain Engine API from Go without BuildKit session plumbing |
| **PostgreSQL 17** | Comparisons, sides, reports and settings | Reliable, JSONB for evolving shapes, well supported by pgx/sqlc/goose |
| **Alpine Linux 3.22** | Base of the `api` image and the copy helper | Small images |
| **node:22-bookworm-slim** | Default runtime for side images | opencode is installed with npm; Debian (glibc) works with more prebuilt packages than Alpine |

## Backend (Go 1.26)

| Library | Role | Why |
|---|---|---|
| **Go standard library** `net/http` | Both HTTP servers, the router (method and wildcard patterns since Go 1.22), `httputil.ReverseProxy` for the proxy | No framework needed; the reverse proxy handles streaming correctly with `FlushInterval: -1` |
| `log/slog` | Structured JSON logs | Standard, no dependency |
| `embed` | Ships the copy helper's files, `collect-result.sh` and the SQL migrations inside the binary | One binary, no files to mount |
| **`github.com/moby/moby/client`** (v0.6) and **`/api`** (v1.56) | Docker Engine API: build, commit, create, attach, start, resize, exec, wait, stop, remove, logs, copy from container, list images and containers, inspect networks | The official Go client of Docker, which is itself written in Go |
| **`github.com/coder/websocket`** | Terminal WebSockets | Small, maintained, context-aware, same-origin check by default |
| **asciicast v2** (format) | Timed terminal recordings, written by `terminal.Recorder` and replayed by `RecordingPlayer` | The asciinema format: one JSON line per output or resize event, easy to write as a stream and to replay; also playable with `asciinema play` |
| **`github.com/jackc/pgx/v5`** | PostgreSQL driver and connection pool | The standard high-performance Postgres driver for Go; sqlc supports it natively |
| **`github.com/pressly/goose/v3`** | Applies embedded SQL migrations at startup | Plain SQL files, embeddable, no separate migration step |
| **`connectrpc.com/connect/v2`** (release candidate) | Serves the Connect services (`connect.Server` + `connecthttp.Mount`), including the `EventService.Watch` server stream, and the Go client used by the spike | See [API and contracts](10-api-and-contracts.md); v2 chosen to avoid a migration later |
| **`google.golang.org/protobuf`** | Protobuf runtime for generated messages | Required by Connect |

## Frontend

| Library | Role | Why |
|---|---|---|
| **React 19** | UI | Ecosystem, shadcn, xterm.js wrappers |
| **TypeScript 6** | Types across the app | Safety, generated contract types |
| **Vite 8** | Dev server with `/api` proxy, production build | Fast, simple config, reads the root `.env` via `envDir` |
| **Tailwind CSS v4** (`@tailwindcss/vite`) | Styling with design tokens in CSS | Utility classes and tokens in one place; v4 needs no config file |
| **shadcn/ui** (preset `lyra`) on **Radix UI** | Accessible components (tabs, selects, dialogs, tooltips…) copied into the repo | Owned code instead of a dependency; accessible primitives |
| `lucide-react` | Icons | Default icon set of shadcn |
| `class-variance-authority`, `cn` | Component variants and class merging | shadcn conventions |
| **TanStack Router** | Routing, declared in code | Type-safe routes and params |
| **TanStack Query** | Server state and caching; mutations | Standard for server state in React; the event stream writes into its cache, so nothing polls |
| **`@connectrpc/connect-query`** | Query hooks from the generated method descriptors (`useQuery(Service.method.x, input)`), `TransportProvider`, cache keys | Typed queries with keys the event stream can target exactly |
| **`@connectrpc/connect`**, **`@connectrpc/connect-web`**, **`@bufbuild/protobuf`** | Connect transport and clients (including the server stream), protobuf runtime | Generated, typed calls to the backend from the same contract |
| **xterm.js 6** (`@xterm/xterm`) and `@xterm/addon-fit` | Terminal emulator in the browser, live and for replaying recordings | The de facto browser terminal (used by VS Code) |
| `@fontsource/ibm-plex-sans`, `@fontsource/ibm-plex-mono` | Self-hosted fonts | No external font requests from a local app |
| **oxlint** | Linting | Very fast, sensible defaults |

## Code generation and tooling

| Tool | Role | Why |
|---|---|---|
| **pnpm 10** (workspace) | Package manager and root scripts | Fast, strict, workspace support; scripts are the cross-platform entry points (no bash required) |
| **buf** | Lints the `.proto` files and runs the generators | Modern protobuf toolchain from the authors of Connect |
| `protoc-gen-go`, `protoc-gen-connect-go`, `protoc-gen-es` | Generate Go messages, Go Connect code and TypeScript | Official plugins |
| **sqlc** | Generates typed Go from SQL queries, checked against the migrations | SQL stays SQL; no ORM |
| `infra/docker/gen.Dockerfile` | One image with all generators at pinned versions; `pnpm gen` runs it | Same output on macOS, Windows and Linux, nothing to install |

## External services

| Service | Role |
|---|---|
| **models.dev** | Catalogue of models and prices (see [Models and pricing](09-models-and-pricing.md)) |
| **OpenAI API** | Phase 1 provider for the agents and the report model, reached only through the proxy |
| **npm registry** | The latest published opencode version, shown in Settings next to the pinned one |
| **Anthropic API** | Phase 2 provider (wired in the proxy, not offered in the UI yet) |
| **Local OpenAI-compatible servers** (Ollama, LM Studio, llama.cpp, vLLM) | Phase 2, reached at `host.docker.internal` |

## Agent CLIs

| CLI | Installed as | Phase |
|---|---|---|
| **opencode** `1.18.34` | `npm install -g opencode-ai@1.18.34` in the side image | 1 |
| **codex** | to be decided (binary or npm) | 2 |
| **claude** (Claude Code) | to be decided | 2 |

## Considered and not used

| Tool | Why not |
|---|---|
| Temporal | Too much infrastructure for one user; it cannot resume a live terminal anyway. See [Decisions](17-decisions.md) |
| LiteLLM | Its main value is translating between APIs, which is not needed when each CLI speaks its provider's native API |
| gRPC-Web, Twirp, OpenAPI generators, TypeSpec, GraphQL, tRPC | Compared with Connect; see [Decisions](17-decisions.md) |
| Docker-in-Docker | Slower, separate image cache, needs a privileged container |
| An ORM | sqlc keeps queries as reviewed SQL with generated types |
