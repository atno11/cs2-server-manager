# Phase 1.4.1 — Event streaming review

Objective: investigate the three reported SSE findings and correct confirmed defects in stream capacity, log reconciliation and write deadline handling. Scope excludes new features, UI redesign, TUI work and Phase 1.5. This fixed checklist was recorded before application changes; every criterion begins PENDING.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| SSER-01 | The maximum number of active SSE HTTP handlers remains bounded, including disconnected subscribers whose network writes have not yet returned. Administrative HTTP capacity remains available. | PASS | Eight blocked handlers retain reservations after broker overflow; ninth stream gets 503, administration gets 200, and a slot is reusable after handler exit. |
| SSER-02 | Log reconciliation remains correct across instance removal, same-ID recreation, delayed log responses, selection changes and reconnects. | PASS | Three asynchronous JavaScript regressions pass on unchanged browser code; old responses are ignored and fresh logs reconcile by offsets. |
| SSER-03 | Write deadline configuration failures are handled safely. SSE cannot silently operate without its required bounded-write guarantees. | PASS | Controlled writer tests cover initial deadline failure (503 before stream) and refresh failure (committed stream ends without a new error response). |
| SSER-04 | Deterministic tests reproduce confirmed defects where feasible and preserve all existing Go and JavaScript test scenarios. | PASS | Both confirmed Go defects failed before the fix; 106 Go top-level tests include the original 103, and 10 JavaScript tests include the original seven. |
| SSER-05 | Applicable regression tests, race checks, JavaScript tests, build and scripts/check.sh pass. | PASS | `go mod verify`, Node tests, targeted race count=3, full race count=3 and final `bash scripts/check.sh` all exit 0. |
| SSER-06 | Document findings, decisions, criterion-level evidence, limitations and final Git state. | PASS | This report and architecture, status, roadmap, security and testing documents record the findings and validation; final branch and changes are listed below. |

Native CS2 validation remains PENDING.

## Baseline and review findings

The branch was `feat/realtime-events` at `0624749f2d8ae2722384bc4648505d06f9cce3f9`. The only pre-existing changes were the untracked `scripts/download-skins.py` and `scripts/extract-skins.py`; neither was opened for editing, staged or deleted. The published Phase 1.4 baseline reports 103 Go top-level tests, seven JavaScript tests and 81.1% Go statement coverage. The initial `node cmd/cs2m/ui.test.cjs` exited 0 with seven tests passing.

The first `bash scripts/check.sh` attempt began before application edits but overlapped development of the new regression tests. It was interrupted after gofmt, vet, uncached tests and race passed because the initial deadline regression test waited for a stream that the original implementation never terminated. Coverage and build in that interrupted run exited 1; this run is not treated as a passing baseline. The regression test was changed to use a bounded request context. A detached temporary worktree at the unmodified `0624749` commit then ran `bash scripts/check.sh` to completion with exit 0: gofmt, vet, tests, race, coverage, build and Git diff check all passed; coverage was 81.1%. The worktree was removed after the run. The final complete check on the review changes is recorded below.

### SSE-R01 — effective stream capacity: confirmed

The broker removes a subscriber immediately when its queue fills. A handler already blocked in `ResponseWriter.Write` remains active until its deadline or network write returns. Before the fix, eight controlled writers reached a blocked second write, 130 registration/removal publications saturated their queues, and the broker subscriber count became zero while all eight handlers remained blocked. A ninth authenticated stream entered the handler. `go test -count=1 -timeout=15s -run 'TestSSEDeadlineSetupFailureRejectsBeforeHeaders|TestSSEOverflowKeepsHTTPHandlerReservations' ./cmd/cs2m` exited 1 with `TestSSEOverflowKeepsHTTPHandlerReservations: ninth HTTP handler entered`.

The HTTP handler now owns an independent eight-slot admission channel. A reservation is taken after the existing security middleware and released by a defer only when that HTTP handler exits. Broker overflow still disconnects immediately and publication remains nonblocking. This bounds active handlers, including those awaiting a blocked write, to eight per HTTP handler instance. The general 64-request limit still has room for administrative requests. The regression test also checks that a slot is reusable after the blocked handlers actually exit.

### SSE-R02 — log reconciliation: finding not confirmed

The UI's stream generation, selected ID and incrementing log-load ID reject delayed responses after a new load, removal or selection change. Removal clears the selected ID and increments the load ID; selecting a recreated same-ID instance starts a new load. A fresh reconnect snapshot starts another load for the selected ID and invalidates the older response. Incremental log events received during loading are queued and reconciled by byte offsets, suppressing overlap and rejecting gaps. The manager completes process log flushing before an instance can be removed. No instance incarnation field or HTTP contract change was justified by the tested cases.

