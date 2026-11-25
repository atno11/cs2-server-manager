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
