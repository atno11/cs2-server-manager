# Phase 1.4 — Real-time events / SSE

Objective: deliver real-time public instance state and sanitized logs through one authenticated SSE stream and integrate the existing browser UI. Scope excludes WebSockets, event persistence, native CS2 changes, new dependencies, and Phase 1.5.

The fixed checklist was recorded before application changes. Every criterion begins PENDING.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| SSE-01 | Verify Git state, run the pre-change baseline and preserve all 87 existing top-level test scenarios. | PASS | Clean `118953c` baseline; check exit 0; 87 original tests remain within 103 final Go tests. |
| SSE-02 | Implement an isolated, concurrency-safe event broker with bounded resources and explicit lifecycle. | PASS | Broker tests cover subscriber limits, immutable payloads, concurrency, close and queue saturation. |
| SSE-03 | Successful committed registration and removal publish consistent events. Failed operations do not publish misleading success events. | PASS | `TestEventsCommittedMutationsAndSnapshot` uses injected persistence failures and verifies event absence/presence. |
| SSE-04 | Observable instance state transitions publish accurate events, including unexpected exits and launch failures. | PASS | State, restart, unexpected-exit and failed-launch event tests pass; existing lifecycle tests remain. |
| SSE-05 | Incremental log events use the existing sanitized log output, preserve bounded memory and never expose raw known secrets, including secrets split across writes. | PASS | Sanitized offset, overlap, split-write, finalization and 64 KiB ring tests pass. |
| SSE-06 | Each subscriber receives a consistent initial snapshot without losing changes occurring during subscription. | PASS | `TestEventsSnapshotDuringCommittedRegistration` and before/after registration snapshot tests pass. |
| SSE-07 | Events have documented ordering, sequence semantics and consistency guarantees. Concurrent publication does not introduce silent event gaps. | PASS | Broker concurrent publication and sequence tests pass; saturated subscribers disconnect. |
| SSE-08 | Implement a correctly framed HTTP SSE endpoint with flushing and streaming-compatible timeout handling. | PASS | Real loopback test checks headers, initial frame, flush and heartbeats beyond a shortened normal WriteTimeout. |
| SSE-09 | Preserve bearer authentication, Host and Origin checks, security boundaries and public response contracts. Credentials never appear in URLs or event payloads. | PASS | SSE rejection tests and complete security/HTTP suite pass; public payload and browser-asset tests check no credential/path exposure. |
| SSE-10 | Enforce bounded subscribers, queue sizes, event sizes and aggregate memory consumption. Preserve capacity for administrative HTTP requests. | PASS | Broker limit tests and eight-live-stream HTTP test verify ninth 503 and administrative 200. |
| SSE-11 | Slow or stalled subscribers cannot block producers, other consumers, process supervision or log capture. | PASS | Broker isolation and pipe-backed stalled-writer test pass; manager registration remains bounded. |
| SSE-12 | Heartbeat, request cancellation, network disconnect and write failure release subscriber resources. | PASS | Real-network disconnect, heartbeat, stalled-write deadline and snapshot-failure cleanup tests pass. |
| SSE-13 | Reconnect safely using bounded backoff and a fresh authoritative snapshot. Recover retained sanitized logs without silent duplication or stale state. | PASS | Node parser/model tests cover snapshot replacement, sequence gaps, byte-offset overlap and bounded backoff. |
| SSE-14 | The existing browser UI reflects server state changes and selected instance logs automatically, using a single authenticated stream. | PASS | UI stream and log reconciliation implemented; Node tests cover parser, auth stop and duplicate-stream prevention. SSE-W01 through SSE-W12 were subsequently reported PASS by the user. |
| SSE-15 | Manager and HTTP shutdown close streams, release resources and complete within documented bounds. | PASS | Active-stream HTTP shutdown, broker close/publish race and existing concurrent Close tests pass. |
| SSE-16 | Preserve previous storage, lifecycle, security and HTTP behavior, including existing error contracts. | PASS | All 87 original Go test scenarios retained; full check script passes, including REST and status-only error tests. |
| SSE-17 | Add deterministic tests for event ordering, snapshots, concurrency, redaction, backpressure, authentication, disconnect, reconnection and shutdown. | PASS | 16 new Go top-level tests and seven Node tests pass; targeted race count=10 exits 0. |
| SSE-18 | Complete applicable tests, race checks, build checks and scripts/check.sh. Document implementation, criterion-level evidence and remaining limitations. | PASS | `go mod verify`, full race count=3, Node tests and final `scripts/check.sh` exit 0; final coverage 81.1%. |

## Baseline