Three deterministic JavaScript tests cover removal and same-ID recreation with an old delayed response and queued old/new log events, automatic reconnect with an old delayed response, and switching the selected instance during loading. All pass on the unchanged `ui.js`. Retained logs remain limited to 64 KiB; bytes already expired from the ring cannot be recovered. A disconnected browser continues showing its last known view until a fresh snapshot arrives.

### SSE-R03 — ignored deadline errors: confirmed

The original stream ignored errors from both `ResponseController.SetWriteDeadline` calls. A controlled writer returning `io.ErrClosedPipe` reproduced a 200 SSE snapshot despite failed deadline setup. The same pre-fix targeted command above exited 1 with `TestSSEDeadlineSetupFailureRejectsBeforeHeaders: unsafe stream setup: status=200 writes=1 subscribers=0`.

The first write deadline is now required before any SSE headers or body are committed. Failure returns a status-only 503 and releases the subscription and HTTP reservation. A later deadline failure ends the already committed stream; it does not attempt a new error response. Normal server `WriteTimeout`, heartbeat and per-write deadlines remain. Go's `net/http.ResponseController` documentation states that a write deadline cannot be extended after it expires; the stream will end if it misses a deadline.

## Interactive validation and limitations

The browser implementation and public REST/SSE payloads did not change. The Phase 1.4 user-reported interactive results remain historical evidence. After the automated review, the user completed the targeted interactive retests below and reported all four PASS. No measured timings, network traces or further observations were supplied.

| Interactive criterion | User-reported result |
| --- | --- |
| SSE-W01 — authentication and connection | PASS |
| SSE-W08 — multi-client synchronization | PASS |
| SSE-W09 — reconnection and recovery | PASS |
| SSE-W11 — shutdown | PASS |

These retests exercise the stream behavior affected by the new HTTP reservation. The automated tests use controlled writers and local HTTP listeners, not a native CS2 installation. Native CS2 validation remains PENDING.

## Final validation

| Command | Exit | Evidence |
| --- | --- | --- |
| `bash scripts/check.sh` in detached clean `0624749` worktree | 0 | Pre-change gofmt, vet, tests, race, coverage, build and Git diff check pass; 81.1% total coverage. |
| `node cmd/cs2m/ui.test.cjs` before edits | 0 | Seven JavaScript tests pass. |
| `go test -count=1 -timeout=15s -run 'TestSSEDeadlineSetupFailureRejectsBeforeHeaders\|TestSSEOverflowKeepsHTTPHandlerReservations' ./cmd/cs2m` before fixes | 1 | Both confirmed regressions fail as described above. |
| `go test -count=1 -timeout=60s -run 'TestSSEDeadline\|TestSSEOverflow\|TestSSEEightStreams\|TestSSEServerShutdown\|TestSSEStalledWriter' ./cmd/cs2m` | 0 | Targeted HTTP stream, capacity and shutdown checks pass after fixes. |
| `go mod verify` | 0 | All modules verified; no dependency changes. |
| `node cmd/cs2m/ui.test.cjs` | 0 | Ten tests pass, including seven original scenarios. |
| `go test -race -count=3 -timeout=180s -run 'TestBroker\|TestEvents\|TestSSE' ./internal/events ./internal/manager ./cmd/cs2m` | 0 | Broker, manager event and HTTP streaming cases pass under the race detector. |
| `go test -race -count=3 -timeout=180s ./...` | 0 | All five Go packages pass on the final tree. |
| `bash scripts/check.sh` | 0 | gofmt, vet, uncached tests, race, coverage, build and Git diff check all pass. |
| `git diff --check` | 0 | No whitespace errors. |

`rg '^func Test' cmd internal | wc -l` counts 106 Go top-level tests, three more than the published 103. Node reports 10 tests, three more than the original seven. Final overall Go statement coverage is 81.3%, up 0.2 percentage points from the 81.1% baseline. Coverage is diagnostic, not an acceptance threshold. No application dependencies, SQLite schema, broker queue size or browser production code changed.

## Git state and handoff

The final branch remains `feat/realtime-events` at `0624749f2d8ae2722384bc4648505d06f9cce3f9`. Review changes are unstaged and uncommitted. The two unrelated untracked skin scripts remain untouched and must not be included in the review commit. No push, merge or deployment occurred. Phase 1.5 has not started.
