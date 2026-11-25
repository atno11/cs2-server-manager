# Roadmap

## Phase 0

Read-only environment audit; see `phases/phase-00.md` and historical `../AUDIT.md`.

## Phase 0.1

Document existing MVP, establish regression baseline, and provide a validation entry point. No application feature changes. See `phases/phase-00-1.md`.

## Recommended Phase 1

With host-side Git initialized, review and preserve the existing MVP. Define a fixed Phase 1 checklist. Migrate definitions to SQLite with explicit migration and rollback planning; tighten lifecycle, input and secret handling; add concurrent and failure-path tests; add SSE with bounded delivery and a minimal frontend. Verify each increment against the baseline. Keep native CS2 checks pending until a verified installation and safe test plan exist. Remote hosts, downloads, RCON, scheduling, and deployment remain out of scope.
