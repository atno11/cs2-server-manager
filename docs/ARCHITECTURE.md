# Architecture

## Approved direction

Linux-first Go modular monolith for multiple local CS2 instances, HTTP REST API, SQLite persistence, minimal web frontend, and SSE for live events and logs. Remote hosts are deferred. These are architecture decisions for future implementation, not all current behavior.

## Verified existing implementation

`cmd/cs2m` starts an HTTP server on `127.0.0.1:8080` by default, rejects non-loopback bind IPs, rejects root execution, serves an HTML page, and defines instance REST handlers. `internal/manager` validates IDs, paths, ports and arguments; stores definitions in an atomically replaced JSON file; supervises child processes with `exec.CommandContext`; serializes manager state with a mutex; captures each instance's output in a bounded 64 KiB memory buffer. There are no nonstandard Go dependencies. Current tests use temporary shell scripts, not CS2.

SQLite and SSE are approved architecture but are not implemented. There is no remote agent or native CS2 validation. Current process and API details are inventoried in `PROJECT_STATUS.md`. Avoid interpreting the approved direction as a claim about existing behavior.

## Phase 1.1 lifecycle supervision

The registry mutex protects definitions, state, and one operation reservation per instance; process waits occur outside it. Linux process groups scope TERM and KILL to a managed instance. A completion handler validates the process pointer before changing state. `Close` blocks new mutations, waits for reserved operations, stops active groups concurrently, and shares one completion channel with all callers. See `phases/phase-01-1.md` for state transitions, time bounds, tests, and process-group limits.

## Phase 1.1.1 review correction

The independent review identified an unsafe post-reap process-group signal in Phase 1.1. Supervision now keeps the leader waitable with `waitid(WNOWAIT)` until group cleanup completes, and uses a pidfd process-group signal rather than a negative PGID. Close returns a shared result; HTTP shutdown errors are propagated. See `phases/phase-01-1-1.md` for ownership requirements and limits. The prior Phase 1.1 description is historical.

## Phase 1.2 security boundary

The local operator supplies `policy.json`, defining exact executable identities and working roots. `internal/manager` pins trusted executable device/inode/hash, revalidates before launch, and executes through an open descriptor. HTTP registration cannot modify this policy. `internal/security` creates and checks a private random admin bearer token. `cmd/cs2m` enforces exact loopback Host and Origin, authentication, strict routes and JSON, connection/request limits, and a public response model. Known secrets are redacted from bounded logs. Linux supervision capabilities are probed before managed launches. See [SECURITY.md](SECURITY.md) and [Phase 1.2](phases/phase-01-2.md) for limits and evidence.

## Phase 1.2.1 review correction

The trust-policy boundaries are rechecked at each launch, including root metadata and executable privilege attributes. All JSON input paths use duplicate-key detection. Persisted definitions are bounded before loading. The redactor tracks overlapping secret intervals in captured logs; public instance values are validated before serialization, and API errors have no body. Descriptor execution retains the authorized path in `argv[0]`; native CS2 compatibility is still unverified. See `phases/phase-01-2-1.md`.

## Phase 1.3 SQLite definition storage

`internal/storage` owns `database/sql`, SQLite schema/versioning, migration markers, parameterized definition writes, file checks and the runtime lock. It stores only ID, executable path, working directory, port and ordered arguments. `internal/manager` still validates definitions, owns process handles/states/logs, and uses a small concrete storage boundary for atomic Register/Remove operations. `policy.json` and `admin.token` stay outside the database. A nonblocking `flock` on private `instances.lock` permits one manager for a runtime directory; external SQLite writers remain outside the supported operating model. See [Phase 1.3](phases/phase-01-3.md).
