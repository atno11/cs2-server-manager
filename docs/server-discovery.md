# Server Discovery

## Overview

CServer Manager provides read-only discovery of existing
Counter-Strike 2 dedicated server instances.

Discovery is shared across CLI, TUI and HTTP through
ServerDiscoveryService.

Optional infrastructure discovery identifies OverlayFS
directory metadata and systemd overlay mount-unit configuration.

Configuration evidence does not establish runtime activity.

Discovery never creates, deletes, starts, stops, restarts
or modifies CS2 servers.

## Configuration

Set an absolute root containing the instance directories:

    export CSERVER_ROOT=/example/servers

Optional infrastructure locations:

    export CSERVER_OVERLAY_ROOT=/example/overlay
    export CSERVER_SYSTEMD_UNIT_DIRS=/example/units

CSERVER_SYSTEMD_UNIT_DIRS supports multiple directories
separated by the operating system's path-list separator.

No default infrastructure location is assumed.

An unconfigured or inaccessible CSERVER_ROOT is reported
as an error instead of an empty inventory.

Infrastructure paths are optional. Missing or unreadable
infrastructure locations produce safe discovery warnings.

## CLI

Discover servers:

    cserver servers list

The command returns a JSON array.

When infrastructure evidence is available, each server may
contain an optional `infrastructure` object.

When no infrastructure metadata is available, that object
is omitted.

An empty root returns [].

Exit codes:

- 0: Discovery succeeded.
- 1: Configuration or discovery failed.
- 2: Invalid command usage.

## TUI

Start the TUI:

    cserver tui

Select "Servers" from the main menu.

Controls:

- Up/Down or j/k: Scroll through discovered instances.
- r: Refresh discovery.
- Esc or Backspace: Return to the main menu.
- q: Return to the main menu.
- Ctrl+C: Exit the application.

The first visible server is the focus of the infrastructure
details panel.

The panel displays recognized configuration paths, sources,
unit metadata, runtime inspection status and safe warnings.

A visible systemd mount unit does not mean that it is active.

Discovery runs asynchronously as a Bubble Tea command.
Results from superseded discovery requests are ignored.

## HTTP API

Start the local API:

    cserver api serve

Retrieve the inventory:

    curl http://127.0.0.1:8080/api/v1/servers

The response envelope remains:

    {
      "servers": [],
      "count": 0
    }

The optional `infrastructure` property is available inside
individual server objects when metadata has been discovered.

HTTP status codes remain:

- 200: Discovery succeeded.
- 503: Discovery unavailable or not configured.
- 405: Unsupported HTTP method.

Successful responses include Cache-Control: no-store.

The HTTP listener is restricted to 127.0.0.1.

The response can contain local filesystem paths. Do not
expose the endpoint publicly without suitable security.

## Shared Architecture

Application initialization creates one ServerDiscoveryService.

The service delegates basic discovery to the filesystem
Discoverer and optional infrastructure enrichment to the
InfrastructureBatchReader.

Each configured systemd unit directory is scanned once
per inventory refresh rather than once per server.

CLI, TUI and HTTP call the same discovery service.

No interface independently scans infrastructure paths.

## Discovery Status

Basic discovery status:

- complete: Basic .env and Compose artifacts were found.
- incomplete: A candidate is missing one or more artifacts.

Neither status indicates runtime health.

OverlayFS metadata may describe:

- Lower directories declared by a mount unit.
- Upper directory.
- Work directory.
- Merged directory.
- Configuration evidence source.

systemd metadata may describe:

- Mount-unit filename.
- Source unit-file path.
- Configured mount destination.

Infrastructure runtime_state is always `not_checked`
in this release.

It does not mean active, inactive, mounted, unmounted,
running or stopped.

A missing infrastructure object does not mean that
infrastructure is absent or inactive.

## Security and Limitations

Discovery reads only configured filesystem locations.

Sensitive CS2 environment variables and arbitrary systemd
unit options are not returned.

Read-only infrastructure metadata may expose local
filesystem paths, including installation layout details.

The initial infrastructure scanner does not implement complete
systemd semantics, drop-in parsing, specifier expansion,
advanced mount-option escaping or active mount inspection.

Filesystem symlink protections do not defend against every
concurrent change by an untrusted local user.

Do not scan attacker-controlled directories with unnecessary
privileges.

## Testing

Unit and integration tests use temporary directories and
fixture configuration files.

Run:

    go test ./...
    go vet ./...
    go test -race ./...
    go build -o .tmp/cserver ./cmd/cserver

No active OverlayFS mount, systemd manager, running Docker
daemon or game server is required.

## Related Documentation

See discovery-infrastructure.md for infrastructure-specific
configuration and behavior.

See ../api/openapi.yaml for the HTTP response contract.

## Future Work

Future stages may add verified runtime inspection through
read-only system facilities and Docker integrations.

Runtime checks must remain separate from configuration evidence.

Server lifecycle operations are outside this stage.
