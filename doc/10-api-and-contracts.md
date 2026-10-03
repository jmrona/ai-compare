# 10. API and contracts

The browser talks to `api` (port 4700) in three ways:

| Channel | Used for | State |
|---|---|---|
| **Connect** (protobuf contract) under `/api/rpc/` | Typed request/response and, later, server streaming | `CatalogService` today; everything moves here |
| **JSON** routes under `/api/` | Everything not migrated yet | Shrinking |
| **WebSocket** | Terminals | Stays a WebSocket (see [Terminals](07-terminals.md)) |

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
4. call them from `frontend/src/api/rpc.ts`.

`buf.yaml` enables the `STANDARD` lint rules and `FILE` breaking-change detection.

### CatalogService

```proto
service CatalogService {
  rpc GetCatalog(GetCatalogRequest) returns (GetCatalogResponse) { option idempotency_level = NO_SIDE_EFFECTS; }
  rpc RefreshCatalog(RefreshCatalogRequest) returns (RefreshCatalogResponse);
}
```

`NO_SIDE_EFFECTS` lets the browser call `GetCatalog` with **HTTP GET** (`useHttpGet: true` in the transport), which is cacheable and easy to see in DevTools.

### Server side

`internal/rpc.Handler` creates a `connect.Server`, registers the services and mounts them with `connecthttp.Mount` on a mux, which `main.go` serves under `/api/rpc/` with the prefix stripped. A method's full path is therefore:

```
/api/rpc/aicompare.v1.CatalogService/GetCatalog
```

Errors are returned as Connect errors (`connect.NewError(connect.CodeUnavailable, …)`), which clients receive with a code and message.

### Client side

`frontend/src/api/rpc.ts` creates a Connect transport (`@connectrpc/connect-web`, base URL `${VITE_API_BASE_URL}/rpc`) and clients with `createClient(CatalogService, transport)`. Small mapping functions turn protobuf messages into the plain types the UI already uses (`src/api/types.ts`), for example `Timestamp` → ISO string and an unset optional price → `null`.

The Go side has a generated client too; `spike proxy-check` uses it to read the catalogue.

### Trying it with curl

Connect speaks JSON as well as binary:

```bash
curl -s "http://127.0.0.1:4700/api/rpc/aicompare.v1.CatalogService/GetCatalog?connect=v1&encoding=json&message=%7B%7D"
```

```bash
curl -s -X POST -H "Content-Type: application/json" -d "{}" http://127.0.0.1:4700/api/rpc/aicompare.v1.CatalogService/RefreshCatalog
```

## JSON routes (to be migrated)

| Method and path | Purpose |
|---|---|
| `GET /api/health` | `{"status":"ok","database":"ok","providers":{"openai":true,"anthropic":false}}` |
| `POST /api/projects/inspect` | `{"path"}` → files, size, Git, harness files and who reads them, excluded `.env` files, proposed profile |
| `POST /api/comparisons` | Start a comparison → `{"id"}` (body in [Comparison lifecycle](04-comparison-lifecycle.md)) |
| `GET /api/comparisons` | Every comparison, newest first |
| `GET /api/comparisons/active` | The newest comparison with a side still running, or `null` |
| `GET /api/comparisons/{id}` | One comparison with both sides, status and metrics |
| `POST /api/comparisons/{id}/sides/{side}/finish` | End a side as finished |
| `POST /api/comparisons/{id}/sides/{side}/cancel` | End a side as cancelled |
| `GET /api/comparisons/{id}/sides/{side}/logs` | Orchestrator notes and proxied requests |
| `GET /api/comparisons/{id}/sides/{side}/download` | Zip of the side's `/workspace` |
| `GET /api/comparisons/{id}/sides/{side}/terminal` | WebSocket |
| `POST /api/spike/proxy/sessions`, `GET …/{id}` | Phase 0 tools: create and inspect proxy sessions by hand |
| `GET /api/spike/terminal?image=` | Phase 0 WebSocket: throwaway bash container |

Errors are `{"error": "message"}` with a suitable status. Unknown `/api/` paths get a JSON 404 instead of the SPA's `index.html`.

The JSON shapes mirror `frontend/src/api/types.ts` and the Go `View` types in `internal/comparison`.

## Planned: the event stream

Polling is used today (TanStack Query refetches a live comparison every second). The plan replaces it with a Connect **server stream**, `EventService.Watch`: status changes, live metrics every ~2 s, "comparison finished" and "report generated", each with a sequence number so a reconnecting client can ask for what it missed. Each event updates or invalidates the TanStack Query cache. That is also when `@connectrpc/connect-query` will replace the hand-written hooks.
