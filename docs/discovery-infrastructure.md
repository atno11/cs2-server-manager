# Infrastructure Discovery — Stage 2C

## Purpose

CServer Manager supports read-only discovery of OverlayFS
directory metadata and systemd overlay mount-unit configuration.

Infrastructure discovery enriches already identified CS2
server instances. It never creates server records from
unrelated infrastructure files.

Configuration evidence does not establish runtime activity.

## Configuration

Optional environment variables:

- `CSERVER_OVERLAY_ROOT`: Absolute OverlayFS storage root.
- `CSERVER_SYSTEMD_UNIT_DIRS`: List of absolute unit directories.

Multiple unit directories are separated by the operating
system's path-list separator.

No host-wide systemd search is performed automatically.

No fixed CS2, OverlayFS or systemd installation path
is required.

## Discovery Workflow

1. Discover CS2 instances under CSERVER_ROOT.
2. Read each configured systemd unit directory once.
3. Parse recognized overlay mount-unit declarations.
4. Inspect per-instance OverlayFS directories.
5. Build an index of potential instance associations.
6. Associate units using Where, upperdir and workdir.
7. Reject cross-instance ambiguous associations.
8. Reconcile filesystem and declared paths.
9. Attach safe metadata and warnings.
10. Return results through ServerDiscoveryService.

Repeated configured unit directories are normalized
and scanned only once.

Unrelated units do not create CS2 instances.

## Association Rules

Unit ownership is inferred only from recognized configuration
evidence associated with an already-discovered instance.

A unit with evidence matching exactly one instance
may be associated with that instance.

A unit with evidence matching multiple instances
is not assigned automatically.

Affected instances receive the warning:

    ambiguous_systemd_unit

The warning does not disclose arbitrary unit contents.

If multiple distinct units match the same instance,
the first matching unit is selected and the instance
receives the warning:

    multiple_systemd_units

Unit association does not imply that the unit is valid,
enabled, loaded, active or successfully mounted.

## Path Conflicts

OverlayFS directories found on disk are separate evidence
from paths declared in systemd mount units.

When a discovered filesystem path disagrees with a path
declared by the selected unit, filesystem evidence
takes precedence in the combined OverlayFS metadata.

The instance receives the warning:

    overlay_path_conflict

The systemd metadata preserves the selected unit's
configured Where path independently.

A conflict is a configuration diagnostic, not proof
of an active mount failure.

## OverlayFS Metadata

Optional properties:

- lower_directories
- upper_directory
- work_directory
- merged_directory
- source
- runtime_state

The source can be:

- filesystem
- systemd_unit
- filesystem_and_systemd_unit

Only source and runtime_state are mandatory when
the overlayfs object is present.

A declared path is not necessarily accessible or mounted.

The current filesystem adapter checks configured
per-instance OverlayFS directories and does not verify
that discovered directories are active overlay layers.

## systemd Metadata

Optional per-server metadata:

- unit_name
- unit_file
- where
- runtime_state

Recognized units declare Type=overlay in their [Mount]
section and contain an absolute Where path.

Unit filenames are not required to match a hardcoded
installation path or naming prefix.

Unit presence does not establish that a unit is enabled,
loaded, active or successfully mounted.

## Runtime State

The sole supported runtime_state is:

    not_checked

This states that runtime status was not verified.

It must not be interpreted as:

- active
- inactive
- mounted
- unmounted
- running
- stopped

No Docker lifecycle operation, mount, umount,
systemctl start or systemctl stop command is executed.

No systemd D-Bus or mount-table runtime query occurs.

## CLI and HTTP

The public server JSON includes optional metadata:

    "infrastructure": {
      "overlayfs": {
        "source": "filesystem",
        "runtime_state": "not_checked"
      }
    }

Infrastructure metadata is omitted when it is unavailable
and no infrastructure warning was produced.

Warning-only infrastructure objects are valid.

Existing server fields and the /api/v1/servers response
envelope are preserved.

The HTTP response is local-only and uses no-store caching.

## TUI

The Servers screen displays compact infrastructure indicators
beside discovered instances.

The first visible instance's configuration metadata is
displayed below the inventory.

The panel distinguishes configuration from runtime activity.

The existing asynchronous refresh and navigation behavior
is preserved.

## Performance

Configured unit directories are scanned once per
inventory refresh.

An index of potential Where, upperdir and workdir
associations avoids scanning every parsed unit separately
for every instance.

The typical association cost scales with the number
of instances, parsed units and matching relationships.

Filesystem reads and parsing still contribute to total
discovery time.

Benchmarks use temporary fixtures and do not represent
guaranteed production latency.

## Security

Discovery inspects only explicitly configured locations.

Unit files must be regular, non-symlink files within
the size limit.

Only allowlisted mount configuration fields are retained.

Passwords, tokens, arbitrary mount options, executable
directives and raw unit contents are not returned.

Warnings use fixed diagnostic messages and do not expose
unrecognized unit values.

Unit paths can reveal details about the local installation.
Keep the HTTP listener restricted to trusted local access.

The scanner does not implement:

- Complete systemd unit semantics.
- Drop-in configuration merging.
- Full specifier or mount-option escaping.
- Active mount or systemd runtime verification.
- Complete protection against concurrent adversarial
  filesystem modifications.

Use trusted installation directories and avoid
unnecessary privileges.

## Testing

Use temporary directories and fixtures covering:

- External OverlayFS paths.
- External systemd mount units.
- Multiple CS2 instances.
- Missing or malformed units.
- Unrelated units.
- Symbolic links.
- Multiple matching units for one instance.
- Units matching multiple instances.
- Conflicting OverlayFS directory declarations.
- Duplicate configured unit directories.
- Safe warning propagation.
- Secret filtering.
- CLI JSON serialization.
- HTTP response compatibility.
- TUI infrastructure presentation.
- Runtime state remaining not_checked.

Run:

    go test ./...
    go vet ./...
    go test -race ./...
    go build -o .tmp/cserver ./cmd/cserver

Optional benchmark:

    go test ./internal/infrastructure/filesystem \
      -run '^$' \
      -bench '^BenchmarkInspectAllManyInstances$' \
      -benchmem

The tests must not require privileged operations.
