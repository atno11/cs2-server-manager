# CS2 Server Manager MVP

Local, single-host CS2 process manager. Run as an unprivileged user with `go run ./cmd/cs2m`. The API and web UI listen on `127.0.0.1:8080` by default. Set `CS2M_ADDR` to another loopback address and `CS2M_DATA` to a private data directory if needed. Public bind addresses are rejected. State is stored in `instances.json` with mode 0600. Running processes are stopped on manager shutdown and are not resumed automatically.

## API

- `GET /api/instances` lists definitions and states.
- `POST /api/instances` registers a definition: `{"id":"match1","executable":"/absolute/path/to/cs2","work_dir":"/absolute/path/to/server","port":27015,"args":["+map","de_dust2"]}`.
- `GET /api/instances/{id}` inspects one instance.
- `DELETE /api/instances/{id}` removes a stopped instance.
- `POST /api/instances/{id}/start`, `/stop`, or `/restart` changes its state.
- `GET /api/instances/{id}/logs` returns the last 64 KiB of captured output.

Executable and working directory must already exist. Arguments are passed directly to the process, with `-port <port>` appended by the manager. Shell execution and Steam downloads are not used. Avoid putting secrets in arguments: operating system process inspection can expose them. Use separate working directories and configuration files for each instance.

## Development baseline

The current MVP predates the approved SQLite and SSE architecture. These are planned for a later implementation phase. Read `docs/ARCHITECTURE.md`, `docs/PROJECT_STATUS.md`, `docs/TESTING.md`, and `docs/ROADMAP.md` before changing behavior. Run `scripts/check.sh` for the full baseline. The host initialized Git on `main` and ran `scripts/check.sh` successfully. Codex can read the repository but its `.git` mount remains read-only, so staging and committing require host-side access. Native CS2 tests remain pending.

## Lifecycle behavior

The manager allows one lifecycle operation per instance at a time. Conflicting requests return an error; independent instances can proceed. Stopping sends SIGTERM to the instance's Linux process group, then SIGKILL after two seconds, with a further two-second completion bound. Forced termination and timeout errors are visible in instance state. Manager shutdown rejects new mutations and stops active instances concurrently. Process-group supervision cannot reclaim detached descendants or processes left by abrupt manager termination. See `docs/phases/phase-01-1.md`.

Lifecycle review update: this build requires Linux 6.9+ pidfd process-group signaling and readable `/proc` for confirmed group cleanup. `Close` returns cleanup errors; forced termination can be followed by a restart once cleanup is confirmed, with diagnostics retained in `last_termination`. See `docs/phases/phase-01-1-1.md`.
