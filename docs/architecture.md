# CServer Manager Architecture

## 1. Overview

CServer Manager is a modular Go application designed
to manage Counter-Strike 2 dedicated server instances.

The project follows a modular monolith architecture.

CLI, TUI and HTTP share application services and domain models.

The current implementation provides read-only server discovery
and optional OverlayFS and systemd mount-unit configuration
metadata.

No server lifecycle operations are implemented.

## 2. Architecture Layers

### Core / Domain

Location: `internal/core`

Responsibilities:

- Domain models.
- Business invariants.
- Domain validation.
- Domain-specific errors.
- Discovery and infrastructure metadata.

The domain remains independent of Docker, HTTP, systemd
and presentation libraries.

DiscoveredServer represents a filesystem-discovered instance.

InfrastructureMetadata contains optional OverlayFS,
systemd mount-unit and discovery warning information.

Infrastructure configuration evidence must not be treated
as runtime health or activity.

### Services / Application

Location: `internal/service`

Responsibilities:

- Shared application logic.
- Application use cases.
- Operation orchestration.
- Error propagation.
- Rollback coordination when applicable.

Implemented services include application information
and read-only server discovery.

ServerDiscoveryService exposes the List operation to
all presentation interfaces.

ServerReader abstracts basic instance discovery.

InfrastructureBatchReader abstracts optional,
read-only infrastructure enrichment.

The original ServerDiscoveryService constructor remains
available for clients without infrastructure enrichment.

The service does not parse configuration files or
query infrastructure runtime state directly.

Future use cases may include:

- Create server instances.
- Start and stop servers.
- Restart servers.
- Update installations.
- Read and update server configuration.

### Application Initialization

Location: `internal/app`

Responsibilities:

- Load and validate shared configuration.
- Provide application metadata.
- Construct application services.
- Configure structured logging.
- Wire replaceable infrastructure adapters.

Initialization must not scan or modify existing
CS2 server installations.

Discovery is performed only when requested.

### Infrastructure

Location: `internal/infrastructure`

Implemented adapters include read-only filesystem
discovery of server instances and optional infrastructure
metadata from explicitly configured paths.

The infrastructure reader can inspect:

- CS2 instance configuration evidence.
- Per-instance OverlayFS directory metadata.
- Recognized systemd overlay mount-unit files.

Systemd unit directories are scanned once per
inventory refresh.

Parsed units are associated with already-discovered
server instances using indexed path evidence.

Units matching multiple instances are not assigned
automatically.

Conflicting filesystem and declared OverlayFS paths
produce safe warnings.

Infrastructure readers do not execute mount, umount,
systemctl or Docker lifecycle operations.

Runtime inspection is not implemented.

Future infrastructure integrations may include
read-only Docker and Docker Compose state inspection.

External dependencies are introduced only when a
concrete application use case requires them.

### Interfaces

Location: `internal/interfaces`

Supported interfaces:

- CLI: Direct command execution.
- TUI: Interactive terminal navigation.
- HTTP: REST API for external clients.

Interfaces invoke shared application services
instead of independently scanning infrastructure.

The CLI exposes server inventory as a JSON array.

The HTTP API exposes the inventory inside a response
containing servers and count.

The TUI displays discovered instances and available
configuration metadata.

### Configuration

Location: `internal/config`

Responsibilities:

- Load application configuration.
- Validate configuration values.
- Normalize installation-specific paths.
- Provide settings to application initialization.

All infrastructure roots are configurable.

No server, OverlayFS or systemd installation path
is assumed automatically.

## 3. Dependency Direction

Interfaces depend on shared application services
and application initialization.

Application services depend on domain models
and small integration interfaces.

Infrastructure adapters implement those interfaces.

The domain must not depend on interfaces,
application services or infrastructure implementations.

No interface duplicates the filesystem discovery logic.

## 4. Current Interface Architecture

### CLI

Implemented commands include:

- help
- version
- config show
- servers list
- api serve
- tui

The servers list command returns a JSON array with
optional infrastructure configuration metadata.

### TUI

The TUI uses Bubble Tea v2 and Lip Gloss v2.

Implemented screens include:

- Main menu.
- Application information.
- Configuration information.
- Discovered servers and infrastructure metadata.

Discovery is requested asynchronously.

Results from outdated discovery requests are ignored.

The first visible server is the focus of the
infrastructure metadata panel.

The TUI does not perform server administration.

### HTTP API

The HTTP adapter uses Go net/http.

Implemented endpoints:

- GET /healthz
- GET /api/v1/info
- GET /api/v1/servers

The server inventory includes optional infrastructure
configuration metadata using the shared domain models.

