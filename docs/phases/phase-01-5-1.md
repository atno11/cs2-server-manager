# Phase 1.5.1 — API client review

Objective: investigate log recovery coordination, HTTP transport isolation and SSE payload validation; correct confirmed defects only. Scope excludes TUI and CLI implementation, browser changes, database or process lifecycle changes, and new product features. This fixed checklist was recorded before application edits. All criteria begin PENDING.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| APIR-01 | Establish the clean pre-change baseline and preserve all existing test scenarios. | PASS | Baseline check exit 0; all 121 original Go test names retained, all 10 JavaScript tests pass. |
| APIR-02 | Log recovery across reconnects, snapshots, selection changes and delayed responses is deterministic and has a documented caller integration contract. | PASS | Pre-fix and final `TestStateCallerRecoveryWorkflow` exit 0; [API.md](../API.md) specifies ordering and retry responsibility. |
| APIR-03 | The API client owns its HTTP transport configuration without unsafe assumptions about global transport state. Response headers and bodies remain bounded. | PASS | Pre-fix transport tests exit 1; post-fix targeted and race tests exit 0; explicit 16 KiB header and 64 KiB REST body caps. |
| APIR-04 | SSE payload validation consistently enforces the existing public contract, including instance IDs. | PASS | Pre-fix SSE tests exit 1; post-fix invalid ID and required-field tests exit 0. |
| APIR-05 | Complete applicable regression, race, dependency, JavaScript, build and coverage checks pass. | PASS | Commands and exit codes below; check script exit 0. |
| APIR-06 | Document findings, evidence, decisions, limitations and final Git state. | PASS | This report, API, architecture, roadmap, security, testing and status docs updated; final status below. |

TUI-SEC01 is a Phase 1.6 requirement: logs are untrusted terminal content. The TUI must safely render control characters, ANSI/OSC sequences and malformed UTF-8 while preserving the API client's original sanitized log bytes and byte offsets. No terminal rendering is implemented here.

Native CS2 validation remains PENDING.

## Baseline and scope

Branch `feat/headless-api`, HEAD `60e896ec689a5efe68e92898acd5347b984a240c`. The initial working tree had no tracked changes; only unrelated untracked `scripts/download-skins.py` and `scripts/extract-skins.py` were present and are untouched. Before application edits, `bash scripts/check.sh` exited 0 (format, vet, uncached Go and Node tests, race, coverage, build and whitespace checks). The baseline contained 121 top-level Go tests and 10 JavaScript tests. Locally measured default coverage was 76.1%, 0.1 percentage point below the published 76.2%; `go test -coverpkg=./... -coverprofile=/tmp/cs2m-phase151-baseline.cover -count=1 ./...` and `go tool cover -func=/tmp/cs2m-phase151-baseline.cover` exited 0 and reported 81.6% cross-package coverage.

## Finding investigation and pre-fix reproduction

**API-R01 — caller coordination, not a client defect.** Applying a fresh snapshot clears the selected log and in-flight recovery while retaining selection if the instance still exists. A selected log event before `BeginLog` returns `ErrStaleLog` by design; the pull-based stream lets the caller begin recovery before calling `Next` again. The new `TestStateCallerRecoveryWorkflow` passed before application edits with `go test -count=1 -timeout=30s -run '^TestStateCallerRecoveryWorkflow$' ./internal/apiclient` (exit 0). It covers a fresh snapshot, gated concurrent event processing, failed-fetch retry, delayed stale responses, byte gaps, 128-event backlog and same-ID recreation. No protocol or state abstraction change is needed.

**API-R02 — confirmed.** `http.DefaultTransport.(*http.Transport).Clone()` panicked if another package replaced the global transport with a different `RoundTripper`, and silently inherited external transport settings. A 20 KiB response header was accepted because no header cap was set. The new pre-fix tests `TestClientOwnsTransportWithoutDefaultTransport`, `TestClientRejectsOversizedResponseHeaders` and `TestClientTransportConfiguration` failed (targeted run exit 1): the child process panicked at `client.go:67`, the oversized header was accepted, and configuration reported header=0, per-host connections=0, idle per host=4. The child process isolates the global mutation from other tests.

