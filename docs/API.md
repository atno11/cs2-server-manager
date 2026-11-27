# Local public API — contract version 1

The daemon exposes one HTTP API on a numeric loopback address (default `http://127.0.0.1:8080`). Version 1 documents the current `/api/...` routes; no `/api/v1` alias or runtime version negotiation is implemented. Every API request requires `Authorization: Bearer <local-admin-token>`. Keep credentials in headers, never URLs. `admin.token` is an owner-only file in `CS2M_DATA`; the daemon does not print its value. The API is for one local administrator and does not provide remote-host or TLS deployment.

The server requires the exact configured Host authority. An Origin header, when supplied, must equal `http://<authority>`; cross-site and same-site Fetch Metadata are rejected. No permissive CORS response is sent. API errors have status-only responses with empty bodies. A rejected bearer request includes `WWW-Authenticate: Bearer`. All `/api/` responses set `Cache-Control: no-store`.

## Instance contract

Public instance JSON contains `id` (string), `port` (integer), `state` (string), and optional generic `error` and `last_termination` strings. Private executable paths, working directories, arguments, process handles, internal errors, policy data and credentials are never returned. List order and SSE snapshot order are by ID. Definitions are submitted only for registration:

```json
{"id":"match1","executable":"/srv/cs2/game/bin/linuxsteamrt64/cs2","work_dir":"/srv/cs2-instances/match1","port":27015,"args":[]}
```

The example paths are placeholders. The operator's execution policy must authorize the executable and working directory. IDs use 1–64 ASCII letters, digits, `_` or `-`; ports are 1–65535. There are at most 32 registered instances, with unique IDs, ports and working directories. Paths must be absolute and at most 4096 bytes each. Up to 32 positional arguments are accepted, each 1–256 bytes with a total of at most 4096 bytes, subject to the existing restricted character and execution-policy checks. The registration body is limited to 16 KiB. Definitions persist in private SQLite storage; process state and logs are in memory.

| Method and route | Success response | Route-specific errors and semantics |
| --- | --- | --- |
| `GET /api/instances` | `200` JSON array of public instances; `Content-Type: application/json` | Lists current manager state. |
| `POST /api/instances` | `201` public instance JSON; `Content-Type: application/json` | Requires `Content-Type: application/json` (optional UTF-8 charset). `400` for invalid JSON/definition or policy rejection, `413` for body over 16 KiB, `415` for unsupported media type. Successful registration follows committed persistence. |
| `GET /api/instances/{id}` | `200` public instance JSON | `404` if absent. |
| `DELETE /api/instances/{id}` | `204` empty body | `404` if absent; `409` if active or removal fails. Successful removal follows committed persistence. |
| `POST /api/instances/{id}/start` | `200` public instance JSON | `404` if absent; `409` for invalid lifecycle operation or launch failure. State changes are also observable through SSE. |
| `POST /api/instances/{id}/stop` | `200` public instance JSON | `404` if absent; `409` for invalid lifecycle operation or termination failure. |
| `POST /api/instances/{id}/restart` | `200` public instance JSON | `404` if absent; `409` for invalid lifecycle operation or failure. |
| `GET /api/instances/{id}/logs` | `200` sanitized text bytes; `Content-Type: text/plain; charset=utf-8`; decimal `X-Log-Start` and `X-Log-End` byte offsets | `404` if absent. At most the most recent 64 KiB of finalized sanitized output is available. `X-Log-End - X-Log-Start` equals the response byte length. |
| `GET /api/events` | `200` long-lived SSE; `Content-Type: text/event-stream; charset=utf-8`; `Cache-Control: no-store` | `503` if stream or broker capacity is full, snapshot creation fails, or required write deadlines cannot be set. A successful stream starts with an authoritative snapshot. |

All routes may also return `401` for missing or invalid bearer authentication, `403` for disallowed Origin or Fetch Metadata, `421` for incorrect Host, or `503` when the general request limit is full. Unknown paths return `404`; unsupported methods return `405`. These errors have empty bodies. The server accepts at most 64 active HTTP requests and 64 accepted connections; eight active SSE HTTP handlers leave capacity for administrative calls. The listener and request limits are independent.

## SSE wire format

Events use standard `id:`, `event:` and `data:` lines followed by a blank line. `data` is JSON. The initial event is `snapshot` with `{"instances":[...]}`. Its decimal ID is the sequence boundary. Later accepted events increment the process-local sequence by exactly one. A client that sees a gap must disconnect and obtain a fresh snapshot. Sequence IDs restart after daemon restart, and `Last-Event-ID` does not trigger replay.

| Event | JSON data |
| --- | --- |
| `snapshot` | `{"instances":[<public instance>,...]}` |
| `instance.registered` | `{"instance":<public instance>}` |
| `instance.state` | `{"instance":<public instance>}` |
| `instance.removed` | `{"id":"match1"}` |
| `instance.log` | `{"id":"match1","start":0,"end":3,"bytes":"YWJj"}`; `bytes` is base64 for sanitized bytes, and offsets count bytes, not characters. |

