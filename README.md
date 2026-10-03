# ai-compare

A local, single-user tool for comparing AI coding agents side by side on your own projects: same prompt, same copy of the project, two configurations (CLI, provider, model, effort, mode). It measures cost, time and tokens, and produces a report.

## Run it

You only need [Docker Desktop](https://www.docker.com/products/docker-desktop/) (or Docker Engine with Compose).

```bash
git clone https://github.com/jmrona/ai-compare.git
cd ai-compare
docker compose up -d
```

Open **http://localhost:4700**. That is all: Node, Go and PostgreSQL run inside the containers, and no `.env` is required.

- **Stop:** `docker compose down` (your data stays in Docker volumes).
- **Update after pulling changes:** `docker compose up -d --build`.
- **API keys or ports:** copy `.env.example` to `.env` and edit it, then run `docker compose up -d` again.

> The UI currently runs on built-in sample data (`VITE_USE_MOCKS=true`), because the backend only implements the health check so far. You can start comparisons, answer the interactive side from its terminal, finish or cancel sides and generate reports.

## Repository layout

```
ai-compare/
  compose.yaml      entry point for `docker compose up`
  .env.example      optional shared configuration (backend, frontend and infra read the same .env)
  backend/          Go API (phase 0: health check and serving the built frontend)
  frontend/         React + TypeScript + Tailwind + shadcn/ui, TanStack Router and Query
  infra/            Compose stack (api + postgres) and Dockerfiles
  PLAN.md           product and technical plan (in Spanish)
  mockups/          design mockups
```

## Development without Docker

Requires Node.js 22+, pnpm 10+ and Go 1.26+.

| Command | What it does |
|---|---|
| `pnpm install` | Install frontend dependencies |
| `pnpm dev` | Frontend dev server at http://localhost:5173 |
| `pnpm build` | Type-check and build the frontend |
| `pnpm lint` | Lint the frontend (oxlint) |
| `pnpm backend:dev` | Run the Go API on the host, reading the root `.env` |
| `pnpm backend:test` | Run the Go tests |
| `pnpm db:up` | Start only PostgreSQL (published on 127.0.0.1:55432) |

## Configuration

All settings have defaults. A `.env` at the repo root overrides them:

- **Docker Compose** uses it for the containers and for values such as ports.
- **The Go backend** looks for it in the working directory and its parents when run on the host.
- **Vite** reads it through `envDir`; only `VITE_*` variables reach the browser.

Set `VITE_USE_MOCKS=false` to make the frontend call the real backend at `VITE_API_BASE_URL`.
