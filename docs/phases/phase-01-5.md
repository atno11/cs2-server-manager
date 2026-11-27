# Phase 1.5 — Headless Core & API Consolidation

Objective: separate HTTP transport from daemon bootstrap, make headless operation the default, document the public API, and provide a bounded Go API client for the future TUI. Scope excludes implementing the TUI, CLI, production Web UI, remote hosts, native CS2 changes, or new product features. This fixed checklist was recorded before application changes; every criterion begins PENDING.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| API-01 | Verify Git state, establish the pre-change baseline and preserve all 106 Go and 10 JavaScript test scenarios. | PASS | Clean `2ee67d3` branch apart from two unrelated untracked scripts; baseline check and Node suite exit 0. All 106 original Go test names remain, with the one documented relocation. |
| API-02 | Move HTTP transport responsibilities out of daemon bootstrap into a focused API package without changing existing runtime behavior. | PASS | `internal/api` owns routes, middleware, REST serialization, SSE and deadlines; original `cmd/cs2m` HTTP scenarios pass via test adapters. |
| API-03 | Core manager, storage and event packages remain independent of HTTP transport and UI implementation. No circular dependencies are introduced. | PASS | `go list` import graph confirms core package direction; Go compilation and tests pass. |
| API-04 | REST and SSE public instance representations and relevant payload contracts have consistent shared definitions without exposing internal manager data. | PASS | `internal/apitypes` shapes are shared; representation consistency test passes; private fields remain absent. |
| API-05 | Existing REST methods, routes, successful payloads, status codes, headers and error contracts remain compatible. | PASS | Original HTTP regressions pass; version 1 route/status inventory in `docs/API.md`; status-only writer assertion relocated unchanged. |
| API-06 | Existing SSE framing, event names, payloads, sequence semantics, snapshots, log offsets, authentication and backpressure remain compatible. | PASS | Existing SSE tests pass through relocated transport; broker, manager events and payload JSON shapes are unchanged. |
| API-07 | The daemon runs headless by default and provides all existing administrative functionality through the authenticated API without requiring UI assets. | PASS | Default handler excludes UI routes; browser-free real HTTP simulated workflow exercises the API. |
| API-08 | The temporary browser validation client can be enabled explicitly. Disabled UI routes do not expose its assets. Existing browser behavior is preserved when enabled. | PASS | `CS2M_VALIDATION_UI=1` is the sole opt-in; headless/opt-in route tests and all 10 original Node scenarios pass. |
| API-09 | Implement a reusable Go API client with validated local endpoint configuration, bearer authentication, safe credential handling and controlled HTTP transport behavior. | PASS | Client endpoint, token, redirect and proxy tests pass; bearer value is header-only and omitted from errors. |
| API-10 | The Go client supports all existing REST administrative operations and exposes useful typed results and status errors without leaking secrets. | PASS | All-method client test, real workflow and typed status/bounded response tests pass; no mutation auto-retry. |
| API-11 | The Go client consumes the existing SSE protocol with bounded parsing, cancellation, sequence validation and explicit recovery semantics. | PASS | Stream tests cover snapshot first, framing, UTF-8, CRLF, unknown events, invalid data, gaps, cancellation and incomplete reads. |
| API-12 | The Go client supports consistent instance snapshots, retained log snapshots and incremental log offset reconciliation suitable for the future TUI. | PASS | State/log tests cover snapshot replacement, overlap, byte bounds, stale tickets after removal/recreation/reconnect and gaps. |
| API-13 | Client requests, streaming reads, retries, response sizes, cancellation and resource cleanup remain bounded under documented operating assumptions. | PASS | REST 15-second and 64 KiB limits, 10-second header and 35-second SSE idle deadlines; bounded frames, queues and log bytes; no hidden retries/goroutines. |
| API-14 | Preserve all existing security boundaries, including loopback binding, Host and Origin checks, execution policy, strict JSON, secret redaction, storage permissions and process ownership. | PASS | Existing security, storage and lifecycle regressions pass; real workflow observes sanitized output; middleware and bootstrap checks moved without weakening. |
| API-15 | Demonstrate an authenticated end-to-end headless workflow using the real HTTP transport and a simulated server, without a browser. | PASS | `TestHeadlessSimulatedWorkflow` passes with private runtime, real listener, authenticated client and subscriber cleanup. |
| API-16 | Add deterministic tests for API compatibility, headless mode, client behavior, SSE parsing, log recovery, cancellation and failure handling. Preserve existing regression scenarios. | PASS | New Go tests cover requested paths; baseline-name comparison reports no missing original tests; Node suite remains 10/10. |
| API-17 | All applicable Go and JavaScript tests, repeated race tests, dependency checks, build checks and scripts/check.sh pass. | PASS | `go mod verify`, Node tests, targeted and full race count=3, cross-package coverage run and final `bash scripts/check.sh` all exit 0. |
| API-18 | Document the public REST/SSE contract, versioning, headless configuration, client integration, criterion-level evidence and remaining limitations. | PASS | `docs/API.md`, this report and updated README, architecture, status, roadmap, security and testing documents record the contract and evidence. |

