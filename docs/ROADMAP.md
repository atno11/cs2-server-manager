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