Heartbeat comments are `: heartbeat` followed by a blank line; they have no event ID and do not consume sequence numbers. Known secrets are redacted before log publication, including across process writes. Individual log chunks contain at most 2048 raw bytes, ordinary event JSON payloads at most 4096 bytes, and an initial snapshot at most 16 KiB. Each of at most eight broker subscribers has a 128-event queue. Queue overflow disconnects that subscriber instead of silently dropping events. A handler awaiting a blocked network write retains its separate eight-stream reservation until it exits. The stream uses five-second write deadlines refreshed around writes and a 15-second heartbeat; normal non-stream HTTP uses a 15-second server write timeout.

A reconnect starts with a new authoritative snapshot. Fetch retained logs for the selected instance, queue incremental log events during that fetch, and remove overlap by byte offsets. A stream error or sequence gap requires reconnection; a log gap or recovery backlog requires a new retained-log fetch on the healthy stream. Removal or same-ID recreation invalidates an older pending log response. Bytes older than the 64 KiB ring cannot be recovered. Events and logs are not persisted. Unknown secret values cannot be detected by the redactor.

## Compatibility policy

Version 1 preserves these route paths, methods, response shapes, status-only error contract and SSE event names. Adding optional JSON fields or new event names is compatible for clients that ignore unknown fields and events while still counting their sequence IDs. Removing or renaming fields, changing types, offset units, status semantics or sequence behavior is incompatible. A future incompatible contract should use a separately documented major-version route with a migration plan; this phase adds no alternate routes or negotiation. The application is pre-release, so compatibility promises are limited to the published and tested version 1 contract.

## Go client integration

`internal/apiclient` is the reusable local Go client. Construct it with an explicit numeric loopback endpoint and an already protected bearer token, then use contexts for each operation. It rejects URL credentials, paths, queries, fragments, redirects and ambient proxies. REST calls have a 15-second bound, response headers a 10-second bound and bodies a 64 KiB cap. SSE has no short global timeout; each stream is tied to its context, requires an initial snapshot, validates sequence progression and uses a 35-second read deadline refreshed after complete frames. Call `Stream.Close` and `Client.CloseIdleConnections` when finished.

The client provides `List`, `Get`, `Register`, `Remove`, `Start`, `Stop`, `Restart`, `Logs` and `Subscribe`. `StatusError` exposes the HTTP status without response bodies or credentials. A failed network response after a mutation has an ambiguous commit outcome; the client never retries a mutation automatically. Query `Get` or `List` to recover authoritative state. A stream read error or sequence gap requires an explicit new `Subscribe` call; there is no hidden reconnect goroutine. Authentication failures likewise require caller action.

`apiclient.State` applies validated events and replaces its instance map on a new snapshot. `BeginLog(id)` returns a ticket for one selected instance. While its retained-log request is in flight, `State.Apply` queues at most 128 relevant incremental events. `CompleteLog(ticket, snapshot)` rejects stale tickets after a new snapshot, removal, registration or selection and reconciles queued bytes by offset. `LogBuffer` retains at most 64 KiB, suppresses duplicates and overlap, and reports gaps. These helpers use the server's already sanitized bytes and do not implement another redactor.

Caller ordering is part of log recovery. Apply the initial or reconnected stream snapshot, check whether the selected ID still exists, and call `BeginLog(id)` **before reading another stream event**. Start `Logs(ctx, id)` for that ticket. Event reading may then continue concurrently: `State.Apply` queues selected log events during the fetch. On success, call `CompleteLog(ticket, snapshot)`; it discards duplicate bytes and rejects responses made stale by a newer snapshot, selection change, removal or same-ID registration. A delayed response must be completed only with its original ticket. On fetch failure or cancellation, `ErrLogGap`, or `ErrLogBacklog`, start a new `BeginLog`/`Logs` attempt for the still-selected instance; the stream itself need not reconnect. Do not retry indefinitely without a caller-controlled cancellation or retry budget. On `ErrStaleLog`, discard the old result and check current selection and state before starting another attempt. `State` methods synchronize concurrent access, but callers must serialize the order in which they apply stream events. Byte offsets and retained bytes must remain unchanged; terminal clients must separately escape untrusted control sequences before display.

The client owns a standard-library HTTP transport rather than inheriting `http.DefaultTransport`. It disables ambient proxy use, caps response headers at 16 KiB and connections per host at 16, and supports closing idle connections explicitly. REST body and stream frame limits remain separate.

Headless operation is the daemon default. Set `CS2M_VALIDATION_UI=1` only when the temporary browser validation page is needed; other values leave `/` and `/ui.js` unavailable. The public API remains available in either mode. Node.js is a development validation prerequisite for `scripts/check.sh`, not a daemon runtime dependency. Native CS2 compatibility remains PENDING.
