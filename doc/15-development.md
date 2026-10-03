# 15. Development

## Run it

The only host requirement is Docker with Compose.

```bash
docker compose up -d
```

Open http://localhost:4700. After pulling or changing code:

```bash
docker compose up -d --build
```

Logs:

```bash
docker compose logs -f api
```

To run real comparisons, copy `.env.example` to `.env` and set `OPENAI_API_KEY`.

## Scripts

All scripts are run from the repo root with `pnpm <script>`. The ones marked "Docker" need nothing but Docker.

| Script | Needs | What it does |
|---|---|---|
| `infra:up` | Docker | `docker compose up -d --build` |
| `infra:down` | Docker | `docker compose down` (volumes and data are kept) |
| `infra:logs` | Docker | Follow all logs |
| `db:up` | Docker | Start only Postgres (for running the backend on the host) |
| `gen` | Docker | `buf lint`, `buf generate` and `sqlc generate` in the pinned generator image |
| `test` | Docker | Go tests in a `golang:1.26-alpine` container (module and build caches in volumes) |
| `dev` | Node, pnpm | Vite dev server on 5173, proxying `/api` to the backend |
| `build` | Node, pnpm | Type-check and build the frontend |
| `lint` | Node, pnpm | oxlint |
| `backend:dev` | Go | Run the API on the host (reads the root `.env`) |
| `backend:build` | Go | Build `backend/bin/server` |
| `backend:test` | Go | Go tests on the host |

## Configuration

Everything has a default; the root `.env` overrides it. Compose injects it into `api`; the Go backend finds it by walking up from its working directory when run on the host (variables already in the environment win); Vite reads it through `envDir` and only exposes `VITE_*` to the browser.

| Variable | Default | Meaning |
|---|---|---|
| `APP_PORT` | 4700 | UI and API port, published on 127.0.0.1 |
| `PROXY_PORT` | 4701 | Inference proxy port, internal only |
| `FRONTEND_DEV_PORT` | 5173 | Vite dev server |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Host socket mounted into `api` (Colima: `~/.colima/default/docker.sock`) |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | `aicompare` | Database credentials |
| `POSTGRES_PORT` | 55432 | Host port for Postgres (uncommon to avoid clashes) |
| `DATABASE_URL` | localhost:55432 in `.env`; Compose overrides it to `postgres:5432` inside Docker | Connection string |
| `OPENAI_API_KEY` | empty | Needed for real comparisons |
| `ANTHROPIC_API_KEY` | empty | Phase 2 |
| `LOCAL_MODELS_BASE_URL` | `http://host.docker.internal:11434/v1` | Local OpenAI-compatible server (phase 2) |
| `MODELS_DEV_URL` | `https://models.dev/api.json` | Catalogue source |
| `CATALOG_PROVIDERS` | `openai,anthropic` | Providers kept from the catalogue |
| `VITE_API_BASE_URL` | `/api` | API base as seen by the browser |
| `VITE_USE_MOCKS` | `true` | `true`: hybrid client (real backend + sample data for the rest); `false`: real backend only |

Set by Compose, not usually changed: `STATIC_DIR=/app/web`, `DATA_DIR=/data/app`, `STAGING_DIR=/data/staging`, `STAGING_VOLUME=ai-compare_staging`, `AGENT_NETWORK=ai-compare-agents`.

## Docker objects

| Object | Name | Contents |
|---|---|---|
| Volume | `ai-compare_pgdata` | Postgres data |
| Volume | `ai-compare_appdata` | `catalog.json` cache |
| Volume | `ai-compare_staging` | Project copies, `<id>/project` |
| Volume | `ai-compare_artifacts` | Reserved for diffs, recordings and test output |
| Volumes | `ai-compare_gomod`, `ai-compare_gocache` | Go caches for `pnpm test` |
| Network | `ai-compare` | api, postgres |
| Network | `ai-compare-agents` | api, agent containers |
| Images | `ai-compare/api:local`, `ai-compare/copier:<hash>`, `ai-compare/side:<id>-<side>`, `ai-compare-gen` | |
| Labels | `ai-compare.comparison`, `ai-compare.side`, `ai-compare.role` (`agent`, `side`, `copier`, `copy-project`, `inspect-project`) | On everything api creates |

List or clean what ai-compare created:

```bash
docker ps -a --filter label=ai-compare.role=agent
```

```bash
docker image ls "ai-compare/side"
```

## Tests

```bash
pnpm test
```

Go unit tests cover: the models.dev parser and cache, usage parsing for every API shape (JSON and SSE, errors inside streams), cost with long-context tiers, limits, netguard subnets, host path splitting, the build context for empty projects, and the tar-to-zip conversion.

End-to-end behaviour (Docker, real providers) is checked with the spike commands below and by hand in the browser.

## Spike commands

Run inside the `api` container. In Git Bash on Windows, prefix `docker compose exec` with `MSYS_NO_PATHCONV=1` so `/app/...` is not rewritten.

```bash
docker compose exec api /app/spike prepare "C:\Users\me\projects\my-app"
```

Copies a project into staging and builds side A's image, printing timings (points 1 and 2). Accepts `--runtime` and `--setup`.

```bash
docker compose exec api /app/spike proxy-check
```

Network isolation checks from the agent network, and, with `OPENAI_API_KEY`, real requests through the proxy with usage and cost (points 3 and 6). `--model` picks the model.

## Debugging tips

| Symptom | Where to look |
|---|---|
| A side fails during build | The side's Logs tab (last builder lines), or `docker compose logs api` |
| "docker cannot access this folder" | Docker Desktop → Settings → Resources → File sharing; the path must be absolute |
| Costs show "—" | The model has no price on models.dev, or the provider did not report usage |
| A provider error (no credit, wrong model) | Logs tab, `proxy` lines in amber; the error text is recorded even inside streams |
| Terminal does not fill the pane | Resize the window once; check the WebSocket in DevTools (one connection, binary frames) |
| Port 4700 answers with something else | Another process on the port (for example a `go run` left running) |
| The UI shows sample data | `VITE_USE_MOCKS=true` is the default; only parts not built on the backend use samples |

Talk to the database:

```bash
docker compose exec postgres psql -U aicompare -c "select comparison_id, side, status, end_reason from comparison_sides order by updated_at desc limit 10"
```

## Commits

- British English in code, comments and messages.
- No AI attribution trailers (`Co-Authored-By: …`) in commits or pull requests.
- Regenerate (`pnpm gen`) and commit generated code together with the `.proto` or SQL change that caused it.