Native CS2 validation remains PENDING.

## Baseline and scope

The pre-change tree was on `feat/headless-api` at `2ee67d31d0950a18e32f956f508eee937b311525`, with 140 accumulated acceptance criteria reported PASS. The only existing untracked files were `scripts/download-skins.py` and `scripts/extract-skins.py`; neither was modified, moved, staged or deleted. Before application edits, `bash scripts/check.sh` exited 0: formatting, vet, uncached Go tests, race tests, coverage, build and Git whitespace checks passed. The measured total Go statement coverage was 81.3%, equal to the published baseline. `rg '^func Test' cmd internal | wc -l` counted 106 Go top-level tests. `node cmd/cs2m/ui.test.cjs` exited 0 with 10 JavaScript tests. These are simulated and logic tests, not native CS2 validation.

The existing HTTP contract was inventoried from `cmd/cs2m/main.go`, `sse.go` and their tests before transport relocation. All nine `/api/...` routes, their status-only error behavior, public JSON fields, log offset headers, SSE frames and middleware checks are documented in [API.md](../API.md). The project remains a local, single-administrator modular monolith. This phase does not implement the TUI, CLI, remote hosts, TLS or a production browser UI.

## Package architecture and implementation

`cmd/cs2m` retains data-directory selection, policy and token initialization, manager construction, numeric loopback listener validation, connection limiting, signals and orderly shutdown. `internal/api` now owns routes, Host/Origin/Fetch Metadata and bearer checks, the 64-active-request limit, status-only errors, public REST projection, eight active SSE handler reservations, framing and write deadlines. The 64-connection listener wrapper remains with bootstrap because it is applied at listener creation. The same manager shutdown ordering and HTTP server timeouts are retained.

`internal/apitypes` holds the public definition, instance and event payload shapes. `manager.Definition` and `manager.EventInstance` are aliases of these transport-neutral shapes, so existing manager callers retain their Go names and JSON field contracts. `internal/api` projects manager instances into the public type; manager events serialize the same type without exposing private paths, arguments or internal errors. `internal/apiclient` imports `apitypes` and standard-library packages only. `go list` confirms no manager, storage, event, HTTP transport or UI package is imported by the client and no HTTP/UI package is imported by the core manager, storage or broker.

Headless operation is the default. The literal environment setting `CS2M_VALIDATION_UI=1` enables the unchanged embedded validation page and JavaScript route; any other value leaves `/` and `/ui.js` unavailable. API routes, middleware and credentials are the same in both modes. Existing `cmd/cs2m` HTTP tests use small test-only handler adapters that enable the validation UI to preserve their original scenarios. `TestFollowupHTTPOutputNotLogBounded` moved to `internal/api` with its assertion unchanged because the status-only writer moved there. No other existing Go test scenario was removed; all 10 JavaScript cases remain at their original path. `scripts/check.sh` now includes the Node suite. Node.js is a development validation prerequisite, not a daemon runtime dependency.

## Go client and recovery

`apiclient.New` requires a numeric IPv4 or IPv6 loopback HTTP authority and an explicit 64-character lowercase hexadecimal bearer token. It rejects URL user information, paths, queries, fragments, unsupported schemes, non-loopback hosts, redirects and ambient HTTP proxies. The client sends the token only in the Authorization header. Status and network errors omit response bodies, request paths and credentials. REST requests have a 15-second deadline and bounded request/response bodies; response bodies are always closed. A failed mutating request is not retried because its persistence outcome may be ambiguous. The caller recovers through `Get` or `List`.

The client implements all current REST administrative operations plus a pull-based `Subscribe`. Each stream must begin with an authoritative snapshot. The parser handles fragmented UTF-8, LF/CRLF, multiline data, comments, known JSON event shapes and unknown future event types within fixed frame limits. Decimal sequence IDs must increase by one after the snapshot; a gap, malformed event, unexpected EOF or stalled read is explicit failure. The stream uses a 35-second connection read deadline refreshed after complete frames, accounting for the server's 15-second heartbeat. There is no background reconnection goroutine: callers close the stream and subscribe again for a fresh snapshot. A 401 remains a typed status error requiring caller action.

`Logs` verifies the retained-log byte-offset headers and 64 KiB response cap. `LogBuffer` owns at most 64 KiB of copied sanitized bytes, suppresses duplicate or overlapping incremental events and reports byte gaps. `State` replaces its instance map on snapshot; it allows one selected-instance log recovery at a time and queues no more than 128 bounded events during a retained-log fetch. A generation ticket rejects delayed responses after another selection, snapshot, removal or same-ID registration. A gap requires another authoritative retained-log fetch. The helper never redacts again or stores historical output. Expired server-ring bytes remain unrecoverable.

## Browser-free integration and test preservation

