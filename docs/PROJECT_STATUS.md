# Project status

## Phase 0 inventory (historical)

Phase 0.1 is closed. The user initialized a Git repository on the host with `git init -b main` and ran `scripts/check.sh` successfully (exit 0). Codex can read the repository and confirms branch `main`, no commits, and all intended files untracked. The `.git` directory remains read-only inside the Codex sandbox, so staging and committing must occur host-side or after sandbox permissions change. No files were staged or committed in Phase 0.1. The existing MVP source and tests were preserved. Official host-side coverage baseline: 42.4% `cmd/cs2m`, 71.8% `internal/manager`, 63.4% overall. Native CS2 validation remains pending.

## Existing behavior and limits

Definitions are kept in `instances.json`, not SQLite. Registration, listing, inspection, removal, start, stop, restart, and bounded per-instance log retrieval are implemented. State is protected by a manager mutex; log buffers have separate mutexes. Instances are not automatically resumed after manager restart. The UI is minimal HTML with fetch requests; SSE is not implemented. The API binds to loopback by default and rejects non-loopback IPs, but has no authentication, so local users and browser-origin requests need a security review before broader use.

Known defects or risks from code inspection, not silently fixed in Phase 0.1: definitions and API responses include arbitrary `args`, which may contain secrets; log output may itself contain secrets. JSON persistence validates executable paths on load, so a missing installation can prevent startup. Stored definitions are not validated for duplicate ports or work directories against each other during load because each entry is inserted after validation of its own value; duplicate IDs can overwrite. Validation uses path existence and mode checks but does not prevent symlink or path replacement races. Stop waits up to 10 seconds, then kills; test coverage does not yet exercise concurrent lifecycle calls or separate log streams. `Close` stops processes sequentially. The current HTTP route tests cover registration, inspection, logs and deletion, not all lifecycle routes. No native CS2 integration has been performed.

Go tests and build work with temporary `GOCACHE` and `GOTMPDIR`. Host-side `scripts/check.sh` passed all checks, including standard build and Git whitespace validation. The earlier Codex-side run occurred before host Git initialization and exited 1 because Git-dependent checks were blocked; that history is retained in `phases/phase-00-1.md`. Existing historical checklist claims in `../AUDIT.md` should be read with their stated evidence limits.

## Phase 1.1 status

Process lifecycle hardening is implemented and simulated validation passes. Each instance now has an operation reservation, an isolated Linux process group, bounded graceful and forced shutdown waits, and explicit completion ownership. Manager shutdown is shared and parallel; HTTP shutdown completes before manager cleanup. `Args` slices are copied at input and output boundaries. The full phase report and fixed PL checklist are in `phases/phase-01-1.md`. Final `bash scripts/check.sh` exited 0; overall statement coverage was 78.5% versus the official 63.4% Phase 0.1 baseline. Native CS2 validation remains pending. The earlier Phase 0.1 status above is retained as historical context.

This phase does not change JSON persistence, authentication, or secret handling. Detached descendants, abrupt manager termination, and possible process-group ID reuse after leader reaping remain limitations. The branch is `feat/process-lifecycle-hardening` at base commit `2752782`; Codex's `.git` mount is read-only, so this work remains unstaged and uncommitted.

## Phase 1.1.1 review status

The independent lifecycle review is resolved in code and simulated validation. See `phases/phase-01-1-1.md` for the fixed LCR checklist, one reproduced stale-completion defect, corrected process-group ownership, eight new tests, and the disclosed intermittent race-test failure that did not recur in later runs. Final `bash scripts/check.sh` exited 0; overall coverage is 76.5% against the published Phase 1.1 78.5% baseline. Linux 6.9+ pidfd process-group signaling and readable `/proc` are required for confirmed cleanup. Native CS2 validation remains pending. Branch `feat/process-lifecycle-hardening` is based on `b7b7089`; current changes are unstaged because Codex sees `.git` read-only.

## Phase 1.1.2 stability validation

FLAKY-001 was reproduced and traced to a transient `ESRCH` from `/proc/<pid>/stat` during group-member scanning. A deterministic test failed before the narrow fix and passed after it. Post-fix affected-test runs passed 100/100 normally and 50/50 with the race detector; full race and `scripts/check.sh` validation passed. See `phases/phase-01-1-2.md` for the original failure counts, exact commands, resource evidence and remaining limits. Final coverage is 76.6% overall versus 76.5% at phase start. Native CS2 validation remains pending. Branch `feat/process-lifecycle-hardening` is based on `8d32143`; Codex's `.git` mount is read-only and this phase's changes are unstaged.

## Phase 1.2 current status

The historical sections above describe earlier phases and their then-current limits. Phase 1.2 now requires an owner-only operator `policy.json`, generates a private bearer token, protects all API routes, limits HTTP resources and instance registration, validates exact executable identity and trusted roots, redacts known log secrets, and preflights Linux supervision before launch. See [security model](SECURITY.md) and [Phase 1.2 report](phases/phase-01-2.md). JSON persistence and the existing lifecycle manager remain. Native CS2 validation remains pending. Codex's `.git` mount is read-only; work is unstaged on `feat/security-hardening` at `7f68f77`.

