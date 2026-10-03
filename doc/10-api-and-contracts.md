# 10. API and contracts

The browser talks to `api` (port 4700) in three ways:

| Channel | Used for | Details |
|---|---|---|
| **Connect** (protobuf contract) under `/api/rpc/` | Every request and response, and the live event stream (a server stream) | Six services, below |
| **WebSocket** | Terminals | See [Terminals](07-terminals.md) |
| **Plain HTTP GET** | Files: a side's zip download and its terminal recording; the health check | See [Plain HTTP routes](#plain-http-routes) |

## The contract

`proto/aicompare/v1/*.proto` is the single source of truth. `buf` generates:

| Output | Plugin | Location |
|---|---|---|
| Go messages | `protoc-gen-go` (google.golang.org/protobuf v1.36) | `backend/internal/gen/aicompare/v1/*.pb.go` |
| Go Connect handlers and clients | `protoc-gen-connect-go` (connect-go **v2**, release candidate) | `backend/internal/gen/aicompare/v1/aicomparev1connect/` |
| TypeScript messages and service descriptors | `protoc-gen-es` (protobuf-es v2) | `frontend/src/gen/aicompare/v1/*_pb.ts` |

The generated code is **committed**, so building the project never needs `buf`. To change the API:

1. edit the `.proto` file;
2. run `pnpm gen` (it runs `buf lint`, `buf generate` and `sqlc generate` in a pinned Docker image);
3. implement the new methods in `backend/internal/rpc/` (the generated interface will not compile until you do);
4. use them from the frontend: a query hook in `frontend/src/api/queries.ts` (connect-query) or a mutation through the clients in `transport.ts`, and the mapping to UI types in `convert.ts`.

`buf.yaml` enables the `STANDARD` lint rules and `FILE` breaking-change detection.

Methods without side effects are marked `idempotency_level = NO_SIDE_EFFECTS`, which lets the browser call them with **HTTP GET** (`useHttpGet: true` in the transport): cacheable and easy to read in DevTools.

## Services

