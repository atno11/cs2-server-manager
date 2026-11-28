# Phase 1.6.2 — Independent review hardening

Objective: investigate F01–F04 in TUI log recovery and reconnect backoff, and fix only reproduced defects. Scope is log-ticket scheduling, stale-response handling, bounded retries and backoff; terminal UI design, dependencies, native CS2 and unrelated files are excluded. Starting branch `feat/tui-mvp`, HEAD `33aadd8`, with a clean tracked tree and only unrelated untracked `scripts/download-skins.py` and `scripts/extract-skins.py`.

## Fixed acceptance checklist (initial state)

| ID | Criterion | Status |
|---|---|---|
| IR-01 | Preserve baseline of 147 Go and 10 JS tests | PENDING |
| IR-02 | Reproduce F01 before fixing | PENDING |
| IR-03 | Recover selected logs after every authoritative snapshot | PENDING |
| IR-04 | Establish recovery ticket before scheduling next event read | PENDING |
| IR-05 | Investigate and reproduce F02 | PENDING |
| IR-06 | Ignore stale successful and failed log responses | PENDING |
| IR-07 | Verify and enforce bounded log recovery | PENDING |
| IR-08 | Verify and enforce bounded reconnection backoff | PENDING |
| IR-09 | Verify stopped and running instance recovery | PENDING |
| IR-10 | Preserve existing terminal security and mutation guards | PENDING |
| IR-11 | Pass integration, race and full project checks | PENDING |
| IR-12 | Update phase documentation and Git status | PENDING |

Previous Phase 1.6/1.6.1 user-reported interactive acceptance remains unchanged. Native CS2 validation remains pending.

## Findings and before/after evidence

The pre-change focused TUI baseline (`TestModelReconnectAndStaleGeneration`, `TestReconnectBackoffResetsOnlyAfterStableConnection`, `TestModelIncrementalLogReconciliation`) exited 0. The source baseline contains 147 Go top-level tests and 10 JavaScript tests. Four new deterministic regressions failed before production changes:

- F01: `TestSnapshotRecoversRetainedLogsForUnchangedSelection` failed for both stopped and running instances with `ticket was not established before next event: stale log snapshot`. `State.Apply(snapshot)` invalidated log state; `syncInstances` returned nil for the unchanged selected ID, so a stopped instance had no incremental event to trigger recovery.
- F02: `TestStaleFailedLogRequestDoesNotRetryCurrentSelection` failed because a canceled older request incremented `logRetries` for the new selection. The model checked connection generation but not recovery ticket on the error branch.
- F03: `TestStaleLogEventRecoveryIsBounded` failed with `retries=0` after four forced stale-log events. The `ErrStaleLog` path started new attempts without applying the three-attempt limit.
- F04: `TestFailedSubscribeDoesNotReusePreviousConnectedAt` failed because a failed subscription reset `backoff` to 1 from a previous connection's old timestamp.

The fix starts retained-log recovery after every valid snapshot with a surviving selected ID, including when selection is unchanged; `beginLog` establishes the ticket synchronously before the next event-read command is batched. The model records the active log ticket and ignores any successful or failed response for a superseded ticket. It applies the existing three-attempt limit to stale-log event recovery. On stream end, it evaluates whether the connection was stable once and clears `connectedAt`, so failed subscriptions cannot reuse it. No new dependency or UI redesign was added.

The four targeted regressions passed after the fix. `TestStaleFailedLogRequestDoesNotRetryCurrentSelection` also covers a newer attempt for the same selected ID. Existing stale-success, terminal security, keyboard and mutation tests remain in the accumulated suite. `TestRealHTTPModelWorkflow` now exercises authenticated SSE reconnect and retained-log recovery with the same selected instance while running and stopped; its original final assertion that the simulator remains running after TUI close is preserved. The sandboxed integration command failed on `httptest` loopback denial; the unrestricted real HTTP/SSE run exited 0. Repeated targeted TUI race count=3 exited 0. Native CS2 was not launched.

## Acceptance status