## Phase 1.2.1 security review status

The independent review found reproducible redaction, root/metadata revalidation, persisted count, duplicate-JSON and public-ID exposure defects. Narrow fixes and controlled regression tests are recorded in [Phase 1.2.1](phases/phase-01-2-1.md). The manager now preserves the trusted original `argv[0]` with descriptor execution. Historical Phase 1.2 conclusions above remain intact; this section records the later correction. Native CS2 compatibility and validation remain pending.

Final Phase 1.2.1 validation: 59 top-level tests, full race count=3 exit 0, `bash scripts/check.sh` exit 0, overall statement coverage 78.6% versus the local 77.0% pre-change measurement. All SCR criteria passed under the documented local-administrator threat model. Native CS2 validation remains pending. Codex's `.git` mount is read-only; changes are unstaged on `feat/security-hardening` at `a0e8b20`.


## Phase 1.2.1 HTTP response follow-up

The final review follow-up removes serialized-response redaction through the log ring. Public JSON field names remain stable; API errors have status-only responses. The existing log limit and known-secret capture redaction remain. See the appended SRF checklist and evidence in [Phase 1.2.1](phases/phase-01-2-1.md). Native CS2 validation remains pending.


Final HTTP response follow-up validation: SRF-01 through SRF-05 PASS, 62 top-level tests, full race count=3 exit 0, `bash scripts/check.sh` exit 0, overall coverage 79.8% versus local pre-change 78.5%. Native CS2 validation remains pending.

## Phase 1.3 SQLite persistence

Active definition persistence has moved from `instances.json` to private `instances.db` with a versioned schema. A transactional, one-time migration imports valid legacy JSON and leaves its bytes unchanged; subsequent starts use SQLite exclusively. The manager keeps live states, process handles and bounded logs in memory. A private lock file enforces one manager per runtime directory. Earlier JSON descriptions above are historical. See [Phase 1.3](phases/phase-01-3.md) for validation and limitations. Native CS2 validation remains pending.

Phase 1.3.1 storage review fixes are implemented on `feat/sqlite-persistence`: connection settings are applied on every driver connection, existing version 1 schemas are checked before use, and stored values are bounded by bytes and type before loading. See [Phase 1.3.1](phases/phase-01-3-1.md) for acceptance evidence. Native CS2 validation remains pending.

Phase 1.3 simulated validation passes: 81 top-level tests (the original 62 retained), full race count=3 exit 0, and the final read-only-module `scripts/check.sh` exit 0. Overall coverage is 79.4% against the local 78.7% pre-change run and published 79.8% baseline. Host-side `go mod tidy` and `go mod verify` both exited 0; sandbox read-only dependency resolution, module verification and build passed. DB-01 through DB-18 are PASS with criterion evidence in the phase report. Native CS2 validation remains pending.

## Phase 1.4 real-time events

The feature branch `feat/realtime-events` adds an authenticated `GET /api/events` stream and updates the existing browser page from snapshots, instance changes and sanitized log bytes. REST, SQLite and process supervision remain in place. The stream has bounded subscribers and queues; slow subscribers are disconnected and reconnect from a new snapshot. See [Phase 1.4](phases/phase-01-4.md) for evidence and remaining limits. Native CS2 validation remains pending.

Phase 1.4 automated acceptance is complete: SSE-01 through SSE-18 PASS, 103 Go top-level tests, seven JavaScript tests and reported 81.1% overall Go coverage. The user subsequently reported SSE-W01 through SSE-W12 PASS from interactive browser validation. An initial HTTP 401 was resolved by using the token for the active isolated runtime; no code change was required. Native CS2 validation remains PENDING. The approved next architectural direction is an API-first headless core, with the TUI as the first official interactive client and a CLI planned for future automation. The embedded browser page remains a temporary validation client.

## Phase 1.4.1 event streaming review

Independent review confirmed that broker queue overflow could free a subscriber slot before its blocked HTTP handler exited, and that SSE ignored write deadline setup failures. The follow-up adds an independent eight-handler reservation and fails safely when required deadline setup or refresh fails. Deterministic browser tests did not reproduce log contamination across removal, same-ID recreation, selection changes or a fresh reconnect snapshot, so the public log contract remains unchanged. See [Phase 1.4.1](phases/phase-01-4-1.md) for reproduction and validation evidence. Native CS2 validation remains PENDING.

Phase 1.4.1 final acceptance: SSER-01 through SSER-06 PASS; the user reported targeted interactive retests SSE-W01, SSE-W08, SSE-W09 and SSE-W11 PASS (4/4). Automated validation reported 106 Go top-level tests, 10 JavaScript tests and 81.3% overall Go statement coverage. Native CS2 validation remains PENDING.
