# Project status

## Current state

Phase 0.1 is closed. The user initialized a Git repository on the host with `git init -b main` and ran `scripts/check.sh` successfully (exit 0). Codex can read the repository and confirms branch `main`, no commits, and all intended files untracked. The `.git` directory remains read-only inside the Codex sandbox, so staging and committing must occur host-side or after sandbox permissions change. No files were staged or committed in Phase 0.1. The existing MVP source and tests were preserved. Official host-side coverage baseline: 42.4% `cmd/cs2m`, 71.8% `internal/manager`, 63.4% overall. Native CS2 validation remains pending.

## Existing behavior and limits

Definitions are kept in `instances.json`, not SQLite. Registration, listing, inspection, removal, start, stop, restart, and bounded per-instance log retrieval are implemented. State is protected by a manager mutex; log buffers have separate mutexes. Instances are not automatically resumed after manager restart. The UI is minimal HTML with fetch requests; SSE is not implemented. The API binds to loopback by default and rejects non-loopback IPs, but has no authentication, so local users and browser-origin requests need a security review before broader use.

Known defects or risks from code inspection, not silently fixed in Phase 0.1: definitions and API responses include arbitrary `args`, which may contain secrets; log output may itself contain secrets. JSON persistence validates executable paths on load, so a missing installation can prevent startup. Stored definitions are not validated for duplicate ports or work directories against each other during load because each entry is inserted after validation of its own value; duplicate IDs can overwrite. Validation uses path existence and mode checks but does not prevent symlink or path replacement races. Stop waits up to 10 seconds, then kills; test coverage does not yet exercise concurrent lifecycle calls or separate log streams. `Close` stops processes sequentially. The current HTTP route tests cover registration, inspection, logs and deletion, not all lifecycle routes. No native CS2 integration has been performed.

Go tests and build work with temporary `GOCACHE` and `GOTMPDIR`. Host-side `scripts/check.sh` passed all checks, including standard build and Git whitespace validation. The earlier Codex-side run occurred before host Git initialization and exited 1 because Git-dependent checks were blocked; that history is retained in `phases/phase-00-1.md`. Existing historical checklist claims in `../AUDIT.md` should be read with their stated evidence limits.

## Phase 1.1 status

Process lifecycle hardening is implemented and simulated validation passes. Each instance now has an operation reservation, an isolated Linux process group, bounded graceful and forced shutdown waits, and explicit completion ownership. Manager shutdown is shared and parallel; HTTP shutdown completes before manager cleanup. `Args` slices are copied at input and output boundaries. The full phase report and fixed PL checklist are in `phases/phase-01-1.md`. Final `bash scripts/check.sh` exited 0; overall statement coverage was 78.5% versus the official 63.4% Phase 0.1 baseline. Native CS2 validation remains pending. The earlier Phase 0.1 status above is retained as historical context.

This phase does not change JSON persistence, authentication, or secret handling. Detached descendants, abrupt manager termination, and possible process-group ID reuse after leader reaping remain limitations. The branch is `feat/process-lifecycle-hardening` at base commit `2752782`; Codex's `.git` mount is read-only, so this work remains unstaged and uncommitted.