`TestHeadlessSimulatedWorkflow` uses a private temporary runtime, trusted shell simulator and real loopback HTTP server with the validation UI disabled. The Go client authenticates, reads an empty SSE snapshot, registers and lists an instance, observes registration, starts the process, observes running state and sanitized `[REDACTED]` output, reads retained logs, stops, removes, closes the stream and verifies subscriber cleanup. This is simulated-process validation only. Other tests cover the disabled and enabled validation routes, representation consistency, every client administration method, unsafe endpoint and redirect rejection, proxy isolation, status-only errors, bounded bodies, mutation ambiguity without retry, stream cancellation, malformed/oversized/incomplete SSE frames, sequence gaps, UTF-8 fragments, log overlap, byte bounds and stale recovery tickets.

Two targeted development runs failed before test-fixture corrections and are not reported as product defects. `go test -count=1 -timeout=60s ./internal/apiclient` exited 1 because a test supplied literal backslash-n characters where an SSE newline was intended; the fixture was corrected. `go test -count=1 -timeout=90s ./internal/api ./internal/apiclient` exited 1 when the simulator emitted too few bytes for the existing cross-write redactor to finalize output before process exit; the simulator was changed to emit a longer line. The subsequent targeted integration test exited 0. No production redaction code was changed.

## Validation and remaining limits

Final commands and results on the completed tree:

| Command | Exit | Evidence |
| --- | --- | --- |
| `bash scripts/check.sh` before application changes | 0 | Baseline gofmt, vet, uncached tests, race, coverage, build and Git diff check pass; 81.3% total coverage. |
| `node cmd/cs2m/ui.test.cjs` before application changes | 0 | Ten JavaScript tests pass. |
| `go test -count=1 -timeout=180s ./...` | 0 | All existing and new Go scenarios pass. |
| `go mod verify` | 0 | All existing modules verified; no dependency changes. |
| `node cmd/cs2m/ui.test.cjs` | 0 | All ten original JavaScript tests pass. |
| `go test -race -count=3 -timeout=180s -run 'TestClient\|TestStream\|TestLog\|TestHeadless\|TestPublicREST\|TestSSE\|TestAPI\|TestState\|TestFollowupHTTPOutputNotLogBounded' ./internal/apiclient ./internal/api ./cmd/cs2m` | 0 | Repeated client, transport, stream and compatibility tests pass under the race detector. |
| `go test -race -count=3 -timeout=180s ./...` | 0 | Every Go package passes under the race detector on the final tree. |
| `bash scripts/check.sh` | 0 | gofmt, vet, uncached Go tests, Node tests, race, coverage, standard build and Git diff check all pass. |
| `go test -coverpkg=./... -coverprofile=/tmp/cs2m-phase15-crosspkg.cover -count=1 ./...` | 0 | Cross-package coverage run passes; `go tool cover -func=/tmp/cs2m-phase15-crosspkg.cover \| tail -1` reports 81.6%. |
| `git diff --check` | 0 | No tracked whitespace errors. |

`rg '^func Test' cmd internal | wc -l` counts 121 Go top-level tests, up from 106. A baseline/current test-name comparison found no missing original scenario. The JavaScript count remains 10. The final default `scripts/check.sh` coverage is 76.2%, down 5.1 percentage points from the published 81.3% baseline. This change mainly reflects Go's default per-package coverage attribution: many established `cmd/cs2m` tests still exercise the moved `internal/api` transport, but those calls are not attributed to `internal/api` in a default `go test -coverprofile` run. The separate `-coverpkg=./...` run measures 81.6% across package boundaries; it is a different metric and is not presented as a like-for-like increase over the 81.3% baseline. Coverage remains diagnostic with no arbitrary pass threshold.

## Interactive validation — completed after implementation

The user completed the four targeted interactive checks and reported all four PASS. API-W01 includes the directly supplied HTTP status results below. API-W02 through API-W04 are user-reported outcomes without additional measured timings or traces.

| ID | Result | Evidence |
| --- | --- | --- |
| API-W01 | PASS | Verified HTTP results: `GET /` returned 404; `GET /ui.js` returned 404; unauthenticated `GET /api/instances` returned 401; authenticated `GET /api/instances` returned 200. |
| API-W02 | PASS | User reported the optional validation UI check passed. |
| API-W03 | PASS | User reported administrative operations and SSE passed. |
| API-W04 | PASS | User reported shutdown passed. |

The client is local-only and does not provide TLS, remote hosts, automatic SSE reconnection, event persistence, unlimited log history or native CS2 compatibility. Native CS2 validation remains PENDING. Phase 1.5 closes with API-01 through API-18 PASS, interactive validation 4/4 PASS, and 162/162 accumulated acceptance criteria reported PASS.

## Git state and handoff

The branch remains `feat/headless-api` at `2ee67d31d0950a18e32f956f508eee937b311525`. Changes are unstaged and uncommitted. The two unrelated untracked skin scripts remain untouched and must not be included in a phase commit. No push, merge, deployment or Phase 1.6 work was performed.