The listener is restricted to a configurable
127.0.0.1 address.

The servers endpoint sets Cache-Control: no-store.

The HTTP server supports graceful shutdown
through context.Context.

The API contract is defined in api/openapi.yaml.

## 5. Configuration

Configuration is loaded from environment variables.

Current variables:

- CSERVER_ROOT
- CSERVER_OVERLAY_ROOT
- CSERVER_SYSTEMD_UNIT_DIRS
- CSERVER_HTTP_ADDRESS
- CSERVER_LOG_LEVEL
- CSERVER_LOG_FORMAT

The loader validates configuration before interfaces
and services are initialized.

CSERVER_SYSTEMD_UNIT_DIRS supports multiple absolute
paths separated by the operating system's path-list
separator.

No infrastructure directories are created during
configuration loading.

The HTTP listener must use 127.0.0.1.

## 6. Discovery Semantics

Basic discovery statuses are:

- complete
- incomplete

They indicate basic filesystem configuration
completeness, not runtime status.

Infrastructure metadata may report recognized
OverlayFS directories and systemd mount-unit declarations.

The infrastructure runtime_state is always:

    not_checked

No runtime health or mount activation is inferred
from the presence or absence of configuration files.

Missing infrastructure metadata does not prove
that a server is stopped or unmounted.

## 7. Design Principles

1. Prefer the Go standard library.
2. Avoid unnecessary abstractions.
3. Introduce interfaces when they have actual consumers.
4. Keep business logic independent of presentation.
5. Avoid circular dependencies.
6. Keep external integrations replaceable and testable.
7. Preserve compatibility with existing installations.
8. Never assume fixed server installation paths.
9. Validate administrative inputs before execution.
10. Keep startup free of infrastructure side effects.
11. Distinguish configuration evidence from runtime status.
12. Keep discovery read-only and secrets out of results.

## 8. Error Handling

Errors must be returned explicitly.

Use wrapped errors with %w when preserving
the original error is important.

Expected error categories may use sentinel errors
and errors.Is.

Do not use panic for normal operational failures.

Safe discovery warnings describe incomplete or ambiguous
infrastructure configuration without exposing raw
configuration values.

CLI exit codes:

- 0: Success.
- 1: Execution or configuration error.
- 2: Invalid command usage.

## 9. Structured Logging

Logging uses the Go log/slog package.

The application supports text and JSON output.

Supported levels:

- debug
- info
- warn
- error

Operational logs are written to stderr.

User-facing command results remain on stdout.

Logs must not expose secrets, passwords,
authentication tokens or credentials.

## 10. Testing

Use the standard Go testing package.

Unit tests live alongside their packages.

HTTP handlers are tested using net/http/httptest.

TUI navigation is tested through Bubble Tea messages.

Configuration and logging are tested independently.

Discovery tests use temporary filesystem fixtures.

Infrastructure tests cover:

- External configuration roots.
- Valid and invalid unit files.
- Missing and unrelated units.
- Symbolic links.
- Duplicate unit directories.
- Conflicting configuration paths.
- Cross-instance association ambiguity.
- Public JSON compatibility.
- Secret filtering.
- Cancellation and concurrency safety.

A benchmark measures read-only infrastructure
discovery using multiple fixture instances and units.

Tests do not require live CS2 servers, Docker
containers, active OverlayFS mounts or systemd services.

## 11. Operational Safety

Discovery must remain read-only.

Application startup must not implicitly modify
existing CS2 installations.

Paths must be validated before filesystem access.

Only explicitly configured infrastructure sources
are scanned.

The parser retains recognized metadata instead
of returning raw configuration contents.

Warnings must not disclose secrets.

Filesystem symlink checks do not provide complete
protection against concurrent adversarial changes.

Use trusted directories and minimal privileges.

The HTTP API remains restricted to loopback.

Server lifecycle operations are outside the current scope.

## 12. Platform Compatibility

Target platforms:

- Linux.
- Windows.
- macOS.

Platform-specific infrastructure features must remain
isolated behind adapters.

The core and presentation interfaces must not require
a running Linux systemd manager or OverlayFS mount.

Path behavior and available infrastructure metadata
may vary across platforms.

## 13. Future Evolution

Future work may include:

- Read-only Docker and Docker Compose inspection.
- Verified mount and systemd runtime status.
- Server lifecycle operations.
- Configuration editing.
- Installation and update workflows.
- Expanded TUI navigation.
- Authenticated administrative HTTP endpoints.
- Persistent configuration when required.

Runtime inspection must remain separate from
configuration discovery.

The architecture should remain simple and avoid
premature generalization.
