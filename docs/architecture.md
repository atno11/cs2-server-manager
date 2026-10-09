# CServer Manager Architecture

## 1. Overview

CServer Manager is a modular Go application designed
to manage Counter-Strike 2 dedicated server instances.

The application follows a modular monolith architecture.

CLI, TUI, and HTTP share application services
and domain models.

## 2. Architecture Layers

### Core / Domain

Location: `internal/core`

Responsibilities:

- Domain models.
- Business invariants.
- Domain validation.
- Domain-specific errors.

The domain must remain independent of external
technologies such as Docker, HTTP, and systemd.

### Services / Application

Location: `internal/service`

Responsibilities:

- Shared application logic.
- Application use cases.
- Operation orchestration.
- Error propagation.
- Rollback coordination when applicable.

The initial shared service provides application metadata.

Future use cases include:

- Discover servers.
- Create server instances.
- Start and stop servers.
- Restart servers.
- Update installations.
- Read and update server configuration.

### Application Initialization

Location: `internal/app`

Responsibilities:

- Initialize shared application configuration.
- Provide application metadata to interfaces.
- Construct structured application loggers.

Application initialization must not modify
existing CS2 infrastructure.

### Infrastructure

Location: `internal/infrastructure`

Future infrastructure adapters will integrate with:

- Docker Engine.
- Docker Compose.
- OverlayFS.
- systemd.
- Local filesystems.

Adapters must not contain domain-specific business rules.

External dependencies should be introduced only when
a concrete application use case requires them.

### Interfaces

Location: `internal/interfaces`

Supported interfaces:

- CLI: Direct command execution.
- TUI: Interactive terminal navigation.
- HTTP: REST API for external clients.

Interfaces handle user input, invoke application
services, and present results.

Business operations must not be duplicated.

### Configuration

Location: `internal/config`

Responsibilities:

- Load application configuration.
- Validate configuration values.
- Provide normalized settings.

Installation-specific paths must remain configurable.

## 3. Dependency Direction

Interfaces depend on application services
and shared application initialization.

Application services depend on domain models and
minimal integration contracts.

Infrastructure adapters implement these contracts.

The domain layer must not depend on interfaces,
application services, or infrastructure.

## 4. Current Interface Architecture

### CLI

The CLI is the executable's initial command dispatcher.

Current commands:

- help
- version
- config show
- api serve
- tui

### TUI

The TUI uses Bubble Tea v2 and Lip Gloss v2.

The initial implementation provides:

- Keyboard navigation.
- Application information.
- Configuration information.
- Safe application exit.

The TUI does not perform server administration.

### HTTP API

The HTTP adapter uses the Go net/http package.

Implemented endpoints:

- GET /healthz
- GET /api/v1/info

Application metadata is provided by
the shared service package.

The listener uses a configurable local-only
address, defaulting to 127.0.0.1:8080.

The HTTP server supports graceful shutdown
through context.Context.

The API contract is defined in api/openapi.yaml.

## 5. Configuration

Configuration is loaded from environment variables.

Current variables:

- CSERVER_ROOT
- CSERVER_HTTP_ADDRESS
- CSERVER_LOG_LEVEL
- CSERVER_LOG_FORMAT

The loader validates configuration before
initializing interfaces that require it.

No server directories are created during loading.

The HTTP listener must use 127.0.0.1.

## 6. Design Principles

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

## 7. Error Handling

Errors must be returned explicitly.

Use wrapped errors with %w when preserving
the original error is important.

Expected error categories may use sentinel errors
and errors.Is.

Do not use panic for normal operational failures.

CLI exit codes:

- 0: Success.
- 1: Execution or configuration error.
- 2: Invalid command usage.

## 8. Structured Logging

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
authentication tokens, or credentials.

## 9. Testing

Use the standard Go testing package.

Unit tests live alongside their packages.

HTTP handlers are tested using net/http/httptest.

TUI navigation is tested through Bubble Tea messages.

Configuration and logging are tested independently.

Integration tests may be placed under tests/
when external integrations are introduced.

Unit tests must not require real CS2 servers,
Docker containers, or systemd services.

## 10. Operational Safety

Administrative operations must validate inputs
before modifying infrastructure.

Operations should support cancellation and timeouts.

Rollback should be implemented where applicable.

Application startup must not implicitly modify
existing CS2 servers.

Paths must be validated before filesystem operations.

The current HTTP API is restricted to loopback.

## 11. Platform Compatibility

Target platforms:

- Linux.
- Windows.
- macOS.

Platform-specific infrastructure features must be
isolated behind appropriate adapters.

Linux-only features such as OverlayFS and systemd
must not prevent the application core and interfaces
from compiling on other platforms.

## 12. Future Evolution

Additional components will be introduced as concrete
use cases are implemented and tested.

Future work may include:

- Docker and Docker Compose adapters.
- Instance discovery.
- Platform-specific lifecycle operations.
- Expanded TUI navigation.
- Authenticated administrative HTTP endpoints.
- Persistent configuration when required.

The architecture should remain simple and avoid
premature generalization.