| ID | Status | Evidence |
|---|---|---|
| IR-01 | PASS | All 147 baseline Go tests retained; 151 top-level Go tests and 10 JavaScript tests pass. |
| IR-02 | PASS | F01 regression failed before fix for stopped and running selections. |
| IR-03 | PASS | Snapshot recovery regression and real HTTP/SSE reconnect for both states pass. |
| IR-04 | PASS | Regression applies an incremental event immediately after snapshot and verifies the log fetch precedes the next read command. |
| IR-05 | PASS | F02 regression failed before fix on stale failed response; same-selection supersession also covered. |
| IR-06 | PASS | Active ticket gate rejects stale failed responses; existing stale-success test and full suite pass. |
| IR-07 | PASS | F03 regression failed before fix and passes with three-attempt pause. |
| IR-08 | PASS | F04 regression failed before fix and passes; existing short/stable backoff tests pass. |
| IR-09 | PASS | Unit and real HTTP/SSE integration cover stopped and running selected instances. |
| IR-10 | PASS | Existing sanitizer, keyboard, paste, mutation and integration tests pass unchanged. |
| IR-11 | PASS | Real HTTP/SSE integration, repeated targeted race, full Go/JavaScript/race/coverage/build checks pass. |
| IR-12 | PASS | This report, testing policy and final Git state updated. |

## Validation and Git state

The unrestricted `bash scripts/check.sh` exited 0: gofmt, vet, uncached Go tests, 10/10 JavaScript tests, full race tests, coverage, build and Git whitespace validation passed. Default Go statement coverage is 75.3% (prior committed baseline 75.1%); package `internal/tui` reports 75.2%. Coverage is diagnostic, not a percentage gate. `go test -race -count=3 -timeout=180s ./internal/tui ./cmd/cs2m-tui` exited 0. The unrestricted real HTTP/SSE integration test exited 0; the sandboxed attempt failed solely because `httptest` could not bind loopback. A separate `git diff --check` exited 0. No native CS2 validation or new physical Ghostty validation was performed.

All 12 fixed IR criteria are PASS. Branch remains `feat/tui-mvp` at `33aadd8`. Only the two unrelated untracked skin scripts preceded this phase and remain untouched. This phase changes `internal/tui/model.go`, `internal/tui/tui_test.go`, `internal/tui/integration_test.go`, `docs/TESTING.md`, and this new report. `git status --short` lists exactly those files plus `scripts/download-skins.py` and `scripts/extract-skins.py` as untracked. No commit, push, merge or Phase 1.7 work occurred. Earlier Phase 1.6/1.6.1 user-reported interactive acceptance has not been rewritten; it is separate from this simulated regression work.

## Final physical acceptance follow-up

After commit `fddbe86`, the user reported RT-01 through RT-06 PASS (6/6) from physical Ghostty reconnection and retained-log recovery testing with both running and stopped simulator instances. The individual RT procedures and traces were not supplied here, so these are recorded as user-reported outcomes, separate from the automated and real HTTP/SSE integration evidence above. The user also reported that an independent static review of `fddbe86` found all four F01–F04 corrective findings resolved. That is reviewer-reported static evidence, not a new test execution.

| ID | Result | Evidence |
|---|---|---|
| RT-01 | PASS | User-reported physical Ghostty validation; individual observation not supplied. |
| RT-02 | PASS | User-reported physical Ghostty validation; individual observation not supplied. |
| RT-03 | PASS | User-reported physical Ghostty validation; individual observation not supplied. |
| RT-04 | PASS | User-reported physical Ghostty validation; individual observation not supplied. |
| RT-05 | PASS | User-reported physical Ghostty validation; individual observation not supplied. |
| RT-06 | PASS | User-reported physical Ghostty validation; individual observation not supplied. |

One recent disconnected session was conclusively traced to the daemon not running. Earlier unexpected disconnections remain unexplained; the stopped-daemon explanation is not applied to them. The original IR-01–IR-12 results remain 12/12 PASS. The earlier REG-01 and REG-02 investigative criteria remain PENDING because the historical `n` failure was not reproduced or diagnosed. Native CS2 validation remains PENDING. The Phase 1.6.2 automated baseline remains 151 Go top-level tests, 10 JavaScript tests and 75.3% default Go coverage from the prior executed check; no automated tests were rerun for this documentation-only closeout.

Documentation closeout Git state: branch `feat/tui-mvp` at `fddbe86`; the tracked tree was clean before these edits. Only `docs/phases/phase-01-6-2.md`, `docs/PROJECT_STATUS.md`, `docs/TESTING.md` and `docs/ROADMAP.md` are changed by this closeout. Unrelated untracked `scripts/download-skins.py` and `scripts/extract-skins.py` remain untouched. No production files, tests or dependencies changed; no commit, push, merge or Phase 1.7 work occurred.
