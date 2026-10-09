
# Server Discovery

## Overview

CServer Manager supports read-only discovery of existing
Counter-Strike 2 dedicated server instances.

Discovery is shared across the CLI, TUI, and HTTP API.

The application does not create, delete, start, stop,
restart, or modify existing servers during discovery.

## Configuration

Set CSERVER_ROOT to an absolute path containing instance
directories.

Example:

    export CSERVER_ROOT=/path/to/cs2

The root is not created automatically.

An unconfigured or inaccessible root is reported as an
error rather than an empty server inventory.

## CLI

Discover servers:

    cserver servers list

The command returns a JSON array.

An empty root with no discoverable instances returns [].

Exit codes:

- 0: Discovery succeeded.
- 1: Configuration or discovery failed.
- 2: Invalid command usage.

## TUI

Start the TUI:

    cserver tui

Select "Servers" in the main menu.

Controls:

- Up/Down or j/k: Navigate the displayed list.
- r: Refresh the server inventory.
- Esc or Backspace: Return to the main menu.
- q: Return to the main menu.
- Ctrl+C: Exit the application.

Discovery runs as a Bubble Tea command instead of
blocking the event handler.

Results from superseded requests are ignored.

## HTTP API

Start the local API:

    cserver api serve

Query the inventory:

    curl http://127.0.0.1:8080/api/v1/servers

Successful response:

    {
      "servers": [],
      "count": 0
    }

The HTTP API uses these status codes:

- 200: Discovery succeeded.
- 503: Discovery unavailable or not configured.
- 405: Unsupported HTTP method.

The endpoint is read-only and does not expose raw
environment file contents.

The HTTP listener remains restricted to 127.0.0.1.

The response includes local filesystem paths and should
not be exposed publicly without appropriate security.

## Shared Architecture

Application initialization creates one
ServerDiscoveryService instance.

The service delegates reads to the filesystem Discoverer.

The CLI, TUI, and HTTP interfaces call the same service.

No interface implements its own filesystem scan.

## Discovery Status

The initial filesystem adapter reports:

- complete: Basic .env and Compose files are found.
- incomplete: A candidate is identified but is missing
  one or more basic configuration artifacts.

These statuses do not report:

- Docker container state.
- Docker Compose validity.
- Active OverlayFS mount state.
- systemd unit state.
- Game server health.
- Network availability.

## Security and Limitations

Discovery only reads configuration and directory metadata.

Known non-secret CS2 environment fields may be inspected
to identify candidates.

Passwords, tokens, and raw environment contents are not
returned through the interfaces.

The initial filesystem adapter does not fully parse
Docker Compose YAML or resolve every referenced env_file.

Symlink checks are preliminary and do not provide
complete protection against concurrent filesystem changes
by an untrusted local user.

Do not run the discovery process with elevated privileges
against attacker-controlled directories.

## Testing

Unit and integration tests use temporary directories and
in-memory service readers.

Run:

    go test ./...
    go vet ./...
    go test -race ./...
    go build -o .tmp/cserver ./cmd/cserver

No real CS2 installation, Docker daemon, OverlayFS mount,
or systemd service is required.

## Future Work

A later stage may enrich discovery with read-only Docker,
OverlayFS, and systemd metadata.

Server lifecycle operations are intentionally excluded
from this stage.
