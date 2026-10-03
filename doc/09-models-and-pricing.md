# 9. Models and pricing

Code: `backend/internal/catalog/catalog.go`, served by `internal/rpc/catalog.go`. Frontend: `src/api/queries.ts` and `convert.ts`, `src/lib/catalog.ts`, `src/pages/PricingPage.tsx`, `src/components/compare/SideForm.tsx`. The report model: `backend/internal/report/`.

## One source: models.dev

[models.dev](https://models.dev) is an open, community-maintained catalogue of models with their capabilities, limits, release dates and prices. ai-compare uses it as its **only** source of model lists and prices. There is no price table in the database and nothing to maintain by hand.

models.dev publishes everything in a single file, `https://models.dev/api.json` (about 5.3 MB, about 380 KB compressed). There is no per-provider endpoint, so the whole file is downloaded and filtered.

## Fetching and caching

- `Get` returns the catalogue from memory, refreshing it first when it is older than **24 hours**.
- `Refresh` (the Pricing page's "Check for updates") asks models.dev now.
- Requests send `If-None-Match` with the last `ETag`. A **304** only updates `fetchedAt`, so a check costs almost nothing.
- The catalogue and its ETag are saved to `DATA_DIR/catalog.json` (volume `appdata`), written atomically through a temporary file. On startup the saved copy is loaded, so the app works offline.
- If models.dev cannot be reached, the saved copy is returned with `fromCache: true` and a warning ("models.dev could not be reached; showing the copy saved on …"). With no saved copy, the call fails.

## Filtering and shape

`Parse` keeps only providers listed in `CATALOG_PROVIDERS` (default `openai,anthropic`) and sorts models by `release_date`, **newest first** (then by id), so the dropdowns show the latest models at the top.

Each model becomes:

| Field | From models.dev | Use |
|---|---|---|
| `id`, `name`, `provider`, `family` | id, name, provider key, family | `id` is what opencode receives (`openai/<id>`) |
| `releaseDate` | `release_date` | Sorting, shown in the dropdown |
| `deprecated` | `status == "deprecated"` | Hidden by default |
| `toolCall`, `textOutput` | `tool_call`, `"text"` in `modalities.output` | Coding agents need both; models without them are hidden by default |
| `efforts` | `reasoning_options` entry of type `effort` | The effort dropdown per model (`none`, `low`, `medium`, `high`, `xhigh`, `max`…); empty means no effort setting |
| `contextK` | `limit.context / 1000` | Shown |
| `price` | `cost.input`, `cache_read`, `cache_write`, `output` (USD per million tokens) | Cost calculation; `null` when input or output price is missing |
| `longContext` | the first `cost.tiers` entry of type `context` (its `size` is the threshold) | Higher rates once the prompt exceeds N tokens (for example 272,000) |

## Price snapshots

When a side starts, its model's price and long-context tier are **copied into the side** (`priceSnapshot` with the catalogue's `fetchedAt`). The proxy prices every request with that copy. A comparison therefore always keeps the prices it ran with, even if models.dev changes them later, and the Metrics tab says "Price: models.dev · snapshot from …".

## In the UI

- **Pricing page** (`/pricing`): read-only table per provider (OpenAI, Anthropic, Local · phase 2) with input, cached input, cache write, output and context, plus a "prompts over N tokens" row for long-context rates. Toggles show deprecated models and models agents cannot use.
- **New comparison form:** the model dropdown lists usable models newest first, with price and release date in fixed-width columns; the effort dropdown shows the efforts that model accepts. Both sides default to the newest models.
- **Formatting:** prices in en-GB format; costs under one cent are shown with two significant digits instead of rounding to $0.00.

## The report model

The report is written by one model chosen in `/settings` from the usable OpenAI models in the catalogue; the default is **`gpt-6-luna`**. Calls use the OpenAI Responses API through the inference proxy (see [Inference proxy](06-inference-proxy.md#report-sessions)), with **strict JSON schema** structured outputs, so each answer is parsed into fixed fields. Reasoning effort is set to `low` when the model accepts it, which is enough for reviewing and summarising and keeps the report cheap. The report's cost is priced with the same catalogue and shown apart from the comparison's.

| Stage | Runs | Receives | Produces |
|---|---|---|---|
| **Blind reviewer** | Once per side | The task, a one-line test result, the files changed and the solution diff (cut at 120,000 characters). Nothing about the model, the CLI or which side it is | Findings with severity (`high`, `medium`, `low`), title, impact and location (`path:line`); possibly none |
| **Analyst** | Once per side | The task, the side's configuration (CLI, model, effort, mode) and facts (status, end reason, failure kind, time, requests, tokens, cost, tests, changed files), up to 150 events from the CLI session, the warnings and errors in its logs, and the end of the test output | At most five paragraphs on what the agent did, how it went and the outcome |
| **Judge** | Once, when both sides have ended | The task, and for each side its configuration, facts, findings and analysis | Verdicts `Cheaper`, `Faster`, `Fewer problems`, `Overall` (and `Tests` when tests ran), each `A`, `B` or a tie; two to five paragraphs of conclusions |

With **automatic reports** on (off by default), the reviewer and analyst of a side run as soon as that side ends, so only the judge is left when the second side ends. Otherwise all stages run when **Generate report** is pressed, the two sides' stages in parallel.

Every report carries **warnings**: always that there was one run per side; when the report model is also one of the sides' models (it may favour its own work); and when a side was interactive.

## Local models (phase 2)

models.dev does not know local models. Their list will come from the local server's `/v1/models`, their cost is `null` ("local"), and what is compared is time, tokens per second and quality. The proxy route (`/local/…` → `host.docker.internal`) already works; see [Inference proxy](06-inference-proxy.md).
