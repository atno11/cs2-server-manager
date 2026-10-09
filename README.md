# CServer Manager

A modular, cross-platform Counter-Strike 2 dedicated server manager.

> Development status: 0.1.0-dev

## Overview

CServer Manager aims to replace an existing Bash-based
server management solution with a modular Go application.

The application provides three interface entry points:

- CLI for direct commands.
- TUI for interactive terminal navigation.
- HTTP REST API for external integrations.

All interfaces share application services and domain models.

The current version supports read-only CS2 server discovery
and optional OverlayFS and systemd mount-unit configuration
metadata.

Configuration discovery does not verify runtime activity.

Server lifecycle operations are not implemented yet.

## Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go |
| TUI | Bubble Tea v2 |
| Styling | Lip Gloss v2 |
| HTTP API | Go net/http |
| Configuration | Environment variables |
| Logging | Go log/slog |
| Testing | Go testing |
| API contract | OpenAPI 3.0.3 |
| Docker | Docker SDK (planned) |

## Project Structure

- `cmd/cserver`: Application entry point.
- `internal/app`: Shared initialization and logging.
- `internal/core`: Domain models and validation.
- `internal/service`: Shared application services.
- `internal/infrastructure`: Read-only infrastructure adapters.
- `internal/interfaces/cli`: Command-line interface.
- `internal/interfaces/http`: HTTP API adapter.
- `internal/interfaces/tui`: Interactive terminal UI.
- `internal/config`: Application configuration.
- `api/openapi.yaml`: HTTP API contract.
- `docs`: Architecture and development documentation.

See [Architecture](docs/architecture.md),
[Server Discovery](docs/server-discovery.md) and
[Infrastructure Discovery](docs/discovery-infrastructure.md).

## Requirements

- A Go version compatible with the installed dependencies.
- Make (optional).
- A compatible terminal for interactive TUI usage.

Docker and systemd are not required for the implemented
read-only discovery or automated tests.

## Build

Download dependencies:

```bash
go mod download
```

Build on Linux or macOS:

```bash
go build -o bin/cserver ./cmd/cserver
```

Build on Windows:

```powershell
go build -o bin/cserver.exe ./cmd/cserver
```

Alternatively, on Unix-like systems:

```bash
make build
```

## CLI

Display help:

```bash
./bin/cserver
```

Display application version:

```bash
./bin/cserver version
```

Display configuration:

```bash
./bin/cserver config show
```

Discover existing CS2 server instances:

```bash
CSERVER_ROOT=/path/to/cs2 ./bin/cserver servers list
```

The discovery command returns a JSON array.

Optional infrastructure metadata appears in the
`infrastructure` field of each applicable server.

The command does not query Docker runtime or modify
server configuration.

## Terminal UI

Start the interactive terminal interface:

```bash
./bin/cserver tui
```

Keyboard shortcuts:

| Key | Action |
|-----|--------|
| Up / k | Previous item or scroll |
| Down / j | Next item or scroll |
| Enter | Open selected screen |
| r | Refresh discovery on the Servers screen |
| Esc / Backspace | Return to main menu |
| q | Quit or return to main menu |
| Ctrl+C | Quit immediately |

Available screens:

- Main Menu
- Application Information
- Configuration
- Servers

The Servers screen displays available configuration
metadata for the first visible server.

The terminal UI remains read-only.

## HTTP API

Start the local API:

```bash
./bin/cserver api serve
```

Default address:

```text
127.0.0.1:8080
```

Available endpoints:

| Method | Path | Description |
|--------|------|-------------|
| GET | /healthz | Application health |
| GET | /api/v1/info | Application metadata |
| GET | /api/v1/servers | Read-only server discovery |

