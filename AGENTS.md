# AGENTS.md

Instructions for AI coding agents working on this repository.

## Read first

ai-compare is a local, single-user, dockerised web app that runs two AI coding-agent configurations side by side on the user's own project and measures cost, time and tokens.

The documentation in [`doc/`](doc/README.md) explains how everything works and why. Before changing a part of the system, read the document that covers it:

| If you touch… | Read |
|---|---|
| Anything, for the first time | [Architecture](doc/02-architecture.md), [Repository layout](doc/03-repository-layout.md) |
| `backend/internal/comparison` | [Comparison lifecycle](doc/04-comparison-lifecycle.md) |
| `backend/internal/workspace`, the copier scripts | [Project copy and side images](doc/05-project-copy-and-images.md) |
| `backend/internal/proxy` | [Inference proxy](doc/06-inference-proxy.md) |
| `backend/internal/terminal`, `TerminalView` | [Terminals](doc/07-terminals.md) |
| Networks, ports, secrets, `netguard` | [Networking and security](doc/08-networking-and-security.md) |
| `backend/internal/catalog`, pricing | [Models and pricing](doc/09-models-and-pricing.md) |
| `proto/`, `backend/internal/rpc`, API routes | [API and contracts](doc/10-api-and-contracts.md) |
| `backend/internal/db`, migrations, queries | [Database](doc/11-database.md) |
| `frontend/` | [Frontend](doc/12-frontend.md) |
| `backend/internal/comparison/agent.go`, a new CLI | [Agent CLIs](doc/13-agent-clis.md) |
| A design choice | [Decisions](doc/17-decisions.md): do not reverse one without saying so |

[`PLAN.md`](PLAN.md) is the original product and technical plan, including what is not built yet; [Status and roadmap](doc/18-status-and-roadmap.md) lists where the code differs from it.

## Rules

- **British English** in all code, identifiers, comments, UI text, commit messages and documentation (colour, behaviour, catalogue, initialise). Nothing in Spanish anywhere in the repository, `PLAN.md` included.
- **No AI attribution** in commits or pull requests: never add `Co-Authored-By: …` trailers or "Generated with …" lines.
- **No comments** in the code. The code must be autoexplain following best practise
- **Docker is the only host requirement.** Do not add steps that need Node, Go or other tools on the host for normal use. Host-side commands must be pnpm scripts or `docker compose` commands that work on macOS, Windows and Linux (no bash-only scripts).
- **Never touch the user's original project.** It is mounted read-only only in the copy helper; agents work on copies.
- **Keys stay in `api`.** Never pass provider API keys to agent containers, the browser, logs or the database.
- **Generated code is committed and never edited by hand:** `backend/internal/gen/`, `frontend/src/gen/`, and the sqlc output in `backend/internal/db/` (everything except `postgres.go` and `fs.go`). After changing a `.proto` file or SQL, run `pnpm gen` and commit the result with the change.
- **New API surface goes to Connect** (`proto/aicompare/v1`), not new JSON routes. Terminals stay on WebSocket.
- **LF line endings.** Scripts that run in Linux containers must not have CRLF.
- **Label Docker objects** that the backend creates with `ai-compare.comparison`, `ai-compare.side` and `ai-compare.role`.
- **Match the surrounding code:** comment density, naming, small focused packages, standard library first in Go.
- **Keep `doc/` up to date.** When behaviour, a route, a setting or a decision changes, update the matching document in the same change.

## Commands

```bash
docker compose up -d --build
```

```bash
pnpm test
```

```bash
pnpm gen
```

```bash
pnpm lint
```

- `pnpm test` runs the Go tests in Docker; `pnpm lint` and `pnpm build` check the frontend (they need Node and pnpm).
- The app is at http://localhost:4700. Logs: `docker compose logs -f api`.
- End-to-end checks: `docker compose exec api /app/spike proxy-check` (in Git Bash on Windows, prefix with `MSYS_NO_PATHCONV=1`).

More in [Development](doc/15-development.md).

## Before finishing a change

1. Backend: `go vet ./...` and `pnpm test` pass; `gofmt` reports nothing.
2. Frontend: `pnpm build` (type-check) and `pnpm lint` pass.
3. Generated code is up to date (`pnpm gen` leaves no diff).
4. The stack rebuilds and the affected flow works in the browser.
5. `doc/` reflects the change.
