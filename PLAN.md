# ai-compare — A local platform for comparing AI agents

## Problem

I want to know which combination of CLI, provider, model, effort and preset best solves a task **from my real work**, how much it costs and how long it takes. Today I cannot compare two combinations on the same project and the same prompt without touching my development environment.

The results of each comparison are **disposable**: the generated code is not pushed to GitLab or any other remote. Only the metrics (cost, time, tokens), the quality of the solution and the report matter.

## Concept

1. In the dashboard I enter the **absolute path** of a project on my machine (for example `C:\Users\Jose\Desktop\projects\my-app`).
2. I configure two sides, A and B. Each side has its CLI (`claude`, `codex` or `opencode`), provider, model, effort, preset and mode (interactive or autonomous).
3. I write a shared prompt.
4. A deterministic script, with no LLM, builds **one Docker image per side**. Each image contains:
   - the copy of the project as it is;
   - the project without its original harness files;
   - the chosen preset;
   - the chosen CLI.
5. When both images are ready, it starts both containers, opens each side's CLI and sends it the prompt automatically.
6. The dashboard shows the two terminals **side by side**. In interactive mode I answer each agent's questions from there.
7. When both sides finish, I generate the report: cost, time, tokens, diff, tests and quality review.

It is a local, single-user tool. You clone it from GitHub and bring it up with Docker Compose. There are no accounts, no login and no hosted deployment.

## How it is used

1. **Project.** I type or paste the absolute path. The app checks that it exists and shows:
   - name and estimated size of the copy;
   - number of files that will be copied;
   - harness files that will be excluded;
   - sensitive files detected (`.env*`), excluded by default.
2. **Project profile.** The first time I use a path, the app proposes a profile (runtime, setup command and test command) based on what it detects, for example `package.json` and its lockfile. I confirm or edit it, and it is saved for that path.
3. **Prompt.** I write it in a textarea.
4. **Sides A and B.** I choose CLI, provider/model, effort, preset and mode. Incompatible combinations are disabled with an explanation.
5. **Run.** I press **Run comparison**. I see the progress of the preparation: copy, setup, build and start-up.
6. **During the run.** I see two terminals side by side, the status of each side and the accumulated cost and tokens live.
7. **Finish.** In autonomous mode, a side finishes when the CLI exits. In interactive mode, it finishes when I exit the CLI or press **Finish side**. I can also cancel. An error or a cancellation on one side does not block the report.
8. **Report.** When both sides are in a terminal state (success, error or cancelled), **Generate report** is enabled.
9. **History.** Every comparison is saved with its report, diffs, logs and metrics.

## Pages

| Route | What it is for |
|---|---|
| `/` | Configure and launch a comparison, and follow it live |
| `/harnesses` | Create, edit and delete presets |
| `/pricing` | Prices from models.dev, read-only |
| `/history` | List of comparisons grouped by date |
| `/history/:id` | Detail of a comparison |
| `/settings` | API key status and default values |

A fixed navigation bar gives access to all of them. If a comparison is in progress, it shows an indicator that leads to `/`.

### `/` — New comparison

The page has two states: **configuration** and **execution**. On launch, the form collapses into a summary strip (project, truncated prompt, A vs B) and the screen switches to the two panels.

**Configuration:**

1. **Project.**
   - Absolute path field with a history of recent paths.
   - On validation, it shows name, size, number of files, harness files that will be excluded and `.env*` files detected.
   - Collapsible **Project profile** block: runtime, setup command, test command, additional exclusions and hidden tests folder. It is preloaded with the profile saved for that path or with automatic detection.
2. **Prompt.** Large textarea with a character counter.
3. **Side A and side B,** in two columns with the same controls:
   - **CLI:** `claude`, `codex` or `opencode`.
   - **Provider:** filtered to those the CLI supports and that have an API key configured.
   - **Model:** filtered by provider; the list comes from models.dev (and, for local models, from the local server). If the model has no price on models.dev, a warning appears: the cost will be "not calculable".
   - **Effort:** disabled if the CLI or the model does not support it.
   - **Preset:** those from `/harnesses` compatible with the CLI, plus **No harness** (a fixed option that runs the CLI without any harness file). A link opens the preset in another tab.
   - **Mode:** interactive or autonomous.
   - **Limits (optional):** timeout, maximum tokens and maximum cost. Each has its own toggle and can be left **unlimited**. They are preloaded from `/settings`, where they are off by default. The cost limit does not appear with local models.
   - **Copy A → B** and **Swap A ↔ B** buttons.
4. **Run comparison.** Disabled until everything is valid; errors are shown next to each field.

**Execution:** two panels side by side, one per side. Each panel has:

- **Header:**
  - CLI · model · preset · mode;
  - status: preparing, building, starting, running, waiting for input, finished, error, cancelled or limit reached;
  - elapsed time, tokens and cost live;
  - **Finish side** and **Cancel** buttons.
- **Tabs:**

  | Tab | Content |
  |---|---|
  | **Terminal** | Interactive TUI (xterm.js). In autonomous mode it is read-only. |
  | **Logs** | Preparation log (copy, setup, build), infrastructure log and inference proxy log (requests, 429s, retries), with a level filter. |
  | **Changes** | Solution diff against the baseline, refreshable on demand during the run and final when it ends. Includes the list of modified files. |
  | **Metrics** | Tokens by category, accumulated cost (against the limit, if any), requests and times: preparation, agent and human wait. |
  | **Preview** | Phase 2: the application running on that side. |

- **Shared footer**, when both sides are in a terminal state:
  - **Generate report**, which leads to `/history/:id` with the report being generated;
  - **New comparison**, which returns to the form with the same configuration preloaded.

### `/harnesses` — Presets

**List:**

- One card per preset with title, description, compatible CLIs, number of files and date of last edit.
- Search by name.
- **New preset** button.
- **No harness** appears as a fixed system preset: it cannot be edited or deleted.

**Create** (`/harnesses/new`):

1. Name (required; the folder slug is derived from it), description and compatible CLIs.
2. Content, through any of these routes, which can be combined:
   - **Import from a project:** I enter a path, the app detects its harness files and shows a checklist to choose which to import.
   - **Drop folders or files:** `.claude/`, `.agents/`, `AGENTS.md`, `.mcp.json`, etc. They are placed in `project/` keeping their relative path.
   - **Drop into `home/`:** a separate zone for user configuration, such as `.codex/config.toml`.
3. Saving creates the folder `harnesses/<slug>/` with `preset.md`, `project/` and `home/`.

**Detail and editing** (`/harnesses/:slug`):

- **Header:** title and description editable inline, and compatible CLIs.
- **Cards by category**, inferred from the paths: instructions (`AGENTS.md`, `CLAUDE.md`), skills, rules, agents/subagents, MCP and others. Each card lists its files.
- **File tree** with two roots, `project/` and `home/`.
- **Viewer and editor:**
  - rendered Markdown with an **Edit** button, which opens a text editor (Markdown, JSON, TOML, YAML) with syntax validation for JSON and TOML;
  - MCP secrets must be written as references to environment variables; if the editor detects a value that looks like a key, it warns.
- **Add files** by dropping them onto a folder in the tree; **rename, move and delete** files from the tree.
- **Preset actions:**
  - **Duplicate**, to create variants;
  - **Rename**;
  - **Delete**, with confirmation. Older comparisons are not affected because they keep their own copy of the preset.
