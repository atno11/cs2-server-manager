# CS2 Server Manager

Linux single-host CS2 process manager. Run as an unprivileged user. The HTTP server listens on `127.0.0.1:8080` by default; `CS2M_ADDR` may select another numeric loopback address. `CS2M_DATA` selects the private runtime directory (default `~/.local/share/cs2m`). Public binds are rejected. Go 1.27 and Linux pidfd group signaling with readable `/proc` are required.

The approved product direction is an API-first, headless daemon independent of client UIs. The TUI is the first planned official interactive client; a CLI for automation and an optional default Web UI are planned later. Custom clients can use the public REST and SSE API. The current embedded HTML/JavaScript page is a temporary functional validation client. No TUI or headless-core refactor is part of Phase 1.4.

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

On first start, the manager generates a random 0600 `admin.token` in the runtime directory. Read this file locally as the administrator, enter its value in the web UI, or send it in an `Authorization: Bearer` header from a programmatic client. Keep it out of URLs, shell history and logs. The UI holds the token only in page memory; closing or refreshing the page clears it. All `/api/` routes require it. The token is not printed by the application. See [security policy](docs/SECURITY.md) for the threat model and limitations.

## API

- `GET /api/instances` lists public states.
- `POST /api/instances` registers an instance using JSON: `{"id":"match1","executable":"/srv/cs2/game/bin/linuxsteamrt64/cs2","work_dir":"/srv/cs2-instances/match1","port":27015,"args":[]}`.
- `GET /api/instances/{id}` inspects an instance; `DELETE` removes a stopped one.
- `POST /api/instances/{id}/start`, `/stop`, and `/restart` control it.
- `GET /api/instances/{id}/logs` returns the last 64 KiB of redacted output.
- `GET /api/events` streams public instance changes and sanitized log bytes as Server-Sent Events. Use `Authorization: Bearer` in the request header. A fresh connection begins with an authoritative `snapshot` event; the browser reconnects automatically.

API errors return status codes with empty bodies. The HTTP `args` field accepts only limited positional values. Operator-controlled CS2 flags belong in `base_args`; the manager appends `-port <port>`. Each instance uses its own existing working directory. Definitions persist in private `instances.db`. On the first SQLite start, a valid private `instances.json` is imported once, without changing the original file; subsequent starts use SQLite only. Legacy import rejects duplicate JSON keys, more than 32 definitions, files over 512 KiB and individual compact definitions over 16 KiB. Running processes are stopped on orderly manager shutdown and are not resumed automatically. Only one manager may own a runtime directory at a time. Remote agents and downloads are deferred.

## Development

Read [architecture](docs/ARCHITECTURE.md), [status](docs/PROJECT_STATUS.md), [testing](docs/TESTING.md), and [roadmap](docs/ROADMAP.md) before changing behavior. Run `bash scripts/check.sh` for the full regression baseline. Native CS2 validation remains pending.
