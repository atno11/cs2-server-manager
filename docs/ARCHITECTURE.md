# Architecture

## Approved direction

Linux-first Go modular monolith for multiple local CS2 instances, HTTP REST API, SQLite persistence, minimal web frontend, and SSE for live events and logs. Remote hosts are deferred. These are architecture decisions for future implementation, not all current behavior.

## Verified existing implementation

`cmd/cs2m` starts an HTTP server on `127.0.0.1:8080` by default, rejects non-loopback bind IPs, rejects root execution, serves an HTML page, and defines instance REST handlers. `internal/manager` validates IDs, paths, ports and arguments; stores definitions in an atomically replaced JSON file; supervises child processes with `exec.CommandContext`; serializes manager state with a mutex; captures each instance's output in a bounded 64 KiB memory buffer. There are no nonstandard Go dependencies. Current tests use temporary shell scripts, not CS2.

SQLite and SSE are approved architecture but are not implemented. There is no remote agent or native CS2 validation. Current process and API details are inventoried in `PROJECT_STATUS.md`. Avoid interpreting the approved direction as a claim about existing behavior.

## Phase 1.1 lifecycle supervision

The registry mutex protects definitions, state, and one operation reservation per instance; process waits occur outside it. Linux process groups scope TERM and KILL to a managed instance. A completion handler validates the process pointer before changing state. `Close` blocks new mutations, waits for reserved operations, stops active groups concurrently, and shares one completion channel with all callers. See `phases/phase-01-1.md` for state transitions, time bounds, tests, and process-group limits.