| Service | Method | GET | What it does |
|---|---|---|---|
| **CatalogService** (`catalog.proto`) | `GetCatalog` | Yes | The models.dev catalogue, filtered and sorted (see [Models and pricing](09-models-and-pricing.md)) |
| | `RefreshCatalog` | | Asks models.dev now (with ETag) |
| **ProjectService** (`project.proto`) | `InspectProject` | Yes | What a comparison would copy from a folder: files, size, Git, harness files and the CLIs that read them, excluded `.env` files, proposed profile, and the other entries at the root (`otherEntries`) a preset can import |
| | `ListFolders` | Yes | Sub-folders of a host folder for the folder browser; an empty path starts at the user's home folder |
| **ComparisonService** (`comparison.proto`) | `StartComparison` | | Validates, creates the comparison and starts it; returns its id at once (body in [Comparison lifecycle](04-comparison-lifecycle.md#starting)) |
| | `GetComparison` | Yes | One comparison with both sides: config, status, end reason, failure kind, metrics, changed files, tests, price snapshot, whether a result and a recording exist, report status |
| | `ListComparisons` | Yes | Every comparison, newest first |
| | `GetActiveComparison` | Yes | The newest comparison with a side not yet ended, or nothing |
| | `FinishSide`, `CancelSide` | | End a side as finished or cancelled (at once if it is still being prepared) |
| | `DeleteComparison` | | Remove a comparison that is not live: Docker objects, artefacts, database rows |
| | `GetLogs` | Yes | Orchestrator notes (sources `copy`, `build`, `run`, `verify`) merged with one line per proxied request |
| | `GetHarness` | Yes | The harness files a side ran with (its preset snapshot, or the project's own harness files as copied), each with its root (`project` or `home`), size and content (empty and `omitted` when binary or over 512 KB). `available` is false for an older comparison whose project copy is gone |
| | `GetDiff` | Yes | `kind` `solution` (default, harness files excluded) or `harness`: files with lines added and removed, and the diff as typed lines (`+`, `-`, context, `@@`, `file`). Live while the side runs, saved once it has ended; `ready` is false when there is nothing to read yet |
| | `GetTests` | Yes | The test command, the visible and hidden runs (status, exit code, duration) or why they were skipped, and both outputs |
| | `StopSeries` | | Keeps the attempts of a series that have not started from running |
| | `StartPreview`, `StopPreview` | | Serve a side's files, or start its preview command in a container from its result image; see [Previews](04-comparison-lifecycle.md#previews) |
| | `GetPreview` | Yes | The preview's status (`stopped`, `starting`, `running`, `error`), kind (`static` or `command`), URL, error and the end of its output |
| | `GetTimeline` | Yes | Events from the CLI session (`prompt`, `message`, `tool`, `patch`, `error`) and the session's own tokens and cost; `ready` once the side has ended |
| **EventService** (`events.proto`) | `Watch` | | Server stream of changes; see [The event stream](#the-event-stream) |
| **ReportService** (`report.proto`) | `GenerateReport` | | Starts (or restarts) the report once both sides have ended; progress arrives through the event stream |
| | `GetReport` | Yes | Status, error, model, cost, verdicts, conclusions, analysis of A and B, findings, warnings, and the harness advice when the sides' harnesses differ |
| **PresetService** (`preset.proto`) | `ListPresets`, `GetPreset` | Yes | Presets with their files (root, path, size, category), hash, last edit and how many sides used them |
| | `CreatePreset`, `UpdatePreset`, `DuplicatePreset`, `DeletePreset` | | Manage presets; the slug comes from the title and stays |
| | `GetPresetFile` | Yes | A file's content (bytes) |
| | `WritePresetFile`, `DeletePresetFile`, `MovePresetFile` | | Edit files; writing returns warnings for values that look like secrets |
| | `ImportFromProject` | | Copies chosen harness files of a host project into the preset's `project/` |
| **SettingsService** (`settings.proto`) | `GetSettings` | Yes | Editable settings plus read-only facts (see below) |
| | `UpdateSettings` | | Validates and saves the editable fields; read-only fields are ignored |
| | `CleanUp` | | Applies the retention rule now (what `retention` selects, from comparisons ended more than `retention_days` ago); returns how many comparisons, containers, images, project copies and artefact folders were removed |

**Settings.** Editable: report model, automatic report, default limits, CPUs and memory per side, retention days (0 to 365) and what retention removes (containers, images, project copies, artefacts). Read-only, added by the rpc layer: which provider keys are set, the suggested limits (30 min, 2,000k tokens, $2), `LOCAL_MODELS_BASE_URL`, CLI versions (opencode's pinned version and the latest on the npm registry, read at most once an hour), and disk use (images, artefacts, project copies).

### Server side

`internal/rpc.Handler` creates a `connect.Server`, registers the services and mounts them with `connecthttp.Mount` on a mux, which `main.go` serves under `/api/rpc/` with the prefix stripped. A method's full path is therefore:

```
/api/rpc/aicompare.v1.CatalogService/GetCatalog
```

`CatalogService` and `SettingsService` are always registered; the others need Docker and are left out when it is unreachable. The rpc layer converts the Go types of `internal/comparison`, `internal/report` and `internal/settings` to protobuf messages. Errors are Connect errors with a code: `not_found` for an unknown comparison, `invalid_argument` for bad input (a wrong, missing or unshared path; invalid settings), `failed_precondition` for an action that does not fit the state (deleting a live comparison, a report before both sides ended), `unavailable` when Docker or models.dev cannot be reached.

### Client side

`frontend/src/api/transport.ts` creates the Connect transport (`@connectrpc/connect-web`, base URL `${VITE_API_BASE_URL}/rpc`, `useHttpGet: true`) and one client per service. `main.tsx` wraps the app in connect-query's `TransportProvider`.

- **Queries** use `@connectrpc/connect-query` (`useQuery(ComparisonService.method.getComparison, { id })`), so cache keys come from the generated method descriptors and the event stream can update exactly the right entries. `select` maps protobuf messages to the plain UI types in `types.ts` (functions in `convert.ts`: `Timestamp` → ISO string, unset optional → `null`…).
- **Mutations** (start, finish, cancel, delete, generate report, update settings, clean up, refresh catalogue) call the clients through TanStack Query's `useMutation`.
- **Nothing polls.** See [Frontend](12-frontend.md#data-layer).

The Go side has a generated client too; `spike proxy-check` uses it to read the catalogue.

### Trying it with curl

Connect speaks JSON as well as binary:

```bash
curl -s "http://127.0.0.1:4700/api/rpc/aicompare.v1.CatalogService/GetCatalog?connect=v1&encoding=json&message=%7B%7D"
```

```bash
curl -s -X POST -H "Content-Type: application/json" -d '{"id":"ra3f80e","side":"A"}' http://127.0.0.1:4700/api/rpc/aicompare.v1.ComparisonService/GetLogs
```

## The event stream

`EventService.Watch` is a Connect **server stream**. Each `WatchResponse` carries a sequence number and one of:

- `comparison`: a comparison as it is now (the same message as `GetComparison`);
- `deleted_id`: the id of a comparison that was deleted;
- nothing: a **heartbeat**.

What the server sends:

1. **An empty message as soon as the stream opens.** With connect-go v2 RC1, `SendHeaders` does not flush on the server, so without a first message a client would not know the stream is open until something changed. The browser treats this first message as "connected".
2. **Every comparison that is still live**, as it is now.
3. **Every change** after that. Changes are **coalesced**: a publisher collects the comparisons that changed and sends each once every 250 ms. Comparisons with a side not yet ended are republished every second, so timers and live metrics move.
4. **An empty heartbeat every 20 s**, so idle connections are not closed along the way.

The server subscribes before taking the snapshot of live comparisons, so nothing between the two is lost. A subscriber that falls behind (its buffer of 1,024 events is full) is dropped: the stream ends with `unavailable`, and the client reconnects.

There is no replay by sequence number. A client that reconnects calls `Watch` again, receives the live comparisons first, and refetches whatever else it has cached (the browser invalidates every connect-query query on reconnect).

## Plain HTTP routes

| Method and path | Purpose |
|---|---|
| `GET /api/health` | JSON: `{"status":"ok","database":"ok","providers":{"openai":true,"anthropic":false}}` |
| `GET /api/comparisons/{id}/sides/{side}/terminal` | WebSocket: the side's terminal (see [Terminals](07-terminals.md)) |
| `GET /api/comparisons/{id}/sides/{side}/download` | Zip of the side's files, named `<model>-<id>-<side>.zip`: from the saved artefact `workspace.tar` once the side has ended, from the running container before |
| `GET /api/comparisons/{id}/sides/{side}/recording` | The side's terminal recording (asciicast v2, `application/x-asciicast`) |
| `POST /api/spike/proxy/sessions`, `GET …/{id}` | Phase 0 tools: create and inspect proxy sessions by hand |
| `GET /api/spike/terminal?image=` | Phase 0 WebSocket: throwaway bash container |

Errors on these routes are `{"error": "message"}` with a suitable status (404 for an unknown comparison, side, or missing file). Unknown `/api/` paths get a JSON 404 instead of the SPA's `index.html`.

These stay outside Connect on purpose: a terminal needs input and output at the same time (browsers cannot do Connect bidirectional streaming), and downloads and recordings are files the browser fetches by URL.
