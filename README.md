# CS2 Server Manager

Linux single-host CS2 process manager. Run as an unprivileged user. The HTTP server listens on `127.0.0.1:8080` by default; `CS2M_ADDR` may select another numeric loopback address. `CS2M_DATA` selects the private runtime directory (default `~/.local/share/cs2m`). Public binds are rejected. Go 1.27 and Linux pidfd group signaling with readable `/proc` are required.

The daemon runs headless by default and exposes the public REST and SSE API independently of any UI. Set `CS2M_VALIDATION_UI=1` to enable the temporary HTML/JavaScript validation page at `/` and `/ui.js`; other values leave those routes unavailable. The independent [local TUI](docs/TUI.md) is the first official interactive client and uses this API. A CLI for automation and an optional default Web UI are planned later. Custom clients can use the same API. Quitting the TUI does not stop the daemon or managed instances.

## Operator setup

Create a private runtime directory and a mode 0600 `policy.json` before starting the manager. Paths must be absolute, canonical, and free of symlink components. The executable and working roots must exist. Working directories are owned by the manager user and are not group or world writable; executable trees and files must be owned by root or the manager user and cannot be group or world writable.

```json
{
  "executables": ["/srv/cs2/game/bin/linuxsteamrt64/cs2"],
  "executable_roots": ["/srv/cs2"],
  "work_roots": ["/srv/cs2-instances"],
  "base_args": [],
  "known_secrets": []
}
```

The exact executable path is pinned by device, inode and SHA-256 digest for the manager lifetime. At each start, the trusted roots, executable ownership, mode and file capabilities are rechecked. Changing it requires an operator policy review and manager restart. The manager revalidates the pinned file before every start and launches through its open descriptor. Existing unsafe files or directories cause an explicit failure; permissions are not silently changed.

On first start, the manager generates a random 0600 `admin.token` in the runtime directory. Read this file locally as the administrator and send it in an `Authorization: Bearer` header from a programmatic client. The optional validation page accepts it in memory. Keep it out of URLs, shell history and logs. All `/api/` routes require it. The token is not printed by the application. See [security policy](docs/SECURITY.md) for the threat model and limitations.

## API

The [version 1 local API contract](docs/API.md) specifies routes, payloads, status codes, SSE framing, resource limits and reconnection. The reusable Go client in `internal/apiclient` supports every current REST administration route and a bounded pull-based SSE reader. The daemon needs no browser to operate.

- `GET /api/instances` lists public states.
- `POST /api/instances` registers an instance using JSON: `{"id":"match1","executable":"/srv/cs2/game/bin/linuxsteamrt64/cs2","work_dir":"/srv/cs2-instances/match1","port":27015,"args":[]}`.
- `GET /api/instances/{id}` inspects an instance; `DELETE` removes a stopped one.
- `POST /api/instances/{id}/start`, `/stop`, and `/restart` control it.
- `GET /api/instances/{id}/logs` returns the last 64 KiB of redacted output.
- `GET /api/events` streams public instance changes and sanitized log bytes as Server-Sent Events. Use `Authorization: Bearer` in the request header. A fresh connection begins with an authoritative `snapshot` event; the browser reconnects automatically.

API errors return status codes with empty bodies. The HTTP `args` field accepts only limited positional values. Operator-controlled CS2 flags belong in `base_args`; the manager appends `-port <port>`. Each instance uses its own existing working directory. Definitions persist in private `instances.db`. On the first SQLite start, a valid private `instances.json` is imported once, without changing the original file; subsequent starts use SQLite only. Legacy import rejects duplicate JSON keys, more than 32 definitions, files over 512 KiB and individual compact definitions over 16 KiB. Running processes are stopped on orderly manager shutdown and are not resumed automatically. Only one manager may own a runtime directory at a time. Remote agents and downloads are deferred.

## Development

Build the daemon with `go build -o /tmp/cs2m ./cmd/cs2m` and the interactive client with `go build -o /tmp/cs2m-tui ./cmd/cs2m-tui`. Start the daemon first, then run the TUI against its numeric loopback address. The TUI reads the existing private `admin.token` file and does not create credentials. See [TUI.md](docs/TUI.md) for configuration, controls and an isolated simulator validation procedure.

Read [architecture](docs/ARCHITECTURE.md), [status](docs/PROJECT_STATUS.md), [testing](docs/TESTING.md), and [roadmap](docs/ROADMAP.md) before changing behavior. Run `bash scripts/check.sh` for the full regression baseline; Node.js is needed for its validation-client JavaScript tests, not for running the daemon. Native CS2 validation remains pending.

The TUI's simulated interactive workflow (TUI-W01–TUI-W12) and native Ghostty Ctrl+Shift+V paste were user-reported PASS. See the [Phase 1.6](docs/phases/phase-01-6.md) and [Phase 1.6.1](docs/phases/phase-01-6-1.md) reports for evidence and remaining limits. Native CS2 validation remains pending.