- **Usage:** number of comparisons that used it, with a link to `/history` filtered by that preset.

### `/pricing` — Prices (read-only)

Shows the prices from **models.dev** (`https://models.dev/api.json`) directly. There is no price table of our own and no editing.

- **Rows:** one per model from the configured providers (OpenAI in phase 1; Anthropic and local models later).
- **Columns,** in USD per million tokens:
  - input;
  - cached input (cache read);
  - cache write;
  - output;
  - context limit.

  "Cached output" does not exist at any provider; the real fourth category is **cache write**. Anthropic charges for it separately, OpenAI does not (shown as "—"). Reasoning tokens are billed as output. If models.dev publishes long-context tiers, they are shown as sub-rows.
- **Header:** date of the last query to models.dev and an **Update** button. If the query fails, the cached copy is shown with its date and a warning.
- **Filters:** provider and search box.
- **Local models** (phase 2): they appear as "local · no cost", because they are not on models.dev and are not billed.

Why models.dev: neither the OpenAI API nor the Anthropic API returns prices (their `/v1/models` endpoints only list models). models.dev is a public catalogue by provider and model, maintained by the opencode team, with `cost.input`, `cost.output`, `cost.cache_read`, `cost.cache_write` and context limits.

The internal workings (cache, price snapshot per comparison, models without a price) are in [Prices](#prices).

### `/history` — History

- **Grouped by date:** Today, Yesterday, and then a header per day (for example, "Tuesday 29 Sep 2026"). Within each group, most recent first.
- **One row per comparison:**

  | Column | Content |
  |---|---|
  | Time | Start time |
  | Project | Folder name, with the full path in a tooltip |
  | Prompt | First few words, truncated |
  | A | CLI · model · preset |
  | B | CLI · model · preset |
  | Status | Status of each side |
  | Cost | A / B, with the lower highlighted |
  | Duration | A / B, with the shorter highlighted |
  | Tests | A / B result |
  | Report | Generated or pending |

- **Filters:** text (prompt or project), date range, CLI, model, preset and status. Filters are reflected in the URL so they can be linked.
- **Pagination or infinite scroll** by date group.
- Click on a row → `/history/:id`.

### `/history/:id` — Comparison detail

1. **Header:** date and time, project (path), total duration and status. Actions:
   - **Repeat comparison:** opens `/` with everything preloaded;
   - **Generate/Regenerate report**;
   - **Delete comparison**, with confirmation;
   - **Export** to JSON or Markdown.
2. Full **prompt**, with a copy button.
3. **Configuration A | B** in two columns:
   - CLI and its version, provider, model, effort, mode, limits;
   - preset with a link to its saved copy (the one from the time of the run, not the current one) and its hash;
   - project profile and hash of the copy.
4. **Result:** A/B table with final status, end reason, duration (agent and human wait), tokens by category, estimated and confirmed cost, tests, files and lines changed, and findings by severity. Comparison bars for cost, tokens and duration. models.dev price applied on each side (snapshot with its date).
5. **Report:**
   - conclusions of the comparative judge;
   - analysis per side;
   - reviewer findings, with a link to file and line in the diff;
   - cost of the report itself;
   - if it does not exist yet, a button to generate it.
6. **Artefacts per side,** with the same tabs as during execution:
   - **Terminal:** replay of the PTY log with speed controls;
   - **Logs;**
   - **Changes:** solution diff and harness diff;
   - **Tests:** full output;
   - **Events:** timeline of the JSONL;
   - **Preview:** phase 2.

### `/settings` — Settings

- **API key status**, read from `.env`: configured or not, without showing its value. A button to test the connection through the proxy.
- **Default limits** for each side: each with its own toggle, all **off** by default.
- **Report:** model that generates it and a **Generate automatically** option when the comparison ends (off by default).
- **Exclusion list** of harness files.
- **Retention policy** and current disk usage.
- **Pinned versions** of each CLI.
- **Local models** (phase 2): URL of the local server (for example, `http://host.docker.internal:11434/v1` for Ollama), a button to test the connection and a list of detected models.

## Architecture

### Local stack

Docker Compose starts two services. Each piece is described in detail in [Technical stack](#technical-stack).

- **`api` (Go).** Serves:
  - the compiled frontend;
  - the REST API;
  - the event stream (Connect);
  - WebSocket for the terminals;
  - the inference proxy, on a separate port reachable only from the internal network.

  It also orchestrates the containers.
- **`postgres`.** Database.
- **Persistent volumes** for the database, project profiles, presets (phase 2) and artefacts (diffs, terminal recordings, logs, reports).
- **Docker access** through the host socket, mounted only in `api`. Agent containers never see it.

**Host requirements:** Docker and Docker Compose. Go, Node.js and the CLIs are not needed on the host.

### Access to the project path

The `app` service runs inside Docker and cannot see `C:\...` directly. To copy the project:

1. `app` asks the Docker daemon for a **helper container** that mounts the host path read-only (`<path>:/src:ro`) plus a staging volume.
2. The helper container copies the selected files into staging (see *Project copy*).
3. `app` builds the images using staging as the context.

The original project is never modified.

### Project copy

- **If the project is a Git repository**, tracked files are copied, plus untracked files that are not ignored (`git ls-files --cached --others --exclude-standard`, minus deleted ones). This includes uncommitted changes: the copy reflects the folder **exactly as it is**. Git runs read-only, with hooks and `fsmonitor` disabled.
- **If it is not a Git repository**, everything is copied except a default list (`node_modules`, `dist`, `build`, `.venv`, `target`, etc.), editable in the profile.
- **The original `.git` is not copied.** The copy has no remotes, so the agent cannot push to GitLab. In exchange, the agent cannot see the project history (`git log`, `blame`).
- **`.env*` is excluded by default**, because its contents would end up sent to the provider. The profile can include it explicitly.
- **Harness files.** In **phase 1** they are copied as they are: each side uses the harness the project already has, and if it has none, runs without one. **From phase 2 onwards**, with presets, they are excluded at the root and when nested:
  - `AGENTS.md`, `CLAUDE.md`, `CLAUDE.local.md`, `GEMINI.md`;
  - `.claude/`, `.agents/`, `.codex/`, `.opencode/`, `opencode.json`, `opencode.jsonc`, `.mcp.json`;
  - `.cursor/`, `.cursorrules`, `.github/copilot-instructions.md`.

  The list lives in configuration and can be extended.
- **Configurable size limit.** If the copy exceeds it, a warning is shown before building.

### Project profile

Stored per path, it contains:

- **Base runtime:** an image such as `node:22` or `python:3.12`, or the project's own `Dockerfile` or `devcontainer.json` if there is one.
- **Setup command,** for example `npm ci`. It runs **inside the image**: dependencies installed on Windows are no use in a Linux container, and `node_modules` is not copied.
- **Test/build/lint command**, optional.
- **Additional exclusions.**
- **Preview command and port**, optional (phase 2).

### Building images

Images are built in layers so that both sides start from exactly the same state and the Docker cache speeds up repeats:

```
base runtime (profile)
 └─ project layer: copy + setup              ← shared by A and B
     ├─ side A layer: CLI A + preset A + Git baseline
     └─ side B layer: CLI B + preset B + Git baseline
```

- **CLI images are versioned.** The exact version of each CLI is pinned and recorded in the run.
- **No layer contains API keys.**
- **Preparation (copy, setup and build) does not count** towards the run time. It is recorded separately.

### Git baseline and a clean diff

In each side's layer, after placing the preset:

1. `git init`, with `core.autocrlf=false` so the diff reflects the real bytes and Windows line endings do not create noise.
2. `git add -A` and `git commit -m baseline`.

The baseline contains the project's uncommitted changes, without the original harness and with the preset. When the run finishes:

```bash
git add -A
git diff --cached baseline -- . ':(exclude)AGENTS.md' ':(exclude)CLAUDE.md' ':(exclude).claude' ':(exclude).agents' ...
```

The diff exclusions come from the same list of harness files that the copy uses.

- **Solution diff:** what the agent changed in the code. This is what the reviewer sees and what appears in the report.
- **Harness diff:** changes the agent makes to its own files (for example, if it writes to `CLAUDE.md`). Shown in a separate tab.

### Execution and terminals

- **One container per side**, created from its image, with the same CPU and memory quota on both sides. The container user is not root.
- **Each container has a PTY** relayed over WebSocket to an xterm.js terminal. The terminal supports input, ANSI output, resizing and control keys. Its commands run inside the container, never on the host.
- **The initial prompt is sent according to the CLI adapter:**
  - **Autonomous:** the prompt is passed as an argument to the CLI's non-interactive mode, with permissions set to no confirmations. The container is the security boundary; no attempt is made to maintain an allowlist of commands.
  - **Interactive:** the CLI starts in its TUI with the initial prompt as an argument, if the CLI supports it. If not, the runner writes it to the PTY once the TUI is ready.

  The exact form for each CLI is validated in the spike.
- **Effort:** each adapter translates the value into its CLI's option. If a CLI or model does not support it, the control is disabled in the panel.
- **Per-side limits, all optional:** timeout, maximum tokens and maximum cost. Without limits, the side runs until the CLI finishes or the user ends or cancels it; the proxy keeps measuring regardless. With limits, the proxy enforces the token and cost limits by cutting off requests and marking the side as "limit reached".

### Inference proxy (measurement and keys)

Measuring cost is the main goal, and in interactive mode the TUI does not report usage in a structured way. That is why all traffic to the provider goes through **our own inference proxy, written in Go inside `api`**. The technical detail is in [Inference proxy](#inference-proxy-go).

- **Each side receives a base URL** pointing at the proxy and a **side token**. **The real API key never enters the container.**
- **It does not translate formats.** Each CLI speaks its provider's native API (see [CLI ↔ provider compatibility](#cli--provider-compatibility)), so the proxy only forwards.
- **For each request it records** usage (input, cache read, cache write, output), latency and errors, attributed to the side by its token.
- **The same data feeds** the live cost and tokens on the dashboard and, if enabled, the cost and token limits: once they are exceeded, the proxy rejects the side's requests and marks it as "limit reached".
- **Cross-check:** at the end, the CLI's session files inside the container are also read, to cross-check usage and extract events (commands, files touched). If they do not match, the proxy wins and the discrepancy is noted.

API keys are defined in `ai-compare/.env`, created from `.env.example` and listed in `.gitignore`. The UI only shows whether each key is configured.

### End of a run and verification

1. The side reaches a terminal state: success, error, cancelled or limit reached. **Infrastructure errors** (build, start-up, proxy) **are recorded separately** from agent errors.
2. The runner extracts both diffs, the PTY logs and the CLI's session files.
3. **If the profile defines a test command**, the runner runs it in a **new container** created from the side's image with the diff applied, not in the container where the agent worked.
4. **Optional hidden tests:** the user can point to a test folder on the host that the agent never sees. It is copied only during verification, so the agent cannot tailor the tests to its solution.
5. The agent's container is **stopped but not removed** until the retention policy applies. This allows the preview to be relaunched later (phase 2).

### Reconnection and recovery

**The browser is only a viewer.** The whole comparison lives in the backend: the containers, the orchestrator, the measuring proxy and each terminal's recording. Closing the tab stops nothing.

**If you close the tab or the connection drops:**

1. The containers keep running, the proxy keeps measuring and the orchestrator keeps enforcing limits and timeouts.
2. In interactive mode, if the agent asks a question while nobody is watching, it waits for your answer. Time spent waiting counts as human wait time, not agent time.
3. When you reopen ai-compare, the top bar shows "comparison in progress" and `/` opens the execution view directly.
4. Each terminal is recovered as follows:
   - the WebSocket reconnects on its own, with retries;
   - the backend sends the output accumulated since the last screen clear, so xterm.js can rebuild what was on screen;
   - the browser sends its terminal size and the backend applies it to the container. That resize makes the `opencode` TUI redraw completely, so the screen ends up exact.
5. Metrics and states are recovered by reopening the event stream from the last event received, and TanStack Query refetches the current state.

**Several tabs** on the same comparison work at once: they all see the same terminal and any of them can type.

**If the backend restarts** (`api`):

- **On start-up,** the orchestrator finds the containers by label and reattaches to those still alive.
- **Output produced while `api` was down is not lost.** Docker keeps each container's output in its logs, TTY included. The recording is completed with `docker logs --since <last recorded instant>`.
- **The proxy** comes back with the backend. Requests the CLI attempts during the outage fail and the CLI retries them; they are recorded as infrastructure errors.

**What cannot be recovered:** if Docker Desktop or the machine restarts, the containers stop and their process is lost. Those sides end as infrastructure errors, with everything recorded up to that point (diff, usage, recording).

## Presets

**Phase 2.** In phase 1 each side uses the harness the project already has.

A preset is a reusable set of harness files. It lives **on disk**, inside the ai-compare data volume:

```
harnesses/
  my-preset/
    preset.md        ← frontmatter: title, description, compatible clis
    project/         ← literal mirror of what is copied to the project root
      AGENTS.md
      CLAUDE.md
      .claude/skills/...
      .agents/...
      .mcp.json
    home/            ← copied to the container user's home
      .codex/config.toml
```

- **`project/` is a literal mirror**, with no translation between formats. A preset can carry files for several CLIs at once (`CLAUDE.md` and `AGENTS.md`, `.claude/` and `.agents/`), and each CLI reads what it understands.
- **`home/` exists** because part of the configuration does not live in the project. For example, Codex reads its MCP servers from `~/.codex/config.toml`.
- **MCP secrets** go in as references to environment variables in `.env`, never written into the preset.
- **`preset.md`:** its frontmatter holds the title, description and compatible CLIs; the body is free-form notes. Title and description are what the user sees in the list.
- **Each run** stores a copy of the preset used and its hash. Editing a preset later does not change what explains an old result.
- **No harness:** a fixed system preset. The side runs without any harness files: the project's ones are removed and nothing is added. Useful for measuring the model and the CLI "out of the box".

The interface for managing presets is described in [`/harnesses`](#harnesses--presets).

Outside the MVP: cards as an input mechanism with automatic translation to each CLI, so one "logical" preset can be used in claude, codex and opencode.

## Metrics

Per side:

- **Configuration:** CLI and its version, provider, model, effort, mode, preset with its hash, project path and copy hash.
- **Tokens by category:** input, cache read, cache write and output (reasoning is billed as output). Anything that cannot be measured is marked "not reported", never zero.
- **Estimated cost** using the models.dev price saved when the comparison starts, and **confirmed cost** from the provider when available.
- **Speed:** output tokens per second. Especially useful with local models, where cost is zero and what is compared is time and quality.
- **Times:**
  - preparation (not comparable, for information);
  - agent time;
  - in interactive mode, **human wait time**, estimated as the intervals with no requests in flight that end with user input.
- **Requests, retries and errors** from the provider.
- **Tests:** result of the profile command and of the hidden tests.
- **Diff:** solution diff and harness diff; files changed and lines.
- **Reviewer findings** by severity.

### Prices

- **Single source: models.dev.** There is no price table of our own. `api` downloads `https://models.dev/api.json` and the [`/pricing`](#pricing--prices-read-only) page shows it as is.
- **Download and cache.** models.dev only publishes the full catalogue (~5 MB, 226 providers) at a single URL; it cannot filter by provider. `api` downloads it at most every 24 h or when **Check for updates** is pressed, with `If-None-Match` (an unchanged catalogue returns 304 with no body), keeps only the providers in `CATALOG_PROVIDERS` and stores that subset (~20 KB) with its ETag in the data volume.
- **Ordering and filtering.** Models are sorted by `release_date`, newest first. The model picker hides `deprecated` models and those unsuitable for an agent (no `tool_call` or no text output). The available effort levels come from each model's `reasoning_options`. If models.dev does not respond, the cache is used and its date is shown. If there is neither cache nor connection, the comparison can still start, with cost "not calculable".
- **Snapshot per comparison.** When **Run comparison** is pressed, the full `cost` object of each model used is copied into the `price_snapshots` table, linked to that side, along with the models.dev download date. That comparison's cost is always calculated from this snapshot, even if models.dev changes later.
- **Model catalogue.** The model selects also come from models.dev, filtered by provider. That way the model list and its prices never get out of sync.
- **Model with no price on models.dev:** its cost shows as "not calculable", but tokens are recorded.
- **Local models:** zero cost by definition, marked as "local", not "not calculable".
- **Accepted limitation:** there is no way to correct a price by hand. If models.dev has an error, the estimated cost inherits it; the provider's confirmed cost, when available, serves as a cross-check.

## Report

Generated when **Generate report** is pressed, or automatically if that option is enabled in `/settings`. The stages use a configurable model, and their cost is recorded separately from the comparison's.

**To avoid waiting until the end:** the stages that depend on only one side (reviewing its diff and its analysis) start in the background as soon as that side finishes. When the second side finishes, only the comparative judge remains. Diffs are labelled A/B for the reviewer from the start, so the review stays blind.

1. **Blind code reviewer.** Receives the diffs labelled A and B in random order, without model, CLI or preset. Looks for bugs, functional failures, edge cases, security and test problems; does not score style. Each finding has a file and line, impact and severity. It may find nothing.
2. **Per-side analyst.** Summarises what happened from events, errors, commands, usage, tests and diff, in at most five paragraphs, separating facts from inferences.
3. **Comparative judge.** Receives all of the above plus the metrics and explains which side performed better and where the trade-offs are. It can point to "lower cost", "shorter duration" or "fewer problems" instead of declaring a single winner.

If the judge model is the same as one of those being compared, the report warns about it (self-preference bias).

**Report page:**

- A/B metrics table;
- conclusions;
- comparison bars for cost, tokens by category and duration;
- tests and findings by severity;
- browsable diffs and logs, with links to events.

When there are repeats, a cost-versus-quality chart is added.

## History and retention

- **The database stores, per comparison:**
  - prompt and project path;
  - copy hash and profile used;
  - configuration of both sides, presets with their hashes and CLI versions;
  - states, timestamps, usage, price snapshot, costs and reports;
  - references to the artefacts.
- **Artefacts** (diffs, PTY logs, JSONL, CLI sessions, test results) are stored in persistent volumes.
- **Old comparisons:** terminals are replayed from the log.
- **Configurable retention** (2 days by default): deletes the stopped containers, images and staging copies that ai-compare created, identified by its labels, and never touches the user's other Docker objects. Artefacts and reports are kept, so each side's result stays downloadable from the history.

## Security

- **The UI listens only on `127.0.0.1`.** There is no login. A Jupyter-style session token in the URL was considered and postponed: it only matters if the app is exposed beyond the local machine.
- **WebSockets validate the `Origin` header.** Without this, any website open in the browser could connect to the terminals.
- **Agent containers:**
  - non-root user;
  - no Docker socket;
  - no API keys;
  - CPU and memory quotas.

  In the MVP they have internet access, because the agent may need to install dependencies.
- **The original project is mounted read-only** and only in the copy helper container.

## Validity of the comparison

- **Variance.** A single run per side has high variance. The data model supports **repeats** (side × attempt) from the start, even though the MVP launches one per side. The report warns when there is only one.
- **Interactive mode.** Human intervention makes the comparison less even. Such comparisons are flagged and human wait time is shown separately.
- **Provider limits.** If both sides use the same provider in parallel, they share rate limits. 429s and retries are recorded so they are not mistaken for model slowness.
- **Same starting point.** Both sides start from the same project layer: identical copy and setup.

## Technical stack

```
browser (React)
   │  Connect (protobuf: requests and event stream) · WebSocket (terminals)
   ▼
api (Go) ─────────────── postgres
   │  Docker Engine API (host socket)
   ├──► copy helper container ── mounts the project path read-only
   ├──► side A container ──┐
   └──► side B container ──┴──► inference proxy (inside api, :4701)
                                     ├──► OpenAI (phase 1)
                                     ├──► Anthropic (phase 2)
                                     └──► local server on the host: Ollama, LM Studio… (phase 2)

api ──► models.dev (model catalogue and prices, cached)
```

### Docker from a container

It works, and without nesting Docker. The pattern is called *Docker-out-of-Docker*:

1. The `api` container mounts the host daemon's socket (`/var/run/docker.sock`).
2. Through that socket, `api` asks the **same host daemon** to create containers.

Each side's containers are not born "inside" `api`: they are its **siblings** in the same Docker. The socket path on the host is configurable (`DOCKER_SOCKET`), because it is not the same in every environment (see [Platforms](#platforms-macos-windows-and-linux)).

Consequences to bear in mind:

- **Bind mount paths.** The host daemon resolves them, not `api`. That is why `api` can ask to mount `C:\Users\Jose\...` in the copy helper container even though `api` cannot see that path. Docker Desktop translates Windows paths.
- **Named volumes** (`ai-compare_artifacts`, `ai-compare_staging`). Shared by name between `api` and the containers it creates.
- **Network.** Compose creates the `ai-compare` network and the side containers join it. That way they reach the proxy as `http://api:4701`. That port is not published on the host, and the UI (`:4700`) is not exposed to the side containers.
- **Labels** on every container and image (`ai-compare.comparison=<id>`, `ai-compare.side=A`). Used for cleaning up and to reconcile state if `api` restarts.
- **Images.** Built with Docker's build API (BuildKit), using the staging volume as context.
- **Discarded: Docker-in-Docker** (a daemon inside a `privileged` container). It is slower, has its own image cache and needs more permissions.
- **Accepted risk.** Whoever controls the socket controls the host. That is why only `api` mounts it, and the UI listens only on `127.0.0.1`.

### Platforms: macOS, Windows and Linux

**macOS is the main platform**; Windows and Linux must work too. The only host requirement is Docker with Compose.

| Topic | macOS | Windows | Linux | How it is handled |
|---|---|---|---|---|
| Docker socket | Docker Desktop and OrbStack: `/var/run/docker.sock`. Colima: `~/.colima/default/docker.sock` | `/var/run/docker.sock` (Docker Desktop) | `/var/run/docker.sock` | `DOCKER_SOCKET` in `.env`, defaulting to `/var/run/docker.sock` |
| Project path | `/Users/...` shared by default; `/Volumes` and other paths must be added in Docker's settings | `C:\...`, Docker Desktop translates it | Native | Before copying, `api` checks that Docker can mount the path and, if not, explains what to share |
| Architecture | Apple Silicon (arm64) | amd64 | amd64 or arm64 | Multi-arch base images; the CLI image downloads the binary for the native architecture. Nothing pins `amd64` |
| `host.docker.internal` | Works | Works | Not available by default | `extra_hosts: host.docker.internal:host-gateway` in compose and in each side's containers |
| Mounted project permissions | No problem | No problem | The container's unprivileged user may lack read permission | The copy container runs as root with a read-only mount; the sides work on the copy |
| Copy performance | Shared folders are slower than the native disk | Same | Native | The project is copied once to a volume; agents never work on the shared folder |
| Line endings | LF | The repo forces LF with `.gitattributes`; user projects are copied as they are | LF | `core.autocrlf=false` in each side's Git baseline |
| Development scripts | `pnpm` | `pnpm` (no reliance on bash) | `pnpm` | Commands run on the host are pnpm scripts or `docker compose`; bash is never required |

**Testing:**
- **CI on Linux:** real Docker, Go and frontend tests.
- **CI on macOS and Windows:** Go and frontend tests; their runners do not offer a usable Docker.
- **Docker on macOS and Windows:** tested by hand before each release.

### Backend (Go)

Go is a good fit: the official Docker SDK is written in Go, Connect has an official Go implementation and the terminal WebSocket is simple.

| Need | Choice |
|---|---|
| HTTP | `net/http` from the standard library; Connect services are mounted as `http.Handler` |
| API contract | **Protobuf + Connect** (see [Contracts](#contracts-why-connect)). `buf generate` produces the Go server (`connect-go`) and the TypeScript client (`protobuf-es` + `connect-es`). |
| Database | `pgx` + `sqlc` (generated typed SQL queries) |
| Migrations | `goose`, embedded in the binary and applied on startup |
| Docker | Official Docker SDK for Go (containers, build, attach with TTY) |
| Terminals | WebSocket with `github.com/coder/websocket`; connects to the container's `attach` stream |
| Events | Connect server streaming (works in the browser without proxies) |
| Logs | `log/slog` in JSON |
| Configuration | Environment variables from `.env` |
| Frontend in production | The Vite build is embedded with `embed.FS`; a single binary serves everything |
| Development | `air` for hot reload; Vite with a proxy to `api` |
| Tests | `go test` and `testcontainers-go` for real Postgres and Docker |

Repository structure:

```
ai-compare/
  proto/aicompare/v1/        API contract (.proto); buf.yaml and buf.gen.yaml at the root
  backend/
    cmd/server/              main
    internal/
      gen/                   Go code generated by buf (committed)
      rpc/                   Connect service implementations
      terminal/              terminal WebSocket
      orchestrator/          state machine for each comparison and side
      docker/                copy, build, containers, attach
      adapters/              codex, claude, opencode: command, flags, session paths
      providers/             openai (phase 1), anthropic and openai-compatible/local (phase 2)
      proxy/                 inference proxy: forwarding, usage, limits
      catalog/               cached models.dev client: models and prices
      db/                    migrations (goose, embedded) and sqlc-generated queries
  frontend/                  Vite + React (src/gen/: TS code generated by buf, committed)
  infra/                     compose, Dockerfiles and image with the pinned CLIs
  compose.yaml               entry point for `docker compose up`
  .env.example
  package.json               development scripts (dev, gen, build, lint, test)
```

`adapters` and `providers` are kept separate so that adding Anthropic means registering one more provider, without touching the CLI adapters.

### Contracts: why Connect

**Decided: protobuf with Connect** (`connect-go`, `connect-es`, `protobuf-es`, `connect-query` and the `buf` CLI).

Project requirements:

1. **A single contract** from which both the Go server and the TypeScript client are generated.
2. **Works in the browser without extra pieces:** no proxy such as Envoy.
3. **Server-to-browser streaming** for live events.
4. **TanStack Query integration.**
5. **An active, maintained project.**

Alternatives evaluated (GitHub data checked on 3 Oct 2026):

| Option | Contract | Browser without proxy | Streaming to the browser | TanStack Query | Activity |
|---|---|---|---|---|---|
| **Connect** (`connect-go` + `connect-es` + `connect-query`) | protobuf | Yes | Yes (server streaming) | Yes, official (`connect-query`) | Very active: commits this week in all four repos; `connect-go` v2 in RC (30 Sep 2026), `connect-es` v2.2.0, `protobuf-es` v2.16.0, `buf` 11.5k stars |
| gRPC-Web | protobuf | No: needs Envoy or a Go adapter | Server streaming only | No | Active (2.1.1, Aug 2026), but with 171 open issues and the extra piece |
| Twirp | protobuf | Yes | No | No | Inactive: last release Oct 2022, last commit Aug 2024 |
| OpenAPI, contract first (`oapi-codegen` or `ogen` + `orval` or Hey API) | OpenAPI YAML | Yes | Not in the contract (SSE described by hand) | Yes, with orval or Hey API | Active (`oapi-codegen` v2.8.0, `orval` v8.39.0 this week) |
| OpenAPI, code first (Huma + orval or Hey API) | Generated from Go types | Yes | SSE with Huma's own support | Yes, with orval or Hey API | Active (Huma v2.39.1, Jul 2026) |
| TypeSpec → OpenAPI or protobuf | Its own language | Depends on the target | Depends on the target | Depends on the target | Active, but adds another layer (over 1,000 open issues) |
| GraphQL (`gqlgen` + codegen) | GraphQL schema | Yes | Subscriptions | Indirect | Active; overkill for a single-user local tool |

Ruled out from the start: **tRPC**, because it only works if the backend is TypeScript.

Why Connect:

- **It is the only option that meets all five requirements without extra pieces.** The OpenAPI alternatives come close, but event streaming is left out of the contract.
- **Maintenance:** it is developed by Buf, the team behind `buf` and `protobuf-es`, and it is part of the CNCF.
- **Debugging:** it accepts JSON as well as binary, so it can be tested with `curl`.

Points to bear in mind:

- **`connect-go` v2 is a release candidate.** Since the project is just starting, v2 is used directly to avoid a migration. If the RC causes problems, v1.21 is stable and the project itself provides a migration tool (`connect-go-v2-migrate`).
- **The terminal stays on WebSocket.** Connect does not offer bidirectional streaming in browsers.
- **Generated code is committed.** That way, building the project does not require `buf` to be installed. To regenerate it, use `pnpm gen`, which runs `buf` (with `protoc-gen-go`, `protoc-gen-connect-go` v2 and `protoc-gen-es`) and `sqlc` in a Docker image with pinned versions, so it gives the same result on macOS, Windows and Linux without installing anything on the host.

### CLI ↔ provider compatibility

Each CLI uses only the providers it speaks natively. APIs are not translated.

| CLI | OpenAI | Anthropic | Local models | Phase |
|---|---|---|---|---|
| `opencode` | Yes | Yes | Yes (OpenAI-compatible API) | 1 (OpenAI) · 2 (Anthropic and local) |
| `codex` | Yes | No | No | 2 |
| `claude` | No | Yes | No | 2 |

The form only offers valid combinations: choosing the CLI filters the providers and, with them, the models.

### Orchestration: Temporal?

**Decided: no.** Instead, a custom orchestrator with state in Postgres:

- **State machine per side:** `pending → copying → building → starting → running → verifying → finished`, plus `error`, `cancelled` and `limit_reached`. Each transition is stored in Postgres with its timestamp.
- **Timeouts** with Go's `context`.
- **On startup, `api` reconciles:** it finds containers by label, resumes those still alive and marks whatever it cannot resume as an infrastructure error.
- **`Orchestrator` interface** so it can be switched to Temporal later without touching the rest.

Why not Temporal yet:

- **Extra infrastructure and concepts.** It adds a server (plus its UI), workers and a programming model with its own rules (workflows must be deterministic).
- **It does not cover the longest piece.** A comparison is mostly an interactive container with a live terminal. Temporal cannot resume that stream after a crash; reconciliation with Docker is needed anyway.
- **Minimal volume.** One user and one or two comparisons at a time.

**When it would pay off:** with N queued repetitions per side, batch comparisons or long reports with retries (phase 2 onwards).

### Real time

- **Event stream (Connect server streaming, `EventService.Watch`):**
  - state changes;
  - live metrics, every ~2 s;
  - "comparison finished" and "report generated".

  Each event carries a sequence number; on reconnecting, the client asks for events since the last one it saw. In the frontend, each event updates or invalidates the TanStack Query cache.
- **WebSocket at `/api/comparisons/:id/sides/:side/terminal`:**
  - output and keystrokes in binary;
  - resize as a JSON control message.

  Connect does not work here: browsers do not support its bidirectional streaming, and the terminal needs to send keystrokes and receive output at the same time with low latency.
- **Recording.** Each terminal's output is recorded in asciicast v2 format for playback in the history.

### Database (PostgreSQL)

- **Main tables:**
  - `projects` (path and profile);
  - `comparisons`;
  - `sides`;
  - `side_transitions`;
  - `requests` (one row per request passing through the proxy: usage, latency, status);
  - `price_snapshots` (the models.dev `cost` for each model used, copied when the comparison starts);
  - `reports`;
  - `findings`.

  **There is no prices table:** the catalogue lives in models.dev and its local cache.
- **Artefacts outside the database.** Diffs, recordings, logs and test output go in files on the artefacts volume; the database stores the path and metadata.

### Inference proxy (Go)

LiteLLM was discarded. Its main purpose was translating between APIs, which is no longer needed because each CLI uses its provider's native API. The proxy lives inside `api`, on its own port (`:4701`) that only the side containers can see.

**How a request works:**

1. The side's CLI calls `http://api:4701/<provider>/...` with its **side token** as the API key.
2. The proxy identifies the comparison and side from the token, checks the limits and replaces the token with the real API key.
3. It forwards the request **without modifying the body** (`httputil.ReverseProxy`).
4. It copies the response to the CLI as is, including when streaming, and reads it in parallel to extract usage.
5. It stores a row in `requests` and emits the metrics event on the event stream.

**Reading usage by provider type:**

| Type | Where usage is | Phase |
|---|---|---|
| `openai` | `usage` field of the response; when streaming, in the final Responses API event or the last Chat Completions chunk | 1 |
| `anthropic` | `usage` in `message_start` and `message_delta` when streaming | 2 |
| `openai-compatible` (local) | `usage` field if the server sends it. If not, it is marked "not reported" | 2 |

**Limits:**

- **Optional.** If a side has no token or cost limit, the proxy only measures and never rejects requests for consumption.
- **Tokens and cost per side:** the proxy adds up the usage from each response. Once the limit is exceeded, it rejects subsequent requests with a clear error and the orchestrator marks the side as "limit reached". The request in progress completes; the possible overshoot is one response.
- **Timeout:** applied by the orchestrator, not the proxy.

**Security:**

- Side tokens are random and expire when the comparison ends.
- The real API keys exist only in the `api` environment.
- The proxy only accepts routes for the configured providers; it is not an open proxy.

### Local models

`opencode` supports providers with an OpenAI-compatible API. Local models come in that way: one more provider type, `openai-compatible`, with no changes to the adapters.

- **Where the model runs:** on the host, with Ollama, LM Studio, llama.cpp server or vLLM. The proxy reaches it at `http://host.docker.internal:<port>/v1`; Docker Desktop resolves that name to the host. The URL is configured in `/settings`.
- **Traffic also goes through the proxy**, even without an API key, to measure tokens and timings just as with paid providers.
- **Model list:** comes from the local server's `/v1/models` endpoint, because models.dev does not know about them.
- **Cost:** zero, marked as "local". What is compared is time, tokens per second and quality.
- **Usage when streaming:** some local servers only send usage if the request asks for it (`stream_options.include_usage`). If `opencode` does not ask for it, the proxy would have to add it. That would be the only modification to a request body, and only for this provider type. It is validated in the phase 2 spike.
- **Equal resources for each side:**
  - **Container CPU and memory:** yes, split equally. Each side container has the same quota.
  - **The GPU for local models:** cannot be split. The model does not run in the side's container but in the local server on the host (Ollama, LM Studio…), which manages the GPU. On consumer GPUs there is no way to split it into two guaranteed halves.
- **What happens in parallel depending on the combination:**

  | Sides | Shared GPU | Behaviour |
  |---|---|---|
  | Remote vs remote | No | Parallel, no problem |
  | Local vs remote | No (only one side uses the GPU) | Parallel, no problem |
  | Local vs local | Yes | Parallel by default, with a warning |

- **Local vs local in parallel:**
  - **Requirement:** both models must fit in GPU memory at the same time (in Ollama, `OLLAMA_MAX_LOADED_MODELS` ≥ 2). If they do not fit, the server loads and unloads them in turns, and timings shoot up unfairly.
  - **Before launching:** `api` asks the local server for the loaded models and their size, and warns if they do not fit together.
  - **During the run:** time to first token and tokens per second are measured on each side. The report flags the timings as "with shared GPU".
- **Sequentially, optional:** only offered when both sides are local, for those who prefer clean timings even if it takes longer. Even so, both sides' preparation (copy and build) runs in parallel; only the agent's execution waits.

### Agent images

Layers of each image:

1. **Project runtime** (profile).
2. **Pinned CLIs:** `codex`, `claude` and `opencode`.
3. **Project copy and setup.**
4. **Side layer:** CLI configuration pointing at the proxy, and Git baseline.

Notes:

- CLIs are installed as self-contained binaries when the CLI offers them. If one needs Node.js and the project runtime does not include it (for example, `python:3.12`), Node is copied from a multi-stage build stage.
- The CLI layer is cached per runtime.
- Non-root user, with `git` and `ripgrep` installed.

### Frontend

| Need | Choice |
|---|---|
| Base | Vite + React + TypeScript + Tailwind + shadcn/ui |
| Server state | **TanStack Query** with `@connectrpc/connect-query`: history, model and price catalogue, and comparison state, updated by the event stream. |
| Routing | TanStack Router: typed routes and history filters in the URL |
| API client | `@connectrpc/connect-web` with the code generated from the contract; the terminal uses a separate WebSocket |
| Forms | react-hook-form + zod (shadcn's Form component) |
| Terminal | `@xterm/xterm` + `@xterm/addon-fit` |
| Diffs | `react-diff-view` |
| Markdown | `react-markdown` + `remark-gfm` |
| Charts | shadcn chart components (Recharts) |
| Tests | Vitest + Testing Library; Playwright for the end-to-end flow at the end of phase 1 |

### Docker Compose services

| Service | Image | Port | Volumes |
|---|---|---|---|
| `api` | local build (Go + embedded frontend) | `127.0.0.1:4700` | Docker socket, artefacts, staging |
| `postgres` | `postgres:17` | internal | data |

`api` publishes `:4700` (UI and API) only on `127.0.0.1`. Its second port, `:4701` (inference proxy), is reachable only from the `ai-compare` network.

The copy and side containers are created dynamically by `api`; they are not in the compose file.

## Scope by phase

### Phase 0 — Stack and spike

**Skeleton:**

- `docker-compose.yml` with `api` and `postgres`.
- Go server with a health check and migrations.
- Vite frontend with shadcn and the navigation bar.
- Protobuf contract with `buf generate` → Go and TypeScript (generated code committed), plus sqlc.
- pnpm scripts for `dev`, `gen`, `build`, `lint` and `test`; `docker compose up` for the stack.

**Spike.** Validate on Windows with Docker Desktop:

1. From `api`, create the helper container that mounts a Windows path and copies the project to staging.
2. Build the side image from staging.
3. Start the container on the `ai-compare` network and check that it reaches the proxy (`api:4701`) and nothing else in the stack.
4. `attach` with a TTY and relay it over WebSocket to a minimal page with xterm.js. Typing, resizing and Ctrl+C must work.
5. Launch `opencode` with an OpenAI model through the proxy, in interactive and autonomous mode.
6. Extract the usage of each response in the proxy, with and without streaming, and calculate the cost with the models.dev price.
7. Check that `opencode` can point at `host.docker.internal`, as a preview of local models.

**Exit criterion:** all seven points work, or we know exactly which one does not and what alternative to use.


**Results (3 Oct 2026, Windows 11 + Docker Desktop):**

- **Points 1 and 2: done.** `docker compose exec api /app/spike prepare <path>` copies the project to the staging volume and builds the image for side A.
  - 76-file Git repo: copy in 1.7 s and build in 5 s with cache (27 s the first time).
  - Respects `.gitignore` and uncommitted changes; excludes `.git` and `.env` files (keeps `.env.example`).
  - The side image has a clean `baseline` commit with `core.autocrlf=false`.
- **Finding:** Docker Desktop on Windows **creates on the host** a folder that does not exist if it is mounted as a bind. That is why the project folder is not mounted: its top-level folder (`C:`, `/Users`, `/home`…) is mounted read-only and the copy helper container checks that the path exists. A non-existent path gives a clear error and creates nothing.
- **Classic builder:** the build API without BuildKit works with Docker 29 for tar contexts; whether BuildKit is needed will be reviewed later.
- **Point 4: done.** Hidden page `/spike/terminal`: each connection creates a disposable container with `bash` and a TTY, relays it over WebSocket to xterm.js and deletes it on disconnect.
  - Typing, resizing (`tput cols/lines` matches the window) and control keys work.
  - Ctrl+C interrupts the process (`sleep` exits with code 130).
  - Only same-origin WebSockets are accepted.
- **Point 3: done.** Agent containers run on their own network, `ai-compare-agents`, which only `api` is on. Checked from a container on that network:
  - the proxy (`api:4701`) responds;
  - the UI API (`api:4700`) returns 403;
  - `postgres` does not even resolve;
  - there is internet access.

  From the host, the UI still works and the proxy is not published.
- **Point 6: done.** Tested with real OpenAI (`gpt-5.4-nano`): a normal request (13 + 4 tokens, $0.0000076) and a streaming one through the Responses API (19 + 13 tokens, $0.00002005), both recorded with their cost from models.dev. If the provider returns an error (for example, an account with no credit), the proxy records the message, including when it arrives inside a stream with HTTP 200.
  - **Sessions:** each side has a session with its own token, which the proxy swaps for the real key.
  - **Forwarding:** forwards unmodified, streaming included.
  - **Usage:** read from Chat Completions, Responses and Anthropic Messages.
  - **Cost:** calculated with the models.dev price snapshot, including the long-context tier.
  - **Limits:** once exceeded, requests are rejected with 403 so the CLI does not retry.
  - **Real test:** `docker compose exec api /app/spike proxy-check`, with `OPENAI_API_KEY` in the `.env`.
- **Point 5: done, and wired to the UI.** When **Run comparison** is pressed, the real backend:
  1. copies the project;
  2. builds one image per side with `opencode` 1.18.34, pointed at the proxy with the side token;
  3. starts the containers on the agents network.

  Behaviour:
  - **Interactive:** the TUI opens with the prompt already sent and the conversation can continue from the browser terminal.
  - **Autonomous:** uses `opencode run --auto` and finishes on its own.
  - **Run screen:** shows the two real terminals, with live status, tokens, cost and tokens/s from the proxy.
  - **Buttons:** Finish and Cancel stop the container.
  - **Tested** with `gpt-5.4-nano` against `gpt-5.4-mini` on a sample project: both sides completed the task.
  - **First build:** about 55 s, because it installs `opencode`; later builds use the cache.
- **Point 7: done.** With a test OpenAI-compatible server on host port 11434:
  - from the agents network, `api:4701/local/v1/chat/completions` reaches the host via `host.docker.internal`, with and without streaming;
  - the proxy records the usage of both requests and does not forward the side token;
  - the cost is left empty, because local models have no price on models.dev.
- **Skeleton complete:**
  - **Postgres:** goose migrations embedded in the binary and applied on startup; queries generated with sqlc. The health check reports the database status.
  - **Comparison persistence:** each side is saved on every status change and when it finishes, with the proxy requests, logs and terminal output. The history, the terminal (read-only) and the zip download survive an `api` restart. A side that was running during a restart is closed as an error and its container is stopped.
  - **Contract:** `CatalogService` in protobuf, served with `connect-go` v2 (RC) under `/api/rpc`. The UI uses it with the generated `connect-es` client (reads go via GET) and `spike proxy-check` with the generated Go client. The remaining JSON routes move to Connect service by service in phase 1; `connect-query` comes in when the pages are migrated.
  - **Scripts:** `pnpm gen` (buf and sqlc in Docker) and `pnpm test` (Go tests in Docker).
- **Still to be tested on macOS:** mounting `/Users`, the error when the path is not shared with Docker, and the terminal.

**Phase 0 complete**, except for the macOS test.
### Phase 1 — Comparing OpenAI models with the project's harness

**Scope:**

- **CLI:** only **`opencode`**, on both sides. It is the only one that talks to both OpenAI and Anthropic, so phase 2 only adds a provider. The CLI selector exists in the UI but only offers `opencode`; the `codex` and `claude` adapters arrive in phase 2.
- **Provider:** only **OpenAI**. Models are offered from a provider registry, ready to add Anthropic without changing the adapters.
- **Harness:** each side uses **the harness the project already has**, copied as is. If the project has none, it runs without one. There are no presets and no `/harnesses` page. Since both sides use the same CLI and the same copy, they receive exactly the same instructions. The UI lists the detected harness files and shows which ones `opencode` reads.
- **Chosen per side:**
  - OpenAI model;
  - effort;
  - mode (interactive or autonomous).

  What this phase compares is **model against model**, or the same model with a different effort or mode.
- **The solution diff still excludes harness files**, so that changes to `AGENTS.md` are not mixed with the code.
- **The project is optional.** Without a project folder, both sides start from an empty `/workspace`, for quick tests without preparing a folder.
- **Folder browser.** Next to the path input, **Browse…** opens a dialog that navigates the host's folders and fills in the path when one is selected. Browsers never reveal a folder's absolute path, so the listing comes from `api` through the copy helper (read-only mount of the top-level folder). Only folders Docker can see are browsable (on Windows, the drives Docker Desktop shares); typing the path still works.

**Phase 1 work plan, in order** (phase 0 already delivered launching, terminals, Finish/Cancel, live cost and limits, `/pricing`, price snapshots, persistence and zip downloads):

1. **Foundations.**
   - [ ] Move the JSON routes to Connect (`ComparisonService`, `ProjectService`).
   - [ ] `EventService.Watch` server stream instead of polling; adopt `connect-query`.
   - [ ] Reattach to live containers after an `api` restart and complete the recording with `docker logs --since`.
   - [ ] Non-root user in agent containers.
   - [x] Optional project and the folder browser on the New comparison page (`ProjectService` on Connect).
2. **Changes tab.**
   - [ ] Solution diff against `baseline`, excluding harness files; harness diff in its own tab.
   - [ ] Files and lines changed in the metrics.
3. **Verification.**
   - [ ] Run the profile's test command in a fresh container from the side's result.
   - [ ] Optional hidden tests copied only for verification.
   - [ ] Real Tests tab.
4. **Report.**
   - [ ] Blind reviewer, per-side analyst and comparative judge, with `gpt-6-luna` as the default model (configurable).
   - [ ] Per-side stages start as soon as that side ends; report cost recorded separately; warning when the judge is one of the compared models.
   - [ ] Real report page.
5. **History.**
   - [ ] Timed terminal recording (asciicast v2).
   - [ ] Events tab from opencode's session files, also used to cross-check the proxy's usage.
   - [ ] Human wait time in interactive mode; infrastructure errors recorded apart from agent errors.
   - [ ] Each side's result saved as an artefact when it ends, so A and/or B can be downloaded from the history after retention has removed the containers.
6. **Settings and retention.**
   - [ ] Real `/settings`: defaults, report model, automatic report, retention, suggested limits.
   - [ ] Retention: after **2 days** by default, remove stopped containers and images **created by ai-compare only** (selected by the `ai-compare.*` labels and the `ai-compare/` image prefix; never anything else on the user's Docker) and the staging copies. Artefacts and reports are always kept.
   - [ ] Remove the mocks of everything that is real and default `VITE_USE_MOCKS` to `false`.

**Phase 1a — Launch and watch:**

- `/`: path with its profile, prompt and two sides.
- Copy, layers and baseline.
- Side-by-side panels with Terminal and Logs, in interactive and autonomous mode.
- Finish and cancel each side.
- Reconnection: closing and reopening the tab restores the terminals and the status; `api` restart with reattachment to the containers.

**Phase 1b — Measure and compare:**

- Changes and Metrics tabs, with live cost and tokens and budget limits.
- Read-only `/pricing` with the OpenAI models from models.dev, and a price snapshot per comparison.
- `/history` and `/history/:id` with terminal recording, diffs and tests.
- Verification with tests in a fresh container.
- Report: blind reviewer, analyst and judge.
- `/settings`.

### Phase 2

- Anthropic provider for `opencode`.
- Local models for `opencode` (`openai-compatible` provider), in parallel by default, with a shared-GPU warning and a sequential option.
- `claude` (Anthropic only) and `codex` (OpenAI only) CLIs. With them comes the warning that each CLI reads different harness files (`CLAUDE.md` and `.claude/` versus `AGENTS.md`).
- Presets: `/harnesses` page, a **No harness** option and exclusion of the project's harness.
- Application previews: subdomain proxy (`a-<id>.localhost`) and relaunching from stopped containers.
- N repetitions per side with aggregates and the cost versus quality chart.
- Preset adviser: which differences between presets may have had an influence and what to change.
- Preset cards as an input mechanism with translation between CLIs.

## Assumptions to validate in the spike

- `api` can create sibling containers through the socket on Docker Desktop for Windows, and mount Windows paths in the copy container.
- `opencode` accepts a custom base URL for OpenAI, so that all its traffic goes through the proxy. The same for `codex` and `claude` in phase 2.
- The proxy forwards the API that `opencode` uses with OpenAI unaltered, streaming included, and extracts the usage of each response.
- The models.dev model IDs match the ones `opencode` accepts for OpenAI.
- All three CLIs start their TUI with an initial prompt, or tolerate typing into the PTY.
- Each CLI's session files are readable and useful for extracting events.
- Runtime and setup detection covers my usual projects.
- Building the layers is fast enough with cache not to get in the way of daily use.

## Open decisions

- **Subscriptions.** Claude Pro/Max or ChatGPT instead of an API key. The proxy does not apply in the same way and the cost is not per token. Proposal: out of the MVP, or supported with a "not applicable" cost and tokens read from the CLI sessions.
- **Specific OpenAI models** for phase 1. By default, the ones models.dev lists for `openai`.
- **Reference local server** for phase 2: Ollama or LM Studio.
- **Export and backup** of presets and history: for now, copying the `harnesses/` volume is enough.

**Decided:**

- **Contracts:** protobuf with Connect (`connect-go` v2, `connect-es`, `connect-query`, `buf`). The terminal goes over WebSocket.
- **Platforms:** macOS as the main one, plus Windows and Linux; the only host requirement is Docker with Compose.
- **Temporal:** out. Own orchestrator with state in Postgres, behind an interface.
- **CLIs per provider:** `claude` only with Anthropic, `codex` only with OpenAI and `opencode` with both and with local models. Phase 1 only with `opencode` and OpenAI.
- **Gateway:** own Go proxy inside `api`. LiteLLM ruled out.
- **Pricing:** no table of our own. models.dev is the source, with a local cache; each comparison stores a snapshot of the prices it used.
- **Report model:** `gpt-6-luna` by default, configurable in `/settings`.
- **Suggested limits** when enabling each one: 30 min, 2M tokens and $2 per side.
- **Retention:** 2 days by default for containers, images and staging copies created by ai-compare (never other Docker objects); artefacts and reports are kept.
- **UI session token:** not for now. The app is local, single-user and bound to `127.0.0.1`; it will be added if the app is ever exposed on a network.

## Out of scope

- Pushing, merging or publishing the generated code. The copy has no remotes.
- Modifying the original project or the host CLIs' global configuration.
- Multi-user, accounts, authentication and hosted deployment.
- Bulk comparisons of many models at once.
- Claiming causality between a preset rule and the result from a single run per side.