**API-R03 — confirmed.** The decoder accepted removal/log IDs that exceeded 64 bytes or contained `/`, spaces or non-ASCII characters. It also accepted absent or null required log fields because zero-valued decoding masked omission. The new `TestSSERejectsInvalidRemovalAndLogIDs` and `TestSSELogRequiresContractFields` failed before the fix (targeted run exit 1). Valid future additive JSON fields remain accepted.

## Decisions and guarantees

The client now creates its own standard-library `http.Transport`: no ambient proxy, five-second dial timeout, ten-second response-header timeout, 16 KiB maximum response headers, 16 per-host connections, eight total idle connections, four idle per host and 30-second idle timeout. It retains redirect refusal, explicit contexts, bounded REST bodies and frames, and `CloseIdleConnections`. The stream still requests its own connection. These limits do not enable remote hosts.

SSE removal and log IDs now use the same `validID` rule as REST and state handling. A known log event requires `id`, `start`, `end` and base64 `bytes` with the documented types; missing or null fields fail at the decoder. Existing byte-offset, size and sequence checks still apply. Unknown future event types and additive fields remain compatible. No REST or SSE wire contract changed.

The caller workflow is specified in [API.md](../API.md): after applying each authoritative snapshot, call `BeginLog` for a still-selected instance before reading the next event; start the retained-log request; continue event consumption while the ticket queues increments; complete only with that ticket. On failed or cancelled fetch, log gap or recovery backlog, start a fresh ticket and fetch while the stream remains healthy. Discard stale tickets after selection, removal, recreation or reconnect. Only stream errors or sequence gaps require a new subscription. The caller controls retry/cancellation policy. This preserves the 64 KiB byte buffer, 128-event queue and stale-ticket checks.

TUI-SEC01 is recorded for Phase 1.6 in this report, the roadmap and security documentation. Sanitized log bytes are still untrusted terminal content; the future TUI must escape control characters, ANSI/OSC sequences and malformed UTF-8 while preserving raw bytes and offsets in the API client.

## Validation and limitations

The targeted post-fix run `go test -count=1 -timeout=60s -run 'TestClientOwnsTransportWithoutDefaultTransport|TestClientRejectsOversizedResponseHeaders|TestClientTransportConfiguration|TestSSERejectsInvalidRemovalAndLogIDs|TestSSELogRequiresContractFields|TestStateCallerRecoveryWorkflow' ./internal/apiclient` exited 0. Final validation:

| Command | Exit | Result |
| --- | ---: | --- |
| `go mod verify` | 0 | All modules verified. |
| `node cmd/cs2m/ui.test.cjs` | 0 | 10/10 JavaScript tests pass. |
| `go test -race -count=3 -timeout=180s ./internal/apiclient` | 0 | Repeated client race tests pass. |
| `go test -race -count=3 -timeout=180s ./...` | 0 | Whole Go suite passes three race runs. |
| `go test -coverpkg=./... -coverprofile=/tmp/cs2m-phase151-final.cover -count=1 ./...` and `go tool cover -func=/tmp/cs2m-phase151-final.cover` | 0, 0 | Cross-package coverage 81.8% versus 81.6% baseline. |
| `bash scripts/check.sh` | 0 | Format, vet, uncached Go tests, 10 Node tests, race, default coverage, build and whitespace check pass. |
| `git diff --check` | 0 | No whitespace errors. |

The suite now has 127 top-level Go tests (121 original names retained plus six new review tests) and 10 JavaScript tests. Final default coverage is 76.3% versus locally measured 76.1% and published 76.2%. Cross-package coverage is 81.8% versus 81.6%. Coverage is diagnostic. No manual retest is required for unchanged browser and server behavior; none was performed. The client does not automatically reconnect or retry log recovery, and the retained ring cannot recover expired bytes. Native CS2 remains PENDING.

Final Git state: branch `feat/headless-api` at `60e896ec689a5efe68e92898acd5347b984a240c`; tracked edits are `docs/API.md`, `docs/ARCHITECTURE.md`, `docs/PROJECT_STATUS.md`, `docs/ROADMAP.md`, `docs/SECURITY.md`, `docs/TESTING.md`, `internal/apiclient/client.go`, and `internal/apiclient/stream.go`. New files are this report and `internal/apiclient/review_test.go`. The two unrelated skin scripts remain untracked and untouched. No commit, push, merge or deployment was performed.
