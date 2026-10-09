
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

The current version supports read-only filesystem discovery
of existing CS2 server instances.

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
- `internal/infrastructure`: Filesystem discovery and future adapters.
- `internal/interfaces/cli`: Command-line interface.
- `internal/interfaces/http`: HTTP API adapter.
- `internal/interfaces/tui`: Interactive terminal UI.
- `internal/config`: Application configuration.
- `api/openapi.yaml`: HTTP API contract.
- `docs`: Architecture and development documentation.

See [Architecture](docs/architecture.md) and
[Server Discovery](docs/server-discovery.md).

## Requirements

- A Go version compatible with the installed dependencies.
- Make (optional).
- A compatible terminal for interactive TUI usage.

Docker and systemd are not required for read-only discovery
or the automated tests.

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

It does not query Docker or modify server configuration.

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
| CSERVER_HTTP_ADDRESS | 127.0.0.1:8080 | HTTP listener |
| CSERVER_LOG_LEVEL | info | Logging level |
| CSERVER_LOG_FORMAT | text | Logging output format |

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
export CSERVER_ROOT=/path/to/cs2/servers
export CSERVER_HTTP_ADDRESS=127.0.0.1:9090
export CSERVER_LOG_LEVEL=debug
export CSERVER_LOG_FORMAT=json
```

Configuration values can be inspected with:

```bash
./bin/cserver config show
```

The configuration loader validates absolute paths
but does not create or modify the configured directory.

Discovery reports errors for an unconfigured or
inaccessible root.

The HTTP listener is restricted to 127.0.0.1
until authentication and remote-access security
are implemented.

## Server Discovery

The filesystem adapter inspects the immediate
subdirectories of CSERVER_ROOT.

It identifies CS2 candidates using known configuration
evidence, including per-instance environment settings
and Compose files.

A discovered instance may be complete or incomplete.

This describes filesystem configuration only.

It does not indicate whether the instance is running,
mounted, healthy, or reachable.

Passwords, authentication tokens, and raw environment
file contents are not included in discovery results.

See [Server Discovery](docs/server-discovery.md)
for behavior, security limitations, and testing details.

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

Logging must not expose passwords, tokens, or secrets.

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

## Planned Features

- Read-only Docker and Docker Compose runtime inspection.
- OverlayFS metadata and mount inspection.
- systemd unit discovery and state inspection.
- Server lifecycle operations.
- Server configuration management.
- CS2 installation and updates.
- Expanded HTTP REST API.

## Operational Safety

Server discovery does not perform administrative
operations on existing CS2 servers.

No containers, mounts, or existing server configuration
files are modified by the implemented commands.

The HTTP API is restricted to the local loopback address.

The current filesystem adapter is designed for trusted
installation directories.

It does not provide complete protection against
concurrent changes by untrusted local users.

## License

License selection is pending.
