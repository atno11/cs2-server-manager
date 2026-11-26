# Architecture

## Approved direction

Linux-first, API-first modular monolith for multiple local CS2 instances. The core daemon exposes the public REST and SSE API and must remain independent of any client UI. The TUI is the first planned official interactive client; a CLI for administrative automation and an optional default Web UI are planned later. Custom clients may implement their own interfaces through the documented public API. The current embedded HTML/JavaScript page is a temporary functional validation client, not the final product UI. A future TUI must consume the public API rather than manager internals. Remote hosts are deferred. This direction does not claim that the Phase 1.5 headless consolidation or TUI has been implemented.

## Phase 0 inventory (historical)

`cmd/cs2m` starts an HTTP server on `127.0.0.1:8080` by default, rejects non-loopback bind IPs, rejects root execution, serves an HTML page, and defines instance REST handlers. `internal/manager` validates IDs, paths, ports and arguments; stores definitions in an atomically replaced JSON file; supervises child processes with `exec.CommandContext`; serializes manager state with a mutex; captures each instance's output in a bounded 64 KiB memory buffer. There are no nonstandard Go dependencies. Current tests use temporary shell scripts, not CS2.

At that point SQLite and SSE were approved architecture but had not been implemented. There is still no remote agent or native CS2 validation. Later phase sections describe the current implementation.

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

Phase 1.3.1 initializes SQLite PRAGMAs through the driver DSN on each physical connection. Version 1 startup verifies table columns, expected indexes, migration rows and constraint behavior without changing committed data. Loads check SQLite byte lengths and storage types before scanning rows; the manager applies its existing policy validation afterward. See [Phase 1.3.1](phases/phase-01-3-1.md).

## Phase 1.4 real-time events

`internal/events` is a bounded, concurrency-safe broker. It has no database, process or HTTP responsibilities. The manager publishes public registration/removal and state changes while holding its state mutex, after committed persistence where applicable. The existing log ring publishes only finalized sanitized bytes while holding its own mutex; publication never waits for network writes. The broker lock is never held while acquiring a manager or ring lock. The manager takes its mutex before subscribing and copying a snapshot, making the snapshot sequence boundary atomic with state publication. Logs have monotonic per-instance byte offsets, allowing the browser to reconcile retained-log REST snapshots with queued incremental events. `cmd/cs2m` owns authentication, SSE framing, write deadlines and browser assets. See [Phase 1.4](phases/phase-01-4.md).
