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

Set by Compose from the host: `HOST_HOME` (the user's home folder, from `USERPROFILE` or `HOME`), where the folder browser starts.

Set by Compose, not usually changed: `STATIC_DIR=/app/web`, `DATA_DIR=/data/app`, `STAGING_DIR=/data/staging`, `STAGING_VOLUME=ai-compare_staging`, `ARTIFACTS_DIR=/data/artifacts`, `ARTIFACTS_VOLUME=ai-compare_artifacts`, `AGENT_NETWORK=ai-compare-agents`. The `*_VOLUME` names are passed to the containers `api` creates, which is why both volumes have fixed names in Compose.

Chosen in the app instead (`/settings`, saved in Postgres): report model, automatic report, default limits, CPUs and memory per side, retention days.

## Docker objects

| Object | Name | Contents |
|---|---|---|
| Volume | `ai-compare_pgdata` | Postgres data |
| Volume | `ai-compare_appdata` | `catalog.json` cache |
| Volume | `ai-compare_staging` | Project copies, `<id>/project`, and hidden tests, `<id>/hidden` |
| Volume | `ai-compare_artifacts` | Per side, `<id>/<side>/`: `workspace.tar`, `solution.diff`/`.numstat`, `harness.diff`/`.numstat`, `session.json`, `tests-visible.log`, `tests-hidden.log`, `terminal.cast`. Never removed by retention |
| Volumes | `ai-compare_gomod`, `ai-compare_gocache` | Go caches for `pnpm test` |
| Network | `ai-compare` | api, postgres |
| Network | `ai-compare-agents` | api, agent containers |
| Images | `ai-compare/api:local`, `ai-compare/copier:<hash>`, `ai-compare/side:<id>-<side>`, `ai-compare/result:<id>-<side>`, `ai-compare-gen` | Side and result images are removed by retention |
| Labels | `ai-compare.comparison`, `ai-compare.side`, `ai-compare.role` (`agent`, `side`, `result`, `collect`, `test`, `test-hidden`, `copier`, `copy-project`, `inspect-project`, `list-folders`) | On everything api creates; retention selects by `ai-compare.comparison` |

List what ai-compare created:

```bash
docker ps -a --filter label=ai-compare.role=agent
```

```bash
docker ps -a --filter label=ai-compare.comparison=ra3f80e
```

```bash
docker image ls --filter label=ai-compare.role
```

Do not remove these by hand while a comparison is live. To free space, use **Clean up now** in `/settings` (it applies the retention rule) or delete a comparison from its history page; both remove only that comparison's objects.

Look at a side's artefacts:

```bash
docker compose exec api ls -la /data/artifacts/ra3f80e/A
```

## Tests

```bash
pnpm test
```

Go unit tests cover: the models.dev parser and cache, usage parsing for every API shape (JSON and SSE, errors inside streams), cost with long-context tiers, limits, netguard subnets, host path splitting and rebuilding, the build context for empty projects, the tar-to-zip conversion, the human wait heuristic, diff and numstat parsing, and reading an opencode session export (`backend/internal/comparison/comparison_test.go` with `testdata/session.json`).

End-to-end behaviour (Docker, real providers) is checked with the spike commands below and by hand in the browser. Phase 1 was verified that way on Windows with real OpenAI runs: copy, build, run, verification with visible and hidden tests, events, diffs, report, recording replay, download, restart with reattachment, Finish and Cancel, and deleting a comparison.

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
| A side stays in `verifying` | Logs tab, `verify` lines; a test suite can take up to 10 minutes before it is stopped. `docker ps --filter label=ai-compare.role=test` shows a test run in progress |
| Tests fail only in ai-compare | They run without network and as the user `agent`: a suite that downloads something or needs root fails. Read `tests-visible.log` in the Tests tab |
| Changes, Tests or Events are empty after the side ended | Verification could not collect the result: see the `verify` lines in the Logs tab |
| The status bar says "reconnecting…" | The event stream is down: is `api` running (`docker compose ps`)? It reconnects on its own and refetches everything |
| The UI does not update | Check the `EventService/Watch` request in DevTools: it should stay open and receive a message at least every 20 s |
| A report ends in `error` | The error is on the report page; usually the report model is not available to the API key, or `api` restarted while it was being generated. Generate it again |
| Terminal does not fill the pane | Resize the window once; check the WebSocket in DevTools (one connection, binary frames) |
| Port 4700 answers with something else | Another process on the port (for example a `go run` left running) |

Talk to the database:

```bash
docker compose exec postgres psql -U aicompare -c "select comparison_id, side, status, failure, end_reason from comparison_sides order by updated_at desc limit 10"
```

```bash
docker compose exec postgres psql -U aicompare -c "select data from settings"
```

## Commits

- British English in code, comments and messages.
- No AI attribution trailers (`Co-Authored-By: …`) in commits or pull requests.
- Regenerate (`pnpm gen`) and commit generated code together with the `.proto` or SQL change that caused it.