The clean checkout was on `feat/realtime-events` at `118953c3af9c76f68cba811707d358ee0095a04c`. `bash scripts/check.sh` exited 0 before application edits: gofmt, vet, uncached tests, race tests, coverage, build and Git whitespace checks all passed. `rg '^func Test' cmd internal | wc -l` counted 87 Go top-level tests. The local total statement coverage was 80.6% (cmd 68.8%, manager 83.4%, security 74.4%, storage 80.7%), whereas the stated published baseline is 79.8%; both figures are preserved. Native CS2 validation remains PENDING.

## Event architecture and contracts

`internal/events` owns immutable event payloads, process-local monotonic sequence numbers, subscriptions and closure. It has no process, SQLite, policy or HTTP responsibilities. The manager publishes `instance.registered` after a successful SQLite write and in-memory insert, and `instance.removed` after a successful SQLite delete and in-memory removal. `instance.state` carries the current public ID, port, state and generic error indicators. `instance.log` carries an ID, start/end byte offsets and base64-encoded sanitized bytes. Each SSE connection begins with `snapshot`, a sorted list of current public instances at a sequence boundary. Heartbeats are SSE comments and do not consume sequence numbers. Sequence numbers restart when the process restarts; `Last-Event-ID` is not replayed. A reconnect always receives a new snapshot.

State changes publish while the manager mutex is held. Subscription takes that mutex, registers with the broker, records the sequence boundary and copies current public state before releasing the mutex. Thus a change is either in the snapshot or queued afterward. The broker lock is never held while acquiring the manager or log-ring lock. Log output publishes from the ring after its existing redactor has finalized bytes, while holding only the ring lock; no network write occurs on producer paths. Log and state events share publication order, but a process may write output before its `running` state publication. A connected subscriber receives every accepted event after its boundary or is disconnected when its queue fills. It is never left connected after silent queue loss.

`GET /api/instances/{id}/logs` retains its plain-text body and adds `X-Log-Start` and `X-Log-End` byte offsets. The browser fetches this retained snapshot for its selected instance after a stream snapshot or selection change, buffers incremental events during that fetch, and removes duplicate overlap by offsets. A gap forces a reconnect. Only the last 64 KiB of sanitized output per instance is recoverable; expired bytes cannot be replayed. No event history is persisted.

## Resource, HTTP and browser behavior

The broker permits at most 8 subscribers, 128 queued events per subscriber and 4096 payload bytes per ordinary event. Sanitized output is split into at most 2048-byte log chunks before base64 JSON encoding. A snapshot is separately limited to 16 KiB and is sent outside the ordinary queue. Worst-case queued payload retention is 8 × 128 × 4096 = 4,194,304 bytes, plus at most 131,072 snapshot bytes and bounded channel/object overhead. The existing 32 × 64 KiB log rings retain at most another 2,097,152 bytes. This excludes general Go, SQLite, HTTP and process memory. A full queue disconnects its subscriber; network writes use a five-second per-write deadline and do not hold manager, ring or broker locks. The 8-stream limit leaves capacity under the existing 64-connection and 64-active-request limits for administrative requests.

The SSE endpoint runs through the existing bearer, exact Host/Origin and Fetch Metadata checks. Over-capacity and unauthorized requests fail before streaming headers. The status-only error writer now exposes its underlying writer to `http.ResponseController`, allowing immediate flush and endpoint-specific write deadline refresh while the server retains its 15-second normal `WriteTimeout`. Heartbeats are sent every 15 seconds. The browser uses one `fetch`/`ReadableStream` with an Authorization header, parses fragmented UTF-8 and SSE frames, keeps the token only in memory, and uses bounded 1–30 second reconnect backoff. A 401 response stops retries until the operator enters credentials again. It replaces stale state on each snapshot and uses `textContent` for untrusted output. Explicit event sequence and log-offset gaps trigger a fresh snapshot. The existing manual REST controls and log endpoint remain available.

HTTP shutdown closes the broker before waiting for active handlers, so SSE streams exit before manager process termination and SQLite closure. `Manager.Close` also closes the broker idempotently. An already blocked network write remains bounded by its per-write deadline. Concurrent publication after broker close returns an error without panicking.

## Validation and limits

The implementation uses only the Go standard library and the repository's existing dependencies. Tests use fake processes, real temporary SQLite databases, local HTTP listeners or in-memory pipe connections. Browser parser and reconnect logic are tested with Node's built-in test module; no browser automation framework was added. Interactive browser validation was later reported PASS by the user, as recorded below. Native CS2 validation remains PENDING. Unknown secrets remain undetectable, and direct external database modification is outside the supported model.