Example requests:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/api/v1/info
curl http://127.0.0.1:8080/api/v1/servers
```

The API does not expose lifecycle operations.

The OpenAPI contract is available at:

```text
api/openapi.yaml
```

The API contract is not served as an HTTP endpoint.

## Configuration

Configuration is loaded from environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| CSERVER_ROOT | Empty | CS2 instance root directory |
| CSERVER_OVERLAY_ROOT | Empty | External OverlayFS storage root |
| CSERVER_SYSTEMD_UNIT_DIRS | Empty | External systemd unit directories |
| CSERVER_HTTP_ADDRESS | 127.0.0.1:8080 | HTTP listener |
| CSERVER_LOG_LEVEL | info | Logging level |
| CSERVER_LOG_FORMAT | text | Logging output format |

CSERVER_SYSTEMD_UNIT_DIRS accepts an operating-system
path-list separated sequence of absolute directories.

No infrastructure paths are hardcoded.

Supported logging levels:

- debug
- info
- warn
- error

Supported logging formats:

- text
- json

Example:

```bash
export CSERVER_ROOT=/example/servers
export CSERVER_OVERLAY_ROOT=/example/overlay
export CSERVER_SYSTEMD_UNIT_DIRS=/example/units
export CSERVER_HTTP_ADDRESS=127.0.0.1:9090
export CSERVER_LOG_LEVEL=debug
export CSERVER_LOG_FORMAT=json
```

Configuration values can be inspected with:

```bash
./bin/cserver config show
```

Configuration validation does not create or modify the
configured directories.

An inaccessible or missing instance root produces an error.

Optional infrastructure discovery may report safe warnings
when configured sources are unavailable.

The HTTP listener is restricted to 127.0.0.1 until
authentication and remote-access security are implemented.

## Server Discovery

The basic filesystem adapter inspects immediate
subdirectories of CSERVER_ROOT.

It identifies CS2 candidates using known configuration
evidence, including environment settings and Compose files.

A discovered instance may be complete or incomplete.

Optional infrastructure discovery identifies OverlayFS
directory evidence and systemd overlay mount-unit metadata.

Infrastructure metadata includes a runtime_state field
whose value is always not_checked in this release.

No field indicates whether a server is running, mounted,
healthy or reachable.

Passwords, authentication tokens and raw environment
or unit contents are not included in discovery results.

See [Server Discovery](docs/server-discovery.md) and
[Infrastructure Discovery](docs/discovery-infrastructure.md).

## Logging

CServer Manager uses the Go standard library slog package.

Operational HTTP API logs are written to stderr.

JSON logging can be enabled with:

```bash
CSERVER_LOG_FORMAT=json ./bin/cserver api serve
```

Debug logging can be enabled with:

```bash
CSERVER_LOG_LEVEL=debug ./bin/cserver api serve
```

CLI command results remain on stdout.

Logging must not expose passwords, tokens or secrets.

## Development

Format code:

```bash
go fmt ./...
```

Run tests:

```bash
go test ./...
```

Run static analysis:

```bash
go vet ./...
```

Run race detection:

```bash
go test -race ./...
```

Build:

```bash
go build -o .tmp/cserver ./cmd/cserver
```

Run standard checks:

```bash
make check
```

## Current Features

- Modular Go project foundation.
- Shared application initialization.
- Domain and configuration validation.
- CLI commands.
- Read-only HTTP API.
- Interactive TUI.
- Structured application logging.
- OpenAPI contract.
- Automated unit tests.
- Read-only filesystem server discovery.
- Shared discovery service across all interfaces.
- Incomplete-instance diagnostics.
- External OverlayFS configuration discovery.
- Read-only systemd mount-unit configuration discovery.
- Optional public infrastructure metadata.
- Explicit separation of configuration and runtime state.

## Planned Features

- Read-only Docker and Docker Compose runtime inspection.
- Verification of active OverlayFS mounts.
- Read-only systemd runtime state inspection.
- Server lifecycle operations.
- Server configuration management.
- CS2 installation and updates.
- Expanded HTTP REST API.

## Operational Safety

Server discovery does not perform administrative
operations on existing CS2 servers.

No containers, mounts or existing server configuration
files are modified by the implemented commands.

The HTTP API is restricted to the local loopback address.

Discovery responses can contain local filesystem paths
and should be treated as sensitive operational metadata.

The filesystem adapters are intended for trusted
installation directories.

They do not provide complete protection against
concurrent changes by untrusted local users.

## License

License selection is pending.
