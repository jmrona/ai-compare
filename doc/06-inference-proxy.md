# 6. Inference proxy

Code: `backend/internal/proxy/proxy.go` (sessions, forwarding, cost, limits) and `usage.go` (reading usage from responses). Tests: `proxy_test.go`.

## Why a proxy

Every model request an agent makes goes through `api` on port 4701 instead of straight to the provider. That single choke point gives:

1. **Per-side measurement in every mode.** Tokens, cost, latency and errors per request, even in interactive mode where the CLI's own reporting is not available.
2. **No API keys in agent containers.** The agent gets a random side token; only `api` has the real key. An agent cannot leak a key it never sees.
3. **Limits.** Token and cost budgets per side are enforced on the wire.
4. **One place for every provider.** OpenAI today, Anthropic and local servers later, without translating APIs.

LiteLLM was considered and rejected; see [Decisions](17-decisions.md).

## URLs and providers

```
http://api:4701/<provider>/<provider path>
```

| Provider | Upstream base | Auth header set by the proxy | Example |
|---|---|---|---|
| `openai` | `https://api.openai.com` | `Authorization: Bearer <OPENAI_API_KEY>` | `/openai/v1/responses` |
| `anthropic` | `https://api.anthropic.com` | `x-api-key: <ANTHROPIC_API_KEY>` | `/anthropic/v1/messages` |
| `local` | `LOCAL_MODELS_BASE_URL` without `/v1` (default `http://host.docker.internal:11434`) | none | `/local/v1/chat/completions` |

opencode is configured with `baseURL = http://api:4701/openai/v1`, so it calls `/openai/v1/responses` and never knows it is not talking to OpenAI. `GET /healthz` answers `ok`.

## Sessions and tokens

`NewSession(id, provider, model, price, longContext, limits)` creates a session and returns a token `aic_` + 48 hex characters. The orchestrator creates one per side, passes the token to the container as `OPENAI_API_KEY`, and calls `EndSession` when the side ends, which revokes the token.

For each request, the proxy:

1. reads the token from `x-api-key` or `Authorization: Bearer`;
2. rejects unknown tokens with **401** `invalid_api_key`;
3. rejects a token used for another provider with **403** `wrong_provider`;
4. rejects requests once a limit is reached with **403** `limit_reached`. **403 rather than 429**, because CLIs retry 429s and would loop;
5. reads the `model` field of a JSON body (and puts the body back untouched) to record which model was actually asked for;
6. forwards with `httputil.ReverseProxy`: same method, path, query and body; the auth headers are replaced with the real key; `Accept-Encoding` is removed so the response arrives uncompressed and its usage can be read; `FlushInterval: -1` sends every chunk to the client as soon as it arrives, which keeps streaming smooth;
7. wraps the response body in a **meter** that the client reads unchanged while usage is extracted on the side.

Errors from the proxy itself use the shape both OpenAI and Anthropic clients print: `{"type":"error","error":{"type":"…","code":"…","message":"ai-compare proxy: …"}}`.

## Reading usage

Usage is normalised to four fields that add up to the total, the way providers bill:

| Field | Meaning |
|---|---|
| `input` | Prompt tokens **not** served from cache |
| `cacheRead` | Prompt tokens served from cache |
| `cacheWrite` | Prompt tokens written to cache (Anthropic) |
| `output` | Generated tokens, reasoning included |
| `reported` | `false` when the response had no usage; the UI then shows "not reported", never 0 |

| API | Where usage is | Notes |
|---|---|---|
| OpenAI Chat Completions | `usage.prompt_tokens` (includes cached), `completion_tokens`, `prompt_tokens_details.cached_tokens` | When streaming, in the last chunk if the client asks for it |
| OpenAI Responses | `usage.input_tokens` (includes cached), `output_tokens`, `input_tokens_details.cached_tokens` | When streaming, in the `response.completed`, `response.incomplete` or `response.failed` event |
| Anthropic Messages | `input_tokens` (excludes cache), `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens` | When streaming, spread over `message_start` and `message_delta` |

- **JSON responses:** up to 32 MB of the body is kept and parsed when it ends.
- **SSE responses** (`text/event-stream`): each `data:` line is parsed as it passes; usage accumulates.
- **Errors** are recorded too: the provider's error message from a JSON body, and errors that arrive **inside an SSE stream with HTTP 200** (OpenAI sends `error` and `response.failed` events this way, for example when the account has no credit). Network errors to the provider produce a 502 for the client and a recorded request.

Each request is recorded as `{at, method, path, model, status, streamed, durationSec, usage, costUsd, error}` in its session, and the session's totals update.

## Cost

```
cost = (input × price.input
      + cacheRead × (price.cacheRead ?? price.input)
      + cacheWrite × (price.cacheWrite ?? 0)
      + output × price.output) / 1 000 000
```

- Prices are USD per million tokens from the **snapshot** taken when the side started (see [Models and pricing](09-models-and-pricing.md)), so later price changes on models.dev never alter a past comparison.
- **Long-context tier:** when a request's prompt (`input + cacheRead + cacheWrite`) exceeds the model's threshold (for example 272,000 tokens), the whole request is priced with the long-context rates.
- A model with no price on models.dev gives `costUsd: null` ("cannot be calculated"), never 0. Local models are in this case today.

## Limits

| Limit | Enforced by | Behaviour |
|---|---|---|
| Max tokens (in thousands) | Proxy | After the session total reaches it, further requests get 403. The request in flight completes, so the overshoot is at most one response |
| Max cost (USD) | Proxy | Same, on the session cost |
| Timeout (minutes) | Orchestrator | The run context times out and the container is stopped |

All limits are **optional**. Without them the proxy only measures. When a limit is hit, the orchestrator's watcher (every 2 s) marks the side `limit_reached` with the reason and stops it.

## What the proxy does not do

- It does not modify request bodies. (Local servers that only report streaming usage when `stream_options.include_usage` is set may need that single change in phase 2.)
- It is not an open proxy: only the configured providers exist, and only with a live side token.
- It is not published on the host: only containers on Docker networks shared with `api` can reach port 4701.

## Checking it

```bash
docker compose exec api /app/spike proxy-check
```

From a container on the agent network, it checks that the proxy is reachable, that the app port refuses it with 403, that `postgres` does not resolve and that the internet is reachable. With `OPENAI_API_KEY` set, it then makes a real non-streamed Chat Completions call and a streamed Responses call with the cheapest current OpenAI model and prints what the proxy recorded, cost included. On Git Bash for Windows, prefix it with `MSYS_NO_PATHCONV=1`.