The first sandboxed `GOCACHE=/tmp/cs2m-sse-cache GOTMPDIR=/tmp go test -count=1 -run '^TestSSE' ./cmd/cs2m` exited 1 because `httptest.NewUnstartedServer` could not bind `[::1]:0` (`socket: operation not permitted`). This was an environment restriction, not a test assertion failure. The same targeted command rerun with approved local loopback access exited 0. No test was weakened or skipped to obtain a pass.

Final commands and results on the completed tree:

| Command | Exit | Evidence |
| --- | --- | --- |
| `go mod verify` | 0 | `all modules verified`; no `go.mod` or `go.sum` changes. |
| `node cmd/cs2m/ui.test.cjs` | 0 | Seven parser, snapshot/log, backoff, authentication and stream-ownership tests pass. This is JavaScript logic validation, not a real browser run. |
| `GOCACHE=/tmp/cs2m-sse-cache GOTMPDIR=/tmp go test -race -count=10 -timeout=180s -run 'TestBroker\|TestEvents\|TestSSE' ./internal/events ./internal/manager ./cmd/cs2m` | 0 | Repeated broker, manager-event and HTTP streaming tests pass under the race detector with approved loopback access. |
| `GOCACHE=/tmp/cs2m-sse-cache GOTMPDIR=/tmp go test -race -count=3 -timeout=180s ./...` | 0 | All five Go packages pass under the race detector with approved loopback access. |
| `bash scripts/check.sh` | 0 | With approved loopback access: PASS gofmt, vet, uncached tests, race, coverage, build and Git diff check. |

`rg '^func Test' cmd internal | wc -l` counted 103 Go top-level tests versus the 87-test baseline. The final check reported cmd 75.3%, events 93.3%, manager 82.4%, security 74.4%, storage 80.7%, and total 81.1% statement coverage. That is 0.5 percentage points above the local 80.6% baseline and 1.3 points above the stated 79.8% published baseline. These percentages are diagnostic, not a pass threshold. Simulated process and SQLite validation passed; native CS2 validation remains PENDING.

## Interactive browser validation — user-reported results

The user completed the isolated browser smoke test and reported all twelve criteria PASS. The directly supplied evidence is that REST authentication returned HTTP 200, the browser displayed Connected, registration and running state were visible, sequential simulator logs were visible, and known secrets appeared as `[REDACTED]`. The remaining checklist results were confirmed directly by the user. No independent timings, network traces or additional observations are claimed here.

| ID | Result | Evidence source |
| --- | --- | --- |
| SSE-W01 | PASS | User reported authentication and connection; HTTP 200 and Connected were observed. |
| SSE-W02 | PASS | User confirmed the initial empty snapshot. |
| SSE-W03 | PASS | User confirmed real-time registration; registration was visible. |
| SSE-W04 | PASS | User confirmed Start; running state was visible. |
| SSE-W05 | PASS | User confirmed incremental logs and redaction; sequential simulator logs and `[REDACTED]` were visible. |
| SSE-W06 | PASS | User confirmed Stop. |
| SSE-W07 | PASS | User confirmed Restart. |
| SSE-W08 | PASS | User confirmed multi-client synchronization. |
| SSE-W09 | PASS | User confirmed reconnection and recovery. |
| SSE-W10 | PASS | User confirmed instance removal. |
| SSE-W11 | PASS | User confirmed shutdown. |
| SSE-W12 | PASS | User confirmed browser security checks. |

An initial authentication attempt returned HTTP 401 because its token belonged to a different temporary runtime directory. The user identified `/tmp/cs2m-sse-smoke.NAAwHf8q/data` as the active isolated runtime and authenticated successfully with that runtime's token. No credential value is recorded, no application code change was needed, and temporary test artifacts must be preserved.

## Approved direction after Phase 1.4

The user approved an API-first, headless product direction. The core daemon is to remain independent of client UI implementations. A TUI is the first planned official interactive client; a CLI for administrative automation and an optional default Web UI are planned later. Custom clients may use the documented public REST and SSE API without adopting an official UI. The current embedded HTML/JavaScript page is a temporary functional validation client, not the final product UI. Future TUI work must consume the public API rather than manager internals. This closeout changes documentation only; it does not implement the TUI or refactor application code. The revised sequence is in [ROADMAP.md](../ROADMAP.md).

## Git handoff

The branch remains `feat/realtime-events` at `118953c3af9c76f68cba811707d358ee0095a04c`. Phase 1.4 changes remain unstaged. No commit, push, merge or deployment was performed. All SSE-01 through SSE-18 automated criteria and all SSE-W01 through SSE-W12 user-reported interactive criteria are PASS. Native CS2 validation remains a separate pending integration task. Do not start Phase 1.5 as part of this handoff.
