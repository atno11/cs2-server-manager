# Roadmap

## Phase 0

Read-only environment audit; see `phases/phase-00.md` and historical `../AUDIT.md`.

## Phase 0.1

Document existing MVP, establish regression baseline, and provide a validation entry point. No application feature changes. See `phases/phase-00-1.md`.

## Recommended Phase 1

With host-side Git initialized, review and preserve the existing MVP. Define a fixed Phase 1 checklist. Migrate definitions to SQLite with explicit migration and rollback planning; tighten lifecycle, input and secret handling; add concurrent and failure-path tests; add SSE with bounded delivery and a minimal frontend. Verify each increment against the baseline. Keep native CS2 checks pending until a verified installation and safe test plan exist. Remote hosts, downloads, RCON, scheduling, and deployment remain out of scope.

## Phase 1.1

Lifecycle hardening completed with fake-process regression coverage. See `phases/phase-01-1.md`. The next phase should address the remaining approved scope in separately bounded increments; do not treat simulated process tests as native CS2 validation.

## Phase 1.1.1

Independent lifecycle review fixes are implemented and simulated checks pass. See `phases/phase-01-1-1.md`. Merge readiness still requires normal host-side review and native CS2 validation remains pending; Phase 1.2 is not started.

## Phase 1.1.2

FLAKY-001 was reproduced, fixed narrowly and validated with 100 normal and 50 race-detector repetitions of the affected test. See `phases/phase-01-1-2.md`. The next action is host-side commit and review of the stability fix before any merge decision; Phase 1.2 remains deferred.

## Phase 1.2

Security hardening is implemented for a single local administrator: bearer authentication, origin/authority checks, strict HTTP input, trusted execution policy, private persistence and known-secret log redaction. See `phases/phase-01-2.md`. Next, review and commit this phase on the host, then plan a separately scoped increment. SQLite, SSE and native CS2 validation remain pending; no remote-host feature is included.

## Phase 1.2.1

Independent security review fixes are implemented and covered by controlled tests. See `phases/phase-01-2-1.md`. Host-side review and commit are next; native CS2 compatibility must be validated separately before claiming integration readiness. SQLite, SSE and remote-host management remain deferred.

## Phase 1.3

SQLite definition persistence and one-time legacy JSON import are implemented on the feature branch. See `phases/phase-01-3.md` for validation and migration limits. Native CS2 testing and SSE remain separate future work; this phase does not add a process-adoption or remote-host mechanism.

Phase 1.3.1 closes the storage static-review findings on the same feature branch before merge review. Its evidence and residual limits are in `phases/phase-01-3-1.md`. Phase 1.4, SSE and native CS2 validation remain separate work.

Phase 1.4 adds bounded SSE state and sanitized log delivery on `feat/realtime-events`, with a single authenticated browser stream. Its implementation and validation are recorded in `phases/phase-01-4.md`. Native CS2 validation and later roadmap work remain separate.

Phase 1.4.1 reviews effective SSE handler capacity, log reconciliation and write deadline failures before merge. Confirmed defects and regression evidence are recorded in `phases/phase-01-4-1.md`. This review does not start Phase 1.5 or change the approved sequence below.

## Approved sequence after Phase 1.4

The product direction is API-first and headless. The daemon must remain independent of client UIs. The current HTML/JavaScript page is a temporary validation client. The first official interactive client is the TUI, which must consume the public API rather than manager internals. A CLI for administrative automation remains planned; the optional default Web UI is a later client. Custom clients may use the documented public REST and SSE API.

| Phase | Scope |
| --- | --- |
| 1.4 | Real-time Events / SSE — implementation and user-reported interactive validation complete. |
| 1.5 | Headless Core & API Consolidation. |
| 1.6 | TUI MVP. |
| 1.7 | Native CS2 Validation. |
| 2.0 | SteamCMD Integration. |
| 3.0 | RCON / A2S. |
| 4.0 | Configuration Management. |
| 5.0 | Plugin Management. |
| 6.0 | Monitoring / Automation. |
| 7.0 | Optional Default Web UI. |
| 8.0 | Remote Host Management. |

CLI development remains planned without a phase assignment in this sequence. Native CS2 validation is still PENDING. No work on Phase 1.5 is included in this Phase 1.4 closeout.
